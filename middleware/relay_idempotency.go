package middleware

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash"
	"io"
	"maps"
	"mime"
	"mime/multipart"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	relayIdempotencyWindow = 24 * time.Hour
	relayReplayLimit       = 32 << 10
)

// RelayIdempotency persists inference and management submissions before
// dispatch. An uncertain submission keeps its original claim across restarts.
func RelayIdempotency() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.FullPath()
		managedToken := strings.HasPrefix(c.GetString("token_key"), model.ManagedRelayKeyPrefix)
		management := (path == "/api/token/" && c.Request.Method == http.MethodPost ||
			(path == "/api/token/:id" || path == "/api/token/:id/status") && c.Request.Method == http.MethodPut) &&
			c.GetInt("access_token_id") > 0 && c.GetInt("role") >= common.RoleAdminUser
		switch path {
		case "/v1/chat/completions", "/v1/messages", "/v1/responses", "/v1/images/generations", "/v1/images/edits", "/v1/video/generations", constant.UpscaleTaskPath:
		default:
			if !management {
				return
			}
		}
		headers := c.Request.Header.Values("Idempotency-Key")
		if (management || managedToken || path == constant.UpscaleTaskPath) && len(headers) == 0 {
			abortRelayIdempotency(c, http.StatusBadRequest, "invalid_request", "Idempotency-Key is required", "not_sent")
			return
		}
		if (c.Request.Method != http.MethodPost && !management) || len(headers) == 0 {
			return
		}
		if len(headers) != 1 || len(headers[0]) < 16 || len(headers[0]) > 128 ||
			strings.ContainsFunc(headers[0], func(r rune) bool { return r < '!' || r > '~' || r == ',' }) {
			abortRelayIdempotency(c, http.StatusBadRequest, "invalid_request", "Invalid Idempotency-Key", "not_sent")
			return
		}
		accountID := c.GetInt("id")
		tokenID := c.GetInt("token_id")
		if management {
			defer common.CleanupBodyStorage(c)
			// The management credential must still be authorized on replay.
			tokenID = -c.GetInt("access_token_id")
			if id := c.Param("id"); id != "" {
				path = strings.Replace(path, ":id", id, 1)
			}
		}
		if accountID <= 0 || tokenID == 0 {
			abortRelayIdempotency(c, http.StatusUnauthorized, "invalid_api_key", "Invalid API key", "not_sent")
			return
		}
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			status := http.StatusBadRequest
			if common.IsRequestBodyTooLargeError(err) {
				status = http.StatusRequestEntityTooLarge
			}
			abortRelayIdempotency(c, status, "invalid_request", "Unable to read the request body", "not_sent")
			return
		}
		fingerprint, err := relayRequestFingerprint(c.Request, storage)
		if err != nil {
			abortRelayIdempotency(c, http.StatusBadRequest, "invalid_request", "Invalid request body", "not_sent")
			return
		}
		reader, err := storage.NewReader()
		if err != nil {
			abortRelayIdempotency(c, http.StatusInternalServerError, "internal_error", "Unable to read the request body", "not_sent")
			return
		}
		defer reader.Close()
		c.Request.Body = reader

		now := common.GetTimestamp()
		requestID := c.GetString(common.RequestIdKey)
		if requestID == "" {
			requestID = common.NewRequestId()
			c.Set(common.RequestIdKey, requestID)
			c.Header(common.RequestIdKey, requestID)
			c.Header("X-Request-Id", requestID)
		}
		scope := sha256.Sum256(fmt.Appendf(nil, "%d:%s:%s:%s", accountID, c.Request.Method, path, headers[0]))
		record, err := model.ClaimRelayRequest(c.Request.Context(), model.RelayIdempotency{
			Scope: fmt.Sprintf("%x", scope), TokenID: tokenID, Fingerprint: fingerprint, RequestID: requestID, CreatedAt: now,
		})
		if err != nil {
			logger.LogError(c, common.LogText("failed to claim relay request %s: %s", requestID, err.Error()))
			abortRelayIdempotency(c, http.StatusServiceUnavailable, "unavailable", "Unable to reserve the request", "not_sent")
			return
		}
		c.Header("Cache-Control", "no-store")
		if record.RequestID != requestID {
			replayRelayRequest(c, record, tokenID, fingerprint, now)
			return
		}

		common.SetContextKey(c, constant.ContextKeyIdempotentRelay, true)
		writer := &relayReplayWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		c.Next()
		c.Writer = writer.ResponseWriter

		// Preserve encrypted successful JSON replies. Streaming, oversized, or
		// interrupted replies always keep the claim but cannot be replayed.
		var response string
		status := writer.Status()
		if !writer.skip && writer.body.Len() > 0 && (status/100 == 2 || management && status >= 400 && status < 500) &&
			strings.HasPrefix(writer.Header().Get("Content-Type"), "application/json") && c.Request.Context().Err() == nil {
			if aead, err := relayReplayCipher(); err == nil {
				sealed := aead.Seal(nil, nil, writer.body.Bytes(), []byte(record.Scope+record.RequestID))
				response = base64.RawStdEncoding.EncodeToString(sealed)
			}
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Second)
		defer cancel()
		if err := model.FinishRelayRequest(ctx, record, common.GetTimestamp(), status, response); err != nil {
			logger.LogError(c, common.LogText("failed to finish relay request %s: %s", requestID, err.Error()))
		}
	}
}

func relayRequestFingerprint(req *http.Request, storage common.BodyStorage) (string, error) {
	contentType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		return "", err
	}
	reader, err := storage.NewReader()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	digest := sha256.New()
	boundary := params["boundary"]
	delete(params, "boundary")
	// Negotiation headers can change the response and its billing profile.
	for _, value := range []string{
		req.URL.Query().Encode(), mime.FormatMediaType(contentType, params), req.Header.Get("Accept"),
		req.Header.Get("Anthropic-Version"), req.Header.Get("Anthropic-Beta"), req.Header.Get("OpenAI-Beta"),
	} {
		appendFingerprintValue(digest, value)
	}
	switch {
	case contentType == "multipart/form-data":
		digest.Write([]byte("multipart:"))
		err = appendMultipartFingerprint(digest, multipart.NewReader(reader, boundary))
	case storage.Size() > 4<<20:
		// Avoid a second full-size object for large base64 requests.
		digest.Write([]byte("raw:"))
		_, err = io.Copy(digest, reader)
	default:
		var normalized []byte
		normalized, err = common.NormalizeJSONBody(reader)
		digest.Write([]byte("json:"))
		digest.Write(normalized)
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), err
}

func appendMultipartFingerprint(digest hash.Hash, reader *multipart.Reader) error {
	fields := make(map[string][][32]byte)
	parts := 0
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		parts++
		if parts > 1000 || part.FormName() == "" {
			return fmt.Errorf("invalid multipart fields")
		}
		partDigest := sha256.New()
		appendFingerprintValue(partDigest, part.FileName())
		appendFingerprintValue(partDigest, part.Header.Get("Content-Type"))
		if _, err := io.Copy(partDigest, part); err != nil {
			return err
		}
		name := part.FormName()
		fields[name] = append(fields[name], [32]byte(partDigest.Sum(nil)))
	}
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		appendFingerprintValue(digest, name)
		parts := fields[name]
		_, _ = fmt.Fprintf(digest, "%d:", len(parts))
		for _, part := range parts {
			_, _ = digest.Write(part[:])
		}
	}
	return nil
}

func appendFingerprintValue(digest hash.Hash, value string) {
	_, _ = fmt.Fprintf(digest, "%d:%s", len(value), value)
}

func replayRelayRequest(c *gin.Context, record model.RelayIdempotency, tokenID int, fingerprint string, now int64) {
	if record.TokenID != tokenID {
		abortRelayIdempotency(c, http.StatusConflict, "idempotency_conflict", "The idempotency key is already in use", "not_sent")
		return
	}
	c.Set(common.RequestIdKey, record.RequestID)
	c.Header(common.RequestIdKey, record.RequestID)
	c.Header("X-Request-Id", record.RequestID)
	if record.Fingerprint != fingerprint {
		abortRelayIdempotency(c, http.StatusConflict, "idempotency_conflict", "The idempotency key was used with different parameters", "not_sent")
		return
	}
	if now < record.CreatedAt+int64(relayIdempotencyWindow/time.Second) {
		if record.FinishedAt == 0 {
			abortRelayIdempotency(c, http.StatusConflict, "request_in_progress", "The original request is still in progress", "unknown")
			return
		}
		if record.Response != "" {
			sealed, decodeErr := base64.RawStdEncoding.DecodeString(record.Response)
			aead, cipherErr := relayReplayCipher()
			if decodeErr == nil && cipherErr == nil {
				body, err := aead.Open(nil, nil, sealed, []byte(record.Scope+record.RequestID))
				if err == nil {
					c.Data(record.ResponseStatus, "application/json", body)
					c.Abort()
					return
				}
			}
		}
	}
	abortRelayIdempotency(c, http.StatusConflict, "result_unavailable", "The original response cannot be replayed", "unknown")
}

func relayReplayCipher() (cipher.AEAD, error) {
	key := sha256.Sum256([]byte("relay-idempotency-response:" + common.CryptoSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}

func abortRelayIdempotency(c *gin.Context, status int, code, message, dispatchState string) {
	if c.FullPath() == "/v1/messages" {
		kind := "invalid_request_error"
		if status >= 500 {
			kind = "api_error"
		}
		c.AbortWithStatusJSON(status, gin.H{
			"type": "error", "request_id": c.GetString(common.RequestIdKey),
			"error": gin.H{"type": kind, "message": message},
		})
		return
	}
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{
		"type": "invalid_request_error", "code": code, "message": message, "param": nil,
		"request_id": c.GetString(common.RequestIdKey), "dispatch_state": dispatchState,
		"retryable": code == "request_in_progress" || status >= 500, "retry_after_seconds": nil,
	}})
}

type relayReplayWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
	skip bool
}

func (w *relayReplayWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if w.capture(n, err) {
		w.body.Write(data[:n])
	}
	return n, err
}

func (w *relayReplayWriter) WriteString(data string) (int, error) {
	n, err := w.ResponseWriter.WriteString(data)
	if w.capture(n, err) {
		w.body.WriteString(data[:n])
	}
	return n, err
}

func (w *relayReplayWriter) capture(size int, writeErr error) bool {
	if w.skip {
		return false
	}
	if writeErr != nil || size > relayReplayLimit-w.body.Len() ||
		strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream") {
		w.skip = true
		w.body.Reset()
	}
	return !w.skip
}
