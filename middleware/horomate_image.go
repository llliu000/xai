package middleware

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func HoroMateImageResponse() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.HasPrefix(c.GetString("token_key"), model.ManagedRelayKeyPrefix) {
			return
		}
		writer := &imageProfileWriter{ResponseWriter: c.Writer, status: http.StatusOK}
		c.Writer = writer
		c.Next()
		c.Writer = writer.ResponseWriter
		c.Writer.Header().Del("Content-Length")
		c.Writer.Header().Del("Content-Encoding")
		c.Header("Cache-Control", "no-store")
		if writer.status != http.StatusOK && !writer.overflow {
			c.Data(writer.status, writer.Header().Get("Content-Type"), writer.body.Bytes())
			return
		}
		if writer.overflow || !writeHoroMateImage(c, writer.body.Bytes()) {
			abortRelayIdempotency(c, http.StatusBadGateway, "upstream_error", "The upstream image does not match the response profile", "accepted")
		}
	}
}

func writeHoroMateImage(c *gin.Context, raw []byte) bool {
	type image struct {
		URL           string `json:"url,omitempty"`
		B64           string `json:"b64_json,omitempty"`
		ExpiresAt     int64  `json:"expires_at,omitempty"`
		RevisedPrompt string `json:"revised_prompt,omitempty"`
	}
	var upstream struct {
		Created int64             `json:"created"`
		Data    common.RawMessage `json:"data"`
		Usage   *dto.Usage        `json:"usage"`
	}
	if common.Unmarshal(raw, &upstream) != nil {
		return false
	}
	var images []image
	if common.Unmarshal(upstream.Data, &images) != nil {
		var item image
		if common.Unmarshal(upstream.Data, &item) != nil {
			return false
		}
		images = []image{item}
	}
	format := "b64_json"
	if value, ok := c.Get("horomate_image_format"); ok && slices.Contains(value.([]string), "url") {
		format = "url"
	}
	images = slices.DeleteFunc(images, func(item image) bool {
		return format == "url" && item.URL == "" || format == "b64_json" && item.B64 == ""
	})
	if len(images) != 1 {
		return false
	}
	item := &images[0]
	if format == "url" {
		item.B64 = ""
		if item.ExpiresAt == 0 {
			if target, err := url.Parse(item.URL); err == nil {
				item.ExpiresAt, _ = strconv.ParseInt(target.Query().Get("Expires"), 10, 64)
			}
		}
		if item.ExpiresAt <= common.GetTimestamp() {
			return false
		}
		target, err := service.ResolvePublicVideoMediaURL(c.Request.Context(), item.URL)
		if err != nil || target != item.URL {
			return false
		}
	} else {
		item.URL, item.ExpiresAt = "", 0
		if _, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(item.B64))); err != nil {
			return false
		}
	}
	requestID := c.GetString(common.RequestIdKey)
	record, err := model.GetUsageRecord(c.Request.Context(), c.GetInt("id"), "bill_"+requestID)
	if err != nil || record.TokenID != c.GetInt("token_id") {
		return false
	}
	var usage any
	if upstream.Usage != nil {
		u := upstream.Usage
		input, output := max(0, u.InputTokens, u.PromptTokens), max(0, u.OutputTokens, u.CompletionTokens)
		cached := u.PromptTokensDetails.CachedTokens
		if u.InputTokensDetails != nil {
			cached = max(cached, u.InputTokensDetails.CachedTokens)
		}
		reasoning := u.CompletionTokenDetails.ReasoningTokens
		if u.OutputTokensDetails != nil {
			reasoning = max(reasoning, u.OutputTokensDetails.ReasoningTokens)
		}
		usage = gin.H{"input_tokens": input, "output_tokens": output, "total_tokens": max(u.TotalTokens, input+output),
			"input_tokens_details":  gin.H{"cached_tokens": min(input, max(0, cached))},
			"output_tokens_details": gin.H{"reasoning_tokens": min(output, max(0, reasoning))}}
	}
	c.JSON(http.StatusOK, gin.H{"created": max(0, upstream.Created), "request_id": requestID, "model": record.Model,
		"data": images, "usage": usage, "billing": gin.H{"status": record.Status, "currency": "USD", "amount": record.NetAmount, "record_id": record.RecordID}})
	return true
}

type imageProfileWriter struct {
	gin.ResponseWriter
	status   int
	written  bool
	overflow bool
	body     bytes.Buffer
}

func (w *imageProfileWriter) Status() int     { return w.status }
func (w *imageProfileWriter) Size() int       { return w.body.Len() }
func (w *imageProfileWriter) Written() bool   { return w.written }
func (w *imageProfileWriter) WriteHeaderNow() { w.written = true }
func (w *imageProfileWriter) Flush()          { w.WriteHeaderNow() }
func (w *imageProfileWriter) WriteHeader(code int) {
	if !w.written {
		w.status = code
	}
}
func (w *imageProfileWriter) Write(data []byte) (int, error) {
	w.written = true
	if len(data) > 64<<20-w.body.Len() {
		w.overflow = true
	}
	if w.overflow {
		return len(data), nil
	}
	return w.body.Write(data)
}
func (w *imageProfileWriter) WriteString(data string) (int, error) { return w.Write([]byte(data)) }
