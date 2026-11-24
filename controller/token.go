package controller

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type tokenAutoGroupsInput struct {
	Set    bool
	Groups []string
}

func (input *tokenAutoGroupsInput) UnmarshalJSON(data []byte) error {
	input.Set = true
	if strings.TrimSpace(string(data)) == "null" {
		input.Groups = nil
		return nil
	}
	return common.Unmarshal(data, &input.Groups)
}

type tokenRequest struct {
	model.Token
	AutoGroups tokenAutoGroupsInput `json:"auto_groups"`
	// 指针区分“未传”和显式 false：新建时未传即开启，更新时未传则保留原值。
	AllowUserQuotaQuery *bool `json:"allow_user_quota_query"`
}

type tokenResponse struct {
	*model.Token
	AutoGroups []string `json:"auto_groups"`
}

func maxTokenQuota() int {
	quota, err := common.WalletQuotaFromDecimalStrict(
		decimal.NewFromInt(1_000_000_000).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
	)
	if err != nil {
		return common.MaxWalletQuota
	}
	return quota
}

func buildMaskedTokenResponse(token *model.Token) *tokenResponse {
	if token == nil {
		return nil
	}
	maskedToken := *token
	maskedToken.Key = token.GetMaskedKey()
	autoGroups, err := token.GetAutoGroups()
	if err != nil {
		common.SysError(common.LogText("failed to parse auto groups for token %d: %v", token.Id, err))
		autoGroups = nil
	}
	if len(autoGroups) == 0 {
		autoGroups = nil
	}
	return &tokenResponse{Token: &maskedToken, AutoGroups: autoGroups}
}

func buildMaskedTokenResponses(tokens []*model.Token) []*tokenResponse {
	maskedTokens := make([]*tokenResponse, 0, len(tokens))
	for _, token := range tokens {
		maskedTokens = append(maskedTokens, buildMaskedTokenResponse(token))
	}
	return maskedTokens
}

func getTokenRequestUserGroup(c *gin.Context) (string, error) {
	if userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup); userGroup != "" {
		return userGroup, nil
	}
	if userGroup := c.GetString("group"); userGroup != "" {
		return userGroup, nil
	}
	return model.GetUserGroup(c.GetInt("id"), false)
}

func setTokenAutoGroups(c *gin.Context, token *model.Token, groups []string) bool {
	if len(groups) == 0 {
		if err := token.SetAutoGroups(nil); err != nil {
			common.ApiError(c, err)
			return false
		}
		return true
	}

	maxCount := setting.GetMaxTokenAutoGroups()
	if len(groups) > maxCount {
		common.ApiErrorT(c, "A token can select at most {{max}} Auto groups", map[string]any{"max": maxCount})
		return false
	}

	userGroup, err := getTokenRequestUserGroup(c)
	if err != nil {
		common.ApiError(c, err)
		return false
	}
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		if _, ok := seen[group]; ok {
			common.ApiErrorT(c, "Auto group {{group}} is duplicated", map[string]any{"group": group})
			return false
		}
		seen[group] = struct{}{}
		if !service.IsUserSelectableGroup(userGroup, group) {
			common.ApiErrorT(c, "Auto group {{group}} is unavailable or unauthorized", map[string]any{"group": group})
			return false
		}
	}

	if err := token.SetAutoGroups(groups); err != nil {
		common.ApiError(c, err)
		return false
	}
	return true
}

func GetAllTokens(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	tokens, err := model.GetAllUserTokens(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	total, _ := model.CountUserTokens(userId)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func SearchTokens(c *gin.Context) {
	if c.GetBool("use_access_token") {
		searchManagedTokens(c)
		return
	}
	userId := c.GetInt("id")
	keyword := c.Query("keyword")
	token := c.Query("token")

	pageInfo := common.GetPageQuery(c)

	tokens, total, err := model.SearchUserTokens(userId, keyword, token, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func GetToken(c *gin.Context) {
	if c.GetBool("use_access_token") {
		if token := readManagedToken(c, true); token != nil {
			c.JSON(http.StatusOK, buildManagedTokenResponse(&token.Token, common.GetTimestamp(), token.Version))
		}
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildMaskedTokenResponse(token))
}

func GetTokenAutoGroups(c *gin.Context) {
	userGroup, err := getTokenRequestUserGroup(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"groups":    service.GetUserAutoGroup(userGroup),
		"max_count": setting.GetMaxTokenAutoGroups(),
	})
}

func GetTokenKey(c *gin.Context) {
	if c.GetBool("use_access_token") {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Unauthorized, insufficient privileges"))
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	params := tokenAuditParams(c)
	params["id"], params["name"] = token.Id, token.Name
	common.SetContextKey(c, constant.ContextKeyTokenAuditSucceeded, true)
	common.ApiSuccess(c, gin.H{
		"key": token.GetFullKey(),
	})
}

func GetTokenStatus(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	userId := c.GetInt("id")
	token, err := model.GetTokenByIds(tokenId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}
	c.JSON(http.StatusOK, gin.H{
		"object":          "credit_summary",
		"total_granted":   token.RemainQuota,
		"total_used":      0, // not supported currently
		"total_available": token.RemainQuota,
		"expires_at":      expiredAt * 1000,
	})
}

func GetTokenUsage(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "No Authorization header",
		})
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid Bearer token",
		})
		return
	}
	tokenKey := parts[1]

	token, err := model.GetTokenByKey(strings.TrimPrefix(tokenKey, "sk-"), false)
	if err != nil {
		common.SysError(common.LogText("failed to get token by key: %s", err.Error()))
		common.ApiErrorI18n(c, i18n.MsgTokenGetInfoFailed)
		return
	}

	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}

	data := gin.H{
		"object":               "token_usage",
		"name":                 token.Name,
		"total_granted":        token.RemainQuota + token.UsedQuota,
		"total_used":           token.UsedQuota,
		"total_available":      token.RemainQuota,
		"unlimited_quota":      token.UnlimitedQuota,
		"model_limits":         token.GetModelLimitsMap(),
		"model_limits_enabled": token.ModelLimitsEnabled,
		"expires_at":           expiredAt,
	}
	// 用户额度只在令牌允许时返回；IP 白名单已由 TokenAuthReadOnly 检查。
	if token.AllowUserQuotaQuery {
		userQuota, err := model.GetUserQuota(token.UserId, false)
		var userUsedQuota int
		if err == nil {
			userUsedQuota, err = model.GetUserUsedQuota(token.UserId)
		}
		if err != nil {
			common.SysError(common.LogText("failed to get user quota: %s", err.Error()))
			common.ApiErrorI18n(c, i18n.MsgTokenGetInfoFailed)
			return
		}
		data["user_quota"] = gin.H{
			"total_available": userQuota,
			"total_used":      userUsedQuota,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    true,
		"message": "ok",
		"data":    data,
	})
}

func AddToken(c *gin.Context) {
	request := tokenRequest{}
	managed := c.GetBool("use_access_token")
	if managed {
		if !requireManagedTokenAccess(c) {
			return
		}
		var input managedTokenCreate
		if common.DecodeStrictJsonBody(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10), &input) != nil ||
			strings.TrimSpace(input.Name) == "" || len(input.Name) > 50 ||
			input.ExpiredTime == nil || (*input.ExpiredTime != -1 && *input.ExpiredTime <= common.GetTimestamp()) ||
			input.RemainQuota < 0 || input.RemainQuota > maxTokenQuota() || !validManagedTokenIPs(input.AllowIps) {
			common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
			return
		}
		if input.Group != "" && input.Group != "auto" {
			userGroup, err := getTokenRequestUserGroup(c)
			if err != nil || !service.IsUserSelectableGroup(userGroup, input.Group) {
				common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
				return
			}
		}
		request = tokenRequest{
			Token: model.Token{
				Name: input.Name, ExpiredTime: *input.ExpiredTime, RemainQuota: input.RemainQuota, UnlimitedQuota: input.UnlimitedQuota,
				ModelLimitsEnabled: input.ModelLimitsEnabled, ModelLimits: input.ModelLimits, AllowIps: input.AllowIps,
				Group: input.Group, CrossGroupRetry: input.CrossGroupRetry,
			},
			AutoGroups: input.AutoGroups, AllowUserQuotaQuery: input.AllowUserQuotaQuery,
		}
		if request.AllowUserQuotaQuery == nil {
			request.AllowUserQuotaQuery = new(false)
		}
	} else if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	token := request.Token
	if len(token.Name) > 50 {
		common.ApiErrorT(c, "Token name is too long")
		return
	}
	params := tokenAuditParams(c)
	params["name"] = token.Name
	// 非无限额度时，检查额度值是否超出有效范围
	if !token.UnlimitedQuota {
		if token.RemainQuota < 0 {
			common.ApiErrorT(c, "Quota value cannot be negative")
			return
		}
		maxQuotaValue := maxTokenQuota()
		if token.RemainQuota > maxQuotaValue {
			common.ApiErrorT(c, "Quota value exceeds valid range, maximum is {{max}}", map[string]any{"max": logger.FormatQuota(maxQuotaValue)})
			return
		}
	}
	// 检查用户令牌数量是否已达上限
	maxTokens := operation_setting.GetMaxUserTokens()
	count, err := model.CountUserTokens(c.GetInt("id"))
	if err != nil {
		if managed {
			writeTokenManagementError(c, err)
		} else {
			common.ApiError(c, err)
		}
		return
	}
	if int(count) >= maxTokens {
		err := common.NewMessage("Maximum number of tokens reached ({{max}})", map[string]any{"max": maxTokens})
		if managed {
			common.ApiErrorStatus(c, http.StatusConflict, err)
		} else {
			common.ApiError(c, err)
		}
		return
	}
	if token.Group == "auto" {
		if !setTokenAutoGroups(c, &token, request.AutoGroups.Groups) {
			return
		}
	} else {
		token.CrossGroupRetry = false
		_ = token.SetAutoGroups(nil)
	}
	key, err := common.GenerateKey()
	if err != nil {
		if managed {
			common.ApiErrorStatus(c, http.StatusInternalServerError, common.NewMessage("Failed to generate token"))
		} else {
			common.ApiErrorT(c, "Failed to generate token")
		}
		common.SysLog(common.LogText("failed to generate token key: %s", err.Error()))
		return
	}
	if managed {
		key = model.ManagedRelayKeyPrefix + key
	}
	cleanToken := model.Token{
		UserId:             c.GetInt("id"),
		Name:               token.Name,
		Key:                key,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        token.ExpiredTime,
		RemainQuota:        token.RemainQuota,
		UnlimitedQuota:     token.UnlimitedQuota,
		ModelLimitsEnabled: token.ModelLimitsEnabled,
		ModelLimits:        token.ModelLimits,
		AllowIps:           token.AllowIps,
		Group:              token.Group,
		CrossGroupRetry:    token.CrossGroupRetry,
		AutoGroups:         token.AutoGroups,
	}
	cleanToken.AllowUserQuotaQuery = request.AllowUserQuotaQuery == nil || *request.AllowUserQuotaQuery
	err = cleanToken.Insert()
	if err != nil {
		if managed {
			// Driver errors can include the inserted credential.
			common.ApiErrorStatus(c, http.StatusInternalServerError, common.NewMessage("Failed to generate token"))
		} else {
			common.ApiError(c, err)
		}
		return
	}
	params["id"] = cleanToken.Id
	common.SetContextKey(c, constant.ContextKeyTokenAuditSucceeded, true)
	if managed {
		writeManagedTokenIssued(c, &cleanToken)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func DeleteToken(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	params := tokenAuditParams(c)
	params["id"], params["name"] = token.Id, token.Name
	err = token.Delete()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.SetContextKey(c, constant.ContextKeyTokenAuditSucceeded, true)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func UpdateToken(c *gin.Context) {
	userId := c.GetInt("id")
	statusOnly := c.Query("status_only")
	request := tokenRequest{}
	err := c.ShouldBindJSON(&request)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token := request.Token
	params := tokenAuditParams(c)
	if token.Id > 0 {
		params["id"] = token.Id
	}
	if len(token.Name) > 50 {
		common.ApiErrorT(c, "Token name is too long")
		return
	}
	if !token.UnlimitedQuota {
		if token.RemainQuota < 0 {
			common.ApiErrorT(c, "Quota value cannot be negative")
			return
		}
		maxQuotaValue := maxTokenQuota()
		if token.RemainQuota > maxQuotaValue {
			common.ApiErrorT(c, "Quota value exceeds valid range, maximum is {{max}}", map[string]any{"max": logger.FormatQuota(maxQuotaValue)})
			return
		}
	}
	cleanToken, err := model.GetTokenByIds(token.Id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	params["name"] = cleanToken.Name
	previous := *cleanToken
	if token.Status == common.TokenStatusEnabled {
		if cleanToken.Status == common.TokenStatusExpired && cleanToken.ExpiredTime <= common.GetTimestamp() && cleanToken.ExpiredTime != -1 {
			common.ApiErrorT(c, "Token has expired and cannot be enabled. Please modify the expiration time or set it to never expire")
			return
		}
		if cleanToken.Status == common.TokenStatusExhausted && cleanToken.RemainQuota <= 0 && !cleanToken.UnlimitedQuota {
			common.ApiErrorT(c, "Token quota is exhausted and cannot be enabled. Please modify the remaining quota or set it to unlimited")
			return
		}
	}
	if statusOnly != "" {
		cleanToken.Status = token.Status
	} else {
		// If you add more fields, please also update token.Update()
		cleanToken.Name = token.Name
		cleanToken.ExpiredTime = token.ExpiredTime
		cleanToken.RemainQuota = token.RemainQuota
		cleanToken.UnlimitedQuota = token.UnlimitedQuota
		cleanToken.ModelLimitsEnabled = token.ModelLimitsEnabled
		cleanToken.ModelLimits = token.ModelLimits
		cleanToken.AllowIps = token.AllowIps
		if request.AllowUserQuotaQuery != nil {
			cleanToken.AllowUserQuotaQuery = *request.AllowUserQuotaQuery
		}
		cleanToken.Group = token.Group
		cleanToken.CrossGroupRetry = token.CrossGroupRetry
		if token.Group != "auto" {
			cleanToken.CrossGroupRetry = false
			_ = cleanToken.SetAutoGroups(nil)
		} else if request.AutoGroups.Set {
			if !setTokenAutoGroups(c, cleanToken, request.AutoGroups.Groups) {
				return
			}
		}
	}
	err = cleanToken.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	params["name"] = cleanToken.Name
	if statusOnly != "" {
		params["from"], params["to"] = previous.Status, cleanToken.Status
	} else {
		changedFields := []string{}
		for _, field := range []struct {
			name    string
			changed bool
		}{
			{"name", previous.Name != cleanToken.Name},
			{"expired_time", previous.ExpiredTime != cleanToken.ExpiredTime},
			{"remain_quota", previous.RemainQuota != cleanToken.RemainQuota},
			{"unlimited_quota", previous.UnlimitedQuota != cleanToken.UnlimitedQuota},
			{"model_limits_enabled", previous.ModelLimitsEnabled != cleanToken.ModelLimitsEnabled},
			{"model_limits", previous.ModelLimits != cleanToken.ModelLimits},
			{"allow_ips", (previous.AllowIps == nil) != (cleanToken.AllowIps == nil) ||
				(previous.AllowIps != nil && cleanToken.AllowIps != nil && *previous.AllowIps != *cleanToken.AllowIps)},
			{"allow_user_quota_query", previous.AllowUserQuotaQuery != cleanToken.AllowUserQuotaQuery},
			{"group", previous.Group != cleanToken.Group},
			{"cross_group_retry", previous.CrossGroupRetry != cleanToken.CrossGroupRetry},
			{"auto_groups", previous.AutoGroups != cleanToken.AutoGroups},
		} {
			if field.changed {
				changedFields = append(changedFields, field.name)
			}
		}
		params["changed_fields"] = changedFields
	}
	common.SetContextKey(c, constant.ContextKeyTokenAuditSucceeded, true)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildMaskedTokenResponse(cleanToken),
	})
}

func UpdateTokenSettings(c *gin.Context) {
	if !requireManagedTokenAccess(c) {
		return
	}
	var request map[string]common.RawMessage
	if err := common.DecodeJsonBody(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10), &request); err != nil || len(request) == 0 {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	var version int
	if len(request) < 2 || common.Unmarshal(request["expected_version"], &version) != nil || version < 1 {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	delete(request, "expected_version")
	fields := make(map[string]any, len(request))
	for name, raw := range request {
		var value any
		switch name {
		case "name", "model_limits", "allow_ips":
			value = new(string)
		case "expired_time":
			value = new(int64)
		case "remain_quota":
			value = new(int)
		case "unlimited_quota", "model_limits_enabled", "allow_user_quota_query":
			value = new(bool)
		}
		if value == nil || string(raw) == "null" || common.Unmarshal(raw, value) != nil {
			common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
			return
		}
		fields[name] = value
	}
	if name, ok := fields["name"].(*string); ok && (strings.TrimSpace(*name) == "" || len(*name) > 50) {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	if expiry, ok := fields["expired_time"].(*int64); ok && *expiry != -1 && *expiry <= common.GetTimestamp() {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	if quota, ok := fields["remain_quota"].(*int); ok && (*quota < 0 || *quota > maxTokenQuota()) {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	if ips, ok := fields["allow_ips"].(*string); ok && !validManagedTokenIPs(ips) {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	token := readManagedToken(c, false)
	if token == nil {
		return
	}
	if err := token.UpdateManagedToken(c.Request.Context(), version, fields, false); err != nil {
		writeTokenManagementError(c, err)
		return
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	slices.Sort(names)
	params := tokenAuditParams(c)
	params["name"], params["changed_fields"] = token.Name, names
	common.SetContextKey(c, constant.ContextKeyTokenAuditSucceeded, true)
	c.JSON(http.StatusOK, buildManagedTokenResponse(&token.Token, common.GetTimestamp(), version+1))
}

func ChangeTokenStatus(c *gin.Context) {
	if !requireManagedTokenAccess(c) {
		return
	}
	var request struct {
		Version int    `json:"expected_version"`
		Status  string `json:"status"`
		Reason  string `json:"reason"`
	}
	if err := common.DecodeStrictJsonBody(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10), &request); err != nil || request.Version < 1 ||
		strings.TrimSpace(request.Reason) == "" || utf8.RuneCountInString(request.Reason) > 500 || !managedStatusAllowed(request.Status) {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	requestedStatus := request.Status
	token := readManagedToken(c, false)
	if token == nil {
		return
	}
	from := token.Status
	status := common.TokenStatusDisabled
	if requestedStatus == "active" {
		status = common.TokenStatusEnabled
	}
	if err := token.UpdateManagedToken(c.Request.Context(), request.Version, map[string]any{"status": status}, requestedStatus == "revoked"); err != nil {
		writeTokenManagementError(c, err)
		return
	}
	params := tokenAuditParams(c)
	params["name"], params["from"], params["to"] = token.Name, from, requestedStatus
	common.SetContextKey(c, constant.ContextKeyTokenAuditSucceeded, true)
	c.JSON(http.StatusOK, buildManagedTokenResponse(&token.Token, common.GetTimestamp(), request.Version+1))
}

func writeTokenManagementError(c *gin.Context, err error) {
	if errors.Is(err, model.ErrManagedTokenVersion) {
		common.ApiErrorStatus(c, http.StatusConflict, common.NewMessage("Invalid parameters"), gin.H{"code": "version_conflict"})
		return
	}
	if errors.Is(err, model.ErrManagedTokenInactive) {
		common.ApiErrorStatus(c, http.StatusConflict, common.NewMessage("Token has expired and cannot be enabled. Please modify the expiration time or set it to never expire"), gin.H{"code": "token_inactive"})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Invalid parameters"))
		return
	}
	logger.LogError(c, common.LogText("failed to manage token %s: %s", c.Param("id"), err.Error()))
	common.ApiErrorStatus(c, http.StatusInternalServerError, common.NewMessage("Internal Server Error!"))
}

type TokenBatch struct {
	Ids []int `json:"ids"`
}

func DeleteTokenBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	params := tokenBatchAuditParams(c, tokenBatch.Ids)
	if len(tokenBatch.Ids) == 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	userId := c.GetInt("id")
	count, err := model.BatchDeleteTokens(tokenBatch.Ids, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	params["count"] = count
	common.SetContextKey(c, constant.ContextKeyTokenAuditSucceeded, true)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
}

func GetTokenKeysBatch(c *gin.Context) {
	if c.GetBool("use_access_token") {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Unauthorized, insufficient privileges"))
		return
	}
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	params := tokenBatchAuditParams(c, tokenBatch.Ids)
	if len(tokenBatch.Ids) == 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if len(tokenBatch.Ids) > 100 {
		common.ApiErrorT(c, "Too many items in batch request, maximum is {{max}}", map[string]any{"max": 100})
		return
	}
	userId := c.GetInt("id")
	tokens, err := model.GetTokenKeysByIds(tokenBatch.Ids, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	keysMap := make(map[int]string)
	returnedIDs := make([]int, 0, len(tokens))
	for _, t := range tokens {
		keysMap[t.Id] = t.GetFullKey()
		returnedIDs = append(returnedIDs, t.Id)
	}
	params["count"] = len(tokens)
	params["returned_ids"] = returnedIDs
	common.SetContextKey(c, constant.ContextKeyTokenAuditSucceeded, true)
	common.ApiSuccess(c, gin.H{"keys": keysMap})
}
