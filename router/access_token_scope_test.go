package router

import (
	"context"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accessTokenExemptRoutes are /api routes that never reach dashboard
// authentication, so a personal access token has no rule to satisfy there.
var accessTokenExemptRoutes = []string{
	// Anonymous pages and setup.
	"GET /api/setup",
	"POST /api/setup",
	"GET /api/status",
	"GET /api/uptime/status",
	"GET /api/notice",
	"GET /api/user-agreement",
	"GET /api/privacy-policy",
	"GET /api/about",
	"GET /api/home_page_content",
	"GET /api/ratio_config",
	"GET /api/user/groups",

	// Anonymous account entry points.
	"GET /api/verification",
	"GET /api/reset_password",
	"POST /api/user/reset",
	"POST /api/user/register",
	"GET /api/user/login/encryption-key",
	"POST /api/user/login",
	"POST /api/user/login/2fa",
	"POST /api/user/login/verify",
	"POST /api/user/login/passkey/begin",
	"POST /api/user/login/passkey/finish",
	"POST /api/user/passkey/login/begin",
	"POST /api/user/passkey/login/finish",
	"GET /api/oauth/wechat",
	"GET /api/oauth/telegram/login",
	"GET /api/oauth/telegram/bind/:flow_token",

	// Browser session cookie endpoints.
	"POST /api/user/auth/refresh",
	"POST /api/user/auth/logout",

	// Payment callbacks.
	"POST /api/stripe/webhook",
	"POST /api/creem/webhook",
	"POST /api/waffo/webhook",
	"POST /api/waffo-pancake/webhook/:env",
	"GET /api/user/epay/notify",
	"POST /api/user/epay/notify",
	"GET /api/subscription/epay/notify",
	"POST /api/subscription/epay/notify",
	"GET /api/subscription/epay/return",
	"POST /api/subscription/epay/return",

	// API key authentication (TokenAuthReadOnly).
	"GET /api/usage/token/",
	"GET /api/log/token",
}

// accessTokenHelperRoutes are declared through handlePermissionRoute outside
// channelPermissionRoutes.
var accessTokenHelperRoutes = map[string]authz.Permission{
	"GET /api/task_plugin_options": authz.TaskPluginBind,
	"GET /api/audit":               authz.AuditRead,
}

func newAccessTokenScopeTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// A permission route that is also in the static table panics here.
	require.NotPanics(t, func() {
		SetApiRouter(engine)
		SetDashboardRouter(engine)
		SetRelayRouter(engine)
		SetTaskPluginProtocolRouter(engine)
		SetVideoRouter(engine)
		SetTaskRouter(engine)
	})
	return engine
}

func TestAccessTokenRouteRulesCoverEveryDashboardRoute(t *testing.T) {
	engine := newAccessTokenScopeTestEngine(t)
	exempt := map[string]bool{}
	for _, key := range accessTokenExemptRoutes {
		exempt[key] = true
	}
	registered := map[string]bool{}
	panel := map[string]bool{}
	for _, route := range engine.Routes() {
		if !strings.HasPrefix(route.Path, "/api/") && !strings.HasPrefix(route.Path, "/pg/") {
			continue
		}
		key := route.Method + " " + route.Path
		registered[key] = true
		if !exempt[key] {
			panel[key] = true
		}
	}
	for _, key := range accessTokenExemptRoutes {
		assert.True(t, registered[key], "exempt route %s is no longer registered", key)
	}

	declared := map[string]int{}
	for _, key := range middleware.AccessTokenRouteRuleKeys() {
		declared[key]++
	}
	for key := range panel {
		assert.Equal(t, 1, declared[key], "panel route %s must be declared exactly once", key)
	}
	for key := range declared {
		assert.True(t, panel[key], "declared route %s is not a registered panel route", key)
	}

	permissionRoutes := map[string]authz.Permission{}
	for _, route := range channelPermissionRoutes {
		permissionRoutes[route.method+" "+joinPaths("/api/channel", route.path)] = route.permission
	}
	for key, permission := range accessTokenHelperRoutes {
		permissionRoutes[key] = permission
	}
	for key, permission := range permissionRoutes {
		rule, ok := middleware.AccessTokenRouteRule(key)
		require.True(t, ok, key)
		assert.Equal(t, "scope", rule.Kind(), key)
		assert.Equal(t, service.AccessTokenScopeOf(permission), rule.Scope(), key)
	}
}

func TestDashboardPerfMetricsRequiresAdminReadAccess(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.UserAccessToken{}, &model.AuditLog{}, &model.PerfMetric{}))
	engine := gin.New()
	SetApiRouter(engine)
	for _, tc := range []struct {
		name, scope string
		role, want  int
	}{
		{"guest", "", 0, http.StatusUnauthorized},
		{"member", "log:read", common.RoleCommonUser, http.StatusForbidden},
		{"admin-profile", "profile:read", common.RoleAdminUser, http.StatusForbidden},
		{"admin-log", "log:read", common.RoleAdminUser, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/data/perf-metrics/summary", nil)
			if tc.role != 0 {
				user := model.User{Username: tc.name, AffCode: tc.name, Role: tc.role, Group: "default", Status: common.UserStatusEnabled}
				require.NoError(t, model.DB.Create(&user).Error)
				suffix, err := common.GenerateRandomCharsKey(43)
				require.NoError(t, err)
				raw := model.AccessTokenPrefix + suffix
				token := model.UserAccessToken{Name: tc.name, TokenHash: model.AccessTokenFingerprint(raw), TokenHint: model.AccessTokenHint(raw)}
				require.NoError(t, token.SetScopes([]string{tc.scope}))
				require.NoError(t, model.CreateUserAccessToken(user.Id, &token, service.AccessTokenMaxPerUser))
				req.Header.Set("Authorization", "Bearer "+raw)
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			assert.Equal(t, tc.want, recorder.Code)
			assert.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
		})
	}
}

func TestManageRelayTokenRequiresWriteScopeAndOwnership(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.UserAccessToken{}, &model.AuditLog{}))
	engine := gin.New()
	SetApiRouter(engine)
	credential := func(name string, scopes []string, role int, expiry int64) (int, string) {
		t.Helper()
		user := model.User{Username: name, AffCode: name, Role: role, Group: "default", Status: common.UserStatusEnabled}
		require.NoError(t, model.DB.Create(&user).Error)
		suffix, err := common.GenerateRandomCharsKey(43)
		require.NoError(t, err)
		raw := model.AccessTokenPrefix + suffix
		credential := model.UserAccessToken{Name: name, TokenHash: model.AccessTokenFingerprint(raw), TokenHint: model.AccessTokenHint(raw), ExpiresAt: expiry}
		require.NoError(t, credential.SetScopes(scopes))
		require.NoError(t, model.CreateUserAccessToken(user.Id, &credential, service.AccessTokenMaxPerUser))
		return user.Id, raw
	}
	accountID, writer := credential("token-owner", []string{"api_key:write", "api_key:read", "api_key:reveal"}, common.RoleAdminUser, 0)
	_, foreign := credential("other-writer", []string{"api_key:write", "api_key:read"}, common.RoleAdminUser, 0)
	_, readOnly := credential("token-reader", []string{"api_key:read"}, common.RoleAdminUser, 0)
	_, expired := credential("expired-writer", []string{"api_key:write"}, common.RoleAdminUser, 1)
	_, member := credential("member-writer", []string{"api_key:write", "api_key:read"}, common.RoleCommonUser, 0)
	const relayKey = "relay-secret-for-token-management"
	token := model.Token{UserId: accountID, Key: relayKey, Name: "original", RemainQuota: 300, UsedQuota: 50, ExpiredTime: -1, AllowUserQuotaQuery: true}
	require.NoError(t, token.Insert())
	for range 2 {
		require.NoError(t, model.DB.AutoMigrate(&model.ManagedTokenRevision{}, &model.RelayIdempotency{}))
	}
	var databaseVersion string
	require.NoError(t, model.DB.Raw("SELECT sqlite_version()").Scan(&databaseVersion).Error)
	t.Logf("SQLite version %s", databaseVersion)
	path := "/api/token/" + strconv.Itoa(token.Id)
	request := func(method, path, key, payload string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if method == http.MethodPost || method == http.MethodPut {
			req.Header.Set("Idempotency-Key", fmt.Sprintf("%x", sha256.Sum256([]byte(key+payload))))
		}
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		return recorder
	}
	put := func(path, key, payload string) *httptest.ResponseRecorder {
		return request(http.MethodPut, path, key, payload)
	}
	const create = `{"name":"employee test","expired_time":-1,"remain_quota":75,"model_limits_enabled":true,"model_limits":"model-a","allow_ips":"192.0.2.0/24"}`
	for _, tc := range []struct {
		key, body string
		want      int
	}{
		{member, create, http.StatusForbidden},
		{readOnly, create, http.StatusForbidden},
		{writer, `{"name":"invalid whitelist","expired_time":-1,"allow_ips":"invalid"}`, http.StatusBadRequest},
		{writer, `{"name":"foreign owner","expired_time":-1,"user_id":2}`, http.StatusBadRequest},
	} {
		resp := request(http.MethodPost, "/api/token/", tc.key, tc.body)
		assert.Equal(t, tc.want, resp.Code, "%s", resp.Body.String())
	}
	created := request(http.MethodPost, "/api/token/", writer, create)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	assert.Equal(t, "no-store", created.Header().Get("Cache-Control"))
	var issued struct {
		Key   string `json:"key"`
		Token struct {
			ID                  int    `json:"id"`
			Status              string `json:"status"`
			MaskedKey           string `json:"masked_key"`
			RemainQuota         int    `json:"remain_quota"`
			AllowUserQuotaQuery bool   `json:"allow_user_quota_query"`
		} `json:"token"`
	}
	require.NoError(t, common.Unmarshal(created.Body.Bytes(), &issued))
	require.NotEmpty(t, issued.Key)
	assert.True(t, strings.HasPrefix(issued.Key, "sk-"))
	assert.NotEqual(t, issued.Key, issued.Token.MaskedKey)
	assert.Equal(t, 75, issued.Token.RemainQuota)
	assert.Equal(t, "active", issued.Token.Status)
	assert.False(t, issued.Token.AllowUserQuotaQuery)
	issuedPath := "/api/token/" + strconv.Itoa(issued.Token.ID)
	for _, tc := range []struct {
		method, path, key, body string
		want                    int
	}{
		{http.MethodGet, issuedPath, writer, "", http.StatusOK},
		{http.MethodGet, issuedPath, foreign, "", http.StatusNotFound},
		{http.MethodGet, issuedPath, member, "", http.StatusForbidden},
		{http.MethodGet, issuedPath, issued.Key, "", http.StatusUnauthorized},
		{http.MethodPost, issuedPath + "/key", writer, `{"reason":"private reason"}`, http.StatusForbidden},
		{http.MethodPost, "/api/token/batch/keys", writer, fmt.Sprintf(`{"ids":[%d]}`, issued.Token.ID), http.StatusForbidden},
	} {
		resp := request(tc.method, tc.path, tc.key, tc.body)
		assert.Equal(t, tc.want, resp.Code, "%s", resp.Body.String())
		assert.NotContains(t, resp.Body.String(), strings.TrimPrefix(issued.Key, "sk-"))
	}
	search := "/api/token/search?limit=1&status=active"
	first := request(http.MethodGet, search, writer, "")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var page struct {
		Data []struct {
			ID int `json:"id"`
		} `json:"data"`
		Cursor  *string `json:"next_cursor"`
		HasMore bool    `json:"has_more"`
	}
	require.NoError(t, common.Unmarshal(first.Body.Bytes(), &page))
	require.NotNil(t, page.Cursor)
	require.Len(t, page.Data, 1)
	assert.Equal(t, issued.Token.ID, page.Data[0].ID)
	assert.True(t, page.HasMore)
	cursor := url.QueryEscape(*page.Cursor)
	second := request(http.MethodGet, search+"&cursor="+cursor, writer, "")
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	require.NoError(t, common.Unmarshal(second.Body.Bytes(), &page))
	require.Len(t, page.Data, 1)
	assert.Equal(t, token.Id, page.Data[0].ID)
	assert.Nil(t, page.Cursor)
	assert.False(t, page.HasMore)
	assert.Equal(t, http.StatusBadRequest, request(http.MethodGet, search+"&cursor="+cursor, foreign, "").Code)
	assert.Equal(t, http.StatusBadRequest, request(http.MethodGet, search+"&cursor="+cursor+"x", writer, "").Code)
	const disable = `{"status":"disabled","reason":"private reason","expected_version":2}`
	for _, tc := range []struct {
		key, suffix, body string
		want              int
	}{
		{"", "/status", disable, http.StatusUnauthorized},
		{"sk-" + relayKey, "/status", disable, http.StatusUnauthorized},
		{readOnly, "/status", disable, http.StatusForbidden},
		{expired, "/status", disable, http.StatusUnauthorized},
		{foreign, "/status", disable, http.StatusNotFound},
		{foreign, "", `{"name":"foreign","expected_version":1}`, http.StatusNotFound},
		{writer, "", `{"name":"new","user_id":10,"expected_version":1}`, http.StatusBadRequest},
		{writer, "/status", `{"status":"disabled"}`, http.StatusBadRequest},
	} {
		res := put(path+tc.suffix, tc.key, tc.body)
		assert.Equal(t, tc.want, res.Code, "%s", res.Body.String())
	}
	res := put(path, writer, `{"name":"renamed","allow_user_quota_query":false,"expected_version":1}`)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	assert.NotContains(t, res.Body.String(), relayKey)
	stored, err := model.GetTokenByIds(token.Id, accountID)
	require.NoError(t, err)
	assert.Equal(t, "renamed", stored.Name)
	assert.False(t, stored.AllowUserQuotaQuery)
	assert.Equal(t, 300, stored.RemainQuota)
	assert.Equal(t, 50, stored.UsedQuota)
	stale := put(path, writer, `{"name":"stale","expected_version":1}`)
	assert.Equal(t, http.StatusConflict, stale.Code)
	assert.Contains(t, stale.Body.String(), `"version_conflict"`)

	res = put(path+"/status", writer, disable)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	assert.Contains(t, res.Body.String(), `"status":"disabled"`)
	_, err = model.ValidateUserToken(relayKey)
	require.ErrorIs(t, err, model.ErrTokenInvalid)
	require.NoError(t, model.DB.Model(stored).Update("expired_time", 1).Error)
	res = put(path+"/status", writer, `{"status":"active","reason":"ready","expected_version":3}`)
	require.Equal(t, http.StatusConflict, res.Code, res.Body.String())
	res = put(path, writer, `{"expired_time":-1,"expected_version":3}`)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	res = put(path+"/status", writer, `{"status":"active","reason":"ready","expected_version":4}`)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	cache := miniredis.RunT(t)
	previousRedis := common.RDB
	client := redis.NewClient(&redis.Options{Addr: cache.Addr()})
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = false, previousRedis
		assert.NoError(t, client.Close())
	})
	_, err = model.ValidateUserToken(relayKey)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cached, err := client.Keys(ctx, "token:*").Result()
	require.NoError(t, err)
	require.NotEmpty(t, cached)

	res = put(path+"/status", writer, `{"status":"revoked","reason":"retired","expected_version":5}`)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	assert.Contains(t, res.Body.String(), `"status":"revoked"`)
	assert.NotContains(t, res.Body.String(), relayKey)
	_, err = model.ValidateUserToken(relayKey)
	require.ErrorIs(t, err, model.ErrTokenInvalid)
	res = put(path+"/status", writer, `{"status":"active","reason":"ready","expected_version":6}`)
	require.Equal(t, http.StatusNotFound, res.Code, res.Body.String())
	revoked := request(http.MethodGet, "/api/token/search?status=revoked&keyword=original", writer, "")
	require.Equal(t, http.StatusOK, revoked.Code, revoked.Body.String())
	require.NoError(t, common.Unmarshal(revoked.Body.Bytes(), &page))
	assert.Empty(t, page.Data)
	revoked = request(http.MethodGet, "/api/token/search?status=revoked&keyword=renamed", writer, "")
	require.Equal(t, http.StatusOK, revoked.Code, revoked.Body.String())
	assert.Contains(t, revoked.Body.String(), `"status":"revoked"`)
	assert.NotContains(t, revoked.Body.String(), relayKey)

	var audits []model.AuditLog
	require.NoError(t, model.DB.Where("action = ? AND success = ?", "token.status_update", true).Find(&audits).Error)
	require.Len(t, audits, 3)
	data, err := common.Marshal(audits)
	require.NoError(t, err)
	assert.NotContains(t, string(data), relayKey)
	assert.NotContains(t, string(data), "private reason")
	var creationAudits []model.AuditLog
	require.NoError(t, model.DB.Where("action = ? AND success = ?", "token.create", true).Find(&creationAudits).Error)
	require.Len(t, creationAudits, 1)
	data, err = common.Marshal(creationAudits)
	require.NoError(t, err)
	assert.NotContains(t, string(data), strings.TrimPrefix(issued.Key, "sk-"))
	replay := request(http.MethodPost, "/api/token/", writer, create)
	assert.Equal(t, http.StatusCreated, replay.Code)
	assert.JSONEq(t, created.Body.String(), replay.Body.String())
	count, err := model.CountUserTokens(accountID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
	require.NoError(t, model.DB.AutoMigrate(&model.ManagedTokenRevision{}, &model.RelayIdempotency{}))
	storedRevision, err := model.GetManagedToken(context.Background(), accountID, token.Id)
	require.NoError(t, err)
	assert.Equal(t, 6, storedRevision.Version)
	assert.True(t, storedRevision.DeletedAt.Valid)
}

func TestUsageRecordsRequireAccountAndManagementScope(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.UserAccessToken{}, &model.AuditLog{}, &model.UsageRecord{}))
	engine := gin.New()
	engine.Use(middleware.RequestId())
	SetApiRouter(engine)

	credential := func(name, scope string, role int, expiresAt int64) (int, string) {
		t.Helper()
		user := model.User{Username: name, AffCode: name, Role: role, Group: "default", Status: common.UserStatusEnabled}
		require.NoError(t, model.DB.Create(&user).Error)
		suffix, err := common.GenerateRandomCharsKey(43)
		require.NoError(t, err)
		raw := model.AccessTokenPrefix + suffix
		token := model.UserAccessToken{Name: name, TokenHash: model.AccessTokenFingerprint(raw), TokenHint: model.AccessTokenHint(raw), ExpiresAt: expiresAt}
		require.NoError(t, token.SetScopes([]string{scope}))
		require.NoError(t, model.CreateUserAccessToken(user.Id, &token, service.AccessTokenMaxPerUser))
		return user.Id, raw
	}
	accountID, accountKey := credential("usage-owner", "usage:read", common.RoleAdminUser, 0)
	otherID, otherKey := credential("usage-other", "usage:read", common.RoleAdminUser, 0)
	_, memberKey := credential("usage-member", "usage:read", common.RoleCommonUser, 0)
	_, wrongScopeKey := credential("usage-profile", "profile:read", common.RoleAdminUser, 0)
	_, expiredKey := credential("usage-expired", "usage:read", common.RoleAdminUser, 1)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId: accountID, Key: "usage-test-relay-key", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true,
	}).Error)
	// Migrate twice over data from the original schema.
	require.NoError(t, model.DB.AutoMigrate(&model.UsageRecord{}))
	var sqliteVersion string
	require.NoError(t, model.DB.Raw("SELECT sqlite_version()").Scan(&sqliteVersion).Error)
	t.Logf("SQLite version %s", sqliteVersion)

	record := model.UsageRecord{
		AccountID: accountID, RecordID: "bill_Example", Version: 1, TokenID: 12,
		RequestID: "request_Example", Model: "ModelA", Capability: "text", Status: "settled", RequestStatus: "succeeded",
		Currency: "USD", GrossAmount: new("0.10000000"), RefundAmount: new("0.00000000"), NetAmount: new("0.10000000"),
		Quantities: model.JSONValue("[]"), Metadata: model.JSONValue("{}"), CreatedAt: 9, UpdatedAt: 10, SettledAt: new(int64(10)),
	}
	require.NoError(t, model.DB.Create(&record).Error)
	revision := record
	revision.Sequence = 0
	revision.Version = 2
	revision.Status = "refunded"
	revision.RefundAmount = new("0.10000000")
	revision.NetAmount = new("0.00000000")
	require.NoError(t, model.DB.Create(&revision).Error)
	duplicate := revision
	duplicate.Sequence = 0
	require.Error(t, model.DB.Create(&duplicate).Error)
	foreign := record
	foreign.Sequence = 0
	foreign.AccountID = otherID
	foreign.RecordID = "bill_Foreign"
	require.NoError(t, model.DB.Create(&foreign).Error)
	require.NoError(t, model.DB.AutoMigrate(&model.UsageRecord{}))

	request := func(path, key string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		return res
	}
	const list = "/api/usage/records?updated_from=10&updated_to=11&limit=1"
	const detail = "/api/usage/records/bill_Example"
	for _, tc := range []struct {
		name, path, key string
		status          int
	}{
		{"guest", list, "", 401}, {"inference credential", list, "sk-usage-test-relay-key", 401},
		{"member", list, memberKey, 403}, {"missing scope", detail, wrongScopeKey, 403},
		{"expired credential", detail, expiredKey, 401}, {"other account", detail, otherKey, 404},
		{"case-sensitive id", "/api/usage/records/bill_example", accountKey, 404},
		{"missing time range", "/api/usage/records?request_id=request_Example", accountKey, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := request(tc.path, tc.key)
			assert.Equal(t, tc.status, res.Code, res.Body.String())
			assert.Contains(t, res.Header().Get("Cache-Control"), "no-store")
		})
	}

	first := request(list, accountKey)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.NotEmpty(t, first.Header().Get("X-Request-Id"))
	var page struct {
		Data []model.UsageRecord `json:"data"`
		Next *string             `json:"next_cursor"`
		More bool                `json:"has_more"`
		Scan int64               `json:"scanned_until"`
	}
	require.NoError(t, common.Unmarshal(first.Body.Bytes(), &page))
	require.Len(t, page.Data, 1)
	assert.Equal(t, 1, page.Data[0].Version)
	assert.True(t, page.More)
	assert.EqualValues(t, 10, page.Scan)
	require.NotNil(t, page.Next)
	cursor := *page.Next

	second := request(list+"&cursor="+cursor, accountKey)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	require.NoError(t, common.Unmarshal(second.Body.Bytes(), &page))
	require.Len(t, page.Data, 1)
	assert.Equal(t, 2, page.Data[0].Version)
	assert.Nil(t, page.Next)
	assert.False(t, page.More)
	assert.EqualValues(t, 11, page.Scan)
	assert.Equal(t, http.StatusBadRequest, request(list+"&cursor="+cursor+"a", accountKey).Code)
	assert.Equal(t, http.StatusBadRequest, request(list+"&status=settled&cursor="+cursor, accountKey).Code)
	assert.Equal(t, http.StatusBadRequest, request(list+"&cursor="+cursor, otherKey).Code)

	latest := request(detail, accountKey)
	require.Equal(t, http.StatusOK, latest.Code, latest.Body.String())
	var result model.UsageRecord
	require.NoError(t, common.Unmarshal(latest.Body.Bytes(), &result))
	assert.Equal(t, 2, result.Version)
	require.NotNil(t, result.NetAmount)
	assert.Equal(t, "0.00000000", *result.NetAmount)
}

func TestAccessTokenCatalogCoversEveryPermission(t *testing.T) {
	catalog := service.AccessTokenCatalog(1, common.RoleRootUser)
	scopes := map[string]bool{}
	resources := map[string]int{}
	for _, group := range catalog {
		for _, resource := range group.Resources {
			resources[resource.Resource]++
			for _, action := range resource.Actions {
				scopes[resource.Resource+":"+action.Action] = true
			}
		}
	}
	for _, permission := range authz.AllPermissions() {
		assert.True(t, scopes[service.AccessTokenScopeOf(permission)], "root catalog lacks %s", service.AccessTokenScopeOf(permission))
	}
	for resource, count := range resources {
		assert.Equal(t, 1, count, "resource %s appears in more than one catalog entry", resource)
	}
}

func TestAccessTokenVerificationScopesAreAllListed(t *testing.T) {
	files, err := filepath.Glob("../service/*.go")
	require.NoError(t, err)
	fileSet := token.NewFileSet()
	var scopes []string
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fileSet, file, nil, 0)
		require.NoError(t, err)
		for _, declaration := range parsed.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, spec := range general.Specs {
				value := spec.(*ast.ValueSpec)
				for i, name := range value.Names {
					if !strings.HasPrefix(name.Name, "VerificationScope") || i >= len(value.Values) {
						continue
					}
					literal, ok := value.Values[i].(*ast.BasicLit)
					require.True(t, ok, name.Name)
					scope, err := strconv.Unquote(literal.Value)
					require.NoError(t, err)
					scopes = append(scopes, scope)
				}
			}
		}
	}
	require.NotEmpty(t, scopes)
	for _, scope := range scopes {
		_, listed := service.AccessTokenVerificationScope(scope)
		assert.True(t, listed, "verification scope %s has no access token rule", scope)
	}
}
