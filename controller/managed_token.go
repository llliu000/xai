package controller

import (
	"crypto/hmac"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type managedTokenCreate struct {
	Name                string               `json:"name"`
	ExpiredTime         *int64               `json:"expired_time"`
	RemainQuota         int                  `json:"remain_quota"`
	UnlimitedQuota      bool                 `json:"unlimited_quota"`
	ModelLimitsEnabled  bool                 `json:"model_limits_enabled"`
	ModelLimits         string               `json:"model_limits"`
	AllowIps            *string              `json:"allow_ips"`
	Group               string               `json:"group"`
	CrossGroupRetry     bool                 `json:"cross_group_retry"`
	AutoGroups          tokenAutoGroupsInput `json:"auto_groups"`
	AllowUserQuotaQuery *bool                `json:"allow_user_quota_query"`
}

type managedTokenResponse struct {
	ID                  int      `json:"id"`
	Name                string   `json:"name"`
	MaskedKey           string   `json:"masked_key"`
	Status              string   `json:"status"`
	Version             int      `json:"version"`
	CreatedTime         int64    `json:"created_time"`
	ExpiredTime         int64    `json:"expired_time"`
	RevokedAt           *int64   `json:"revoked_at"`
	RemainQuota         int      `json:"remain_quota"`
	UsedQuota           int      `json:"used_quota"`
	UnlimitedQuota      bool     `json:"unlimited_quota"`
	ModelLimitsEnabled  bool     `json:"model_limits_enabled"`
	ModelLimits         string   `json:"model_limits"`
	AllowIps            []string `json:"allow_ips"`
	Group               string   `json:"group"`
	CrossGroupRetry     bool     `json:"cross_group_retry"`
	AutoGroups          []string `json:"auto_groups"`
	AllowUserQuotaQuery bool     `json:"allow_user_quota_query"`
}

func requireManagedTokenAccess(c *gin.Context) bool {
	if c.GetInt("access_token_id") > 0 && c.GetInt("role") >= common.RoleAdminUser {
		return true
	}
	common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Unauthorized, insufficient privileges"))
	return false
}

func buildManagedTokenResponse(token *model.Token, now int64, version int) managedTokenResponse {
	status := "disabled"
	var revokedAt *int64
	switch {
	case token.DeletedAt.Valid:
		status, revokedAt = "revoked", new(token.DeletedAt.Time.Unix())
	case token.Status == common.TokenStatusDisabled:
	case token.Status == common.TokenStatusExpired || token.ExpiredTime != -1 && token.ExpiredTime <= now:
		status = "expired"
	case token.Status == common.TokenStatusExhausted || token.Status == common.TokenStatusEnabled && !token.UnlimitedQuota && token.RemainQuota <= 0:
		status = "exhausted"
	case token.Status == common.TokenStatusEnabled:
		status = "active"
	}
	autoGroups, _ := token.GetAutoGroups()
	return managedTokenResponse{
		ID: token.Id, Name: token.Name, MaskedKey: token.GetMaskedKey(), Status: status, Version: version,
		CreatedTime: token.CreatedTime, ExpiredTime: token.ExpiredTime, RevokedAt: revokedAt,
		RemainQuota: token.RemainQuota, UsedQuota: token.UsedQuota, UnlimitedQuota: token.UnlimitedQuota,
		ModelLimitsEnabled: token.ModelLimitsEnabled, ModelLimits: token.ModelLimits, AllowIps: token.GetIpLimits(),
		Group: token.Group, CrossGroupRetry: token.CrossGroupRetry, AutoGroups: autoGroups,
		AllowUserQuotaQuery: token.AllowUserQuotaQuery,
	}
}

func validManagedTokenIPs(value *string) bool {
	for _, address := range (&model.Token{AllowIps: value}).GetIpLimits() {
		if _, err := netip.ParseAddr(address); err == nil {
			continue
		}
		if _, err := netip.ParsePrefix(address); err != nil {
			return false
		}
	}
	return true
}

func searchManagedTokens(c *gin.Context) {
	if !requireManagedTokenAccess(c) {
		return
	}
	var filter struct {
		Keyword string `form:"keyword" binding:"max=128"`
		Status  string `form:"status" binding:"omitempty,oneof=active disabled expired exhausted revoked"`
		Limit   *int   `form:"limit" binding:"omitempty,min=1,max=100"`
		Cursor  string `form:"cursor" binding:"max=96"`
	}
	if c.ShouldBindQuery(&filter) != nil || c.Query("token") != "" {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	accountID := c.GetInt("id")
	limit, before := 50, 0
	if filter.Limit != nil {
		limit = *filter.Limit
	}
	sign := func(position string) string {
		return common.GenerateHMAC(fmt.Sprintf("managed-token-cursor:%d:%s:%s:%s", accountID, position, filter.Status, filter.Keyword))
	}
	if filter.Cursor != "" {
		position, signature, ok := strings.Cut(filter.Cursor, ".")
		var err error
		before, err = strconv.Atoi(position)
		if !ok || err != nil || before <= 0 || !hmac.Equal([]byte(signature), []byte(sign(position))) {
			common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
			return
		}
	}
	now := common.GetTimestamp()
	rows, err := model.ListManagedTokens(c.Request.Context(), accountID, filter.Keyword, filter.Status, before, limit+1, now)
	if err != nil {
		writeTokenManagementError(c, err)
		return
	}
	var cursor *string
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
		position := strconv.Itoa(rows[len(rows)-1].Id)
		cursor = new(position + "." + sign(position))
	}
	data := make([]managedTokenResponse, 0, len(rows))
	for i := range rows {
		data = append(data, buildManagedTokenResponse(&rows[i].Token, now, rows[i].Version))
	}
	c.JSON(http.StatusOK, gin.H{"data": data, "next_cursor": cursor, "has_more": hasMore})
}

func readManagedToken(c *gin.Context, includeRevoked bool) *model.ManagedToken {
	if !requireManagedTokenAccess(c) {
		return nil
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return nil
	}
	token, err := model.GetManagedToken(c.Request.Context(), c.GetInt("id"), id)
	if err != nil {
		writeTokenManagementError(c, err)
		return nil
	}
	if !includeRevoked && token.DeletedAt.Valid {
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Invalid parameters"))
		return nil
	}
	return token
}

func writeManagedTokenIssued(c *gin.Context, token *model.Token) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, gin.H{
		"token": buildManagedTokenResponse(token, common.GetTimestamp(), 1),
		"key":   "sk-" + token.GetFullKey(),
	})
}

func managedStatusAllowed(status string) bool {
	return slices.Contains([]string{"active", "disabled", "revoked"}, status)
}
