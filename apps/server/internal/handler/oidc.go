package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"interview_ng/internal/auth"
	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/oidcauth"
	"interview_ng/internal/state"
)

//---- 登录方式 & 单点登录（OIDC）公开端点 ----

// getAuthentication 返回可用的登录方式（登录页据此渲染入口）。
// 配置读取失败一律按「OIDC 未启用」返回：登录页不出按钮，也不向未登录用户泄露内部错误。
func (h *HTTPServer) getAuthentication(c *gin.Context) {
	enabled := false
	if cfg, err := h.svc.GetOidcConfig(c.Request.Context()); err == nil {
		enabled = cfg.Enabled
	}
	c.JSON(http.StatusOK, gin.H{"password": true, "oidc": gin.H{"enabled": enabled}})
}

// startOidcAuthorization 生成 IdP 授权跳转：整页跳转到本端点即可（无 XHR）。
func (h *HTTPServer) startOidcAuthorization(c *gin.Context) {
	cfg, err := h.svc.GetOidcConfig(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	origin := spaOrigin(cfg.RedirectURL)
	if origin == "" {
		// 回调地址未配置完整 → 无从跳回前端，只能直接报错。
		c.JSON(http.StatusBadRequest, gin.H{"error": "统一身份认证未配置完成"})
		return
	}
	if !cfg.Enabled {
		h.redirectOidcError(c, origin, "oidc_not_configured")
		return
	}
	target, err := h.oidc.AuthorizeURL(c.Request.Context(), cfg)
	if err != nil {
		h.redirectOidcError(c, origin, oidcErrorCode(err))
		return
	}
	c.Redirect(http.StatusFound, target)
}

// oidcCallback IdP 回调落地：换 token → 校验 ID token → 命中角色规则 →
// 建号/同步 → 签发 JWT → 302 回前端并携带一次性登录码（token 不出现在 URL 与浏览器历史）。
func (h *HTTPServer) oidcCallback(c *gin.Context) {
	cfg, err := h.svc.GetOidcConfig(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	origin := spaOrigin(cfg.RedirectURL)
	if origin == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "统一身份认证未配置完成"})
		return
	}
	if c.Query("error") != "" {
		h.redirectOidcError(c, origin, "oidc_login_failed")
		return
	}
	if !cfg.Enabled {
		h.redirectOidcError(c, origin, "oidc_not_configured")
		return
	}
	code, stateParam := c.Query("code"), c.Query("state")
	if code == "" || stateParam == "" {
		h.redirectOidcError(c, origin, "oidc_state_invalid")
		return
	}
	id, err := h.oidc.Exchange(c.Request.Context(), cfg, stateParam, code)
	if err != nil {
		h.redirectOidcError(c, origin, oidcErrorCode(err))
		return
	}
	roleID, hit, err := oidcauth.MatchRole(cfg.Rules, id.Claims)
	if err != nil {
		h.redirectOidcError(c, origin, "oidc_login_failed")
		return
	}
	if !hit {
		h.redirectOidcError(c, origin, "oidc_role_unmapped")
		return
	}
	principal := auth.OidcPrincipal{
		Subject:  id.Subject,
		Username: oidcauth.DeriveUsername(id.Claims, id.Subject),
		Name:     oidcauth.ClaimString(id.Claims, "name"),
		RoleIDs:  []uint64{roleID},
	}
	token, uid, _, _, err := h.auth.LoginWithOidc(c.Request.Context(), principal, cfg.AutoProvision)
	if err != nil {
		h.redirectOidcError(c, origin, oidcErrorCode(err))
		return
	}
	loginCode := h.oidc.IssueLoginCode(uid, token)
	c.Redirect(http.StatusFound, origin+"/login?oidc_code="+url.QueryEscape(loginCode))
}

type oidcSessionReq struct {
	Code string `json:"code"`
}

// createOidcSession 用一次性登录码换取会话（响应与密码登录完全一致）。
func (h *HTTPServer) createOidcSession(c *gin.Context) {
	var req oidcSessionReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	uid, token, ok := h.oidc.RedeemLoginCode(req.Code)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录已失效，请重新登录"})
		return
	}
	cache := h.auth.Cache()
	h.writeSession(c, token, uid, cache.UserRoles(uid), cache.UserPermissions(uid))
}

//---- 单点登录配置（users.manage） ----

type oidcRuleReq struct {
	Expression string `json:"expression"`
	RoleID     uint64 `json:"role_id"`
}

type putOidcConfigReq struct {
	Enabled       bool          `json:"enabled"`
	Issuer        string        `json:"issuer"`
	ClientID      string        `json:"client_id"`
	ClientSecret  *string       `json:"client_secret"` // 省略/null=保持；""=清除；非空=覆盖
	Scopes        string        `json:"scopes"`
	RedirectURL   string        `json:"redirect_url"`
	AutoProvision bool          `json:"auto_provision"`
	Rules         []oidcRuleReq `json:"rules"`
}

type oidcRuleResp struct {
	ID         uint64 `json:"id"`
	Position   int    `json:"position"`
	Expression string `json:"expression"`
	RoleID     uint64 `json:"role_id"`
}

// oidcConfigResp 是配置的读取响应：明文密钥永不下发，只回是否已配置。
type oidcConfigResp struct {
	ID              uint64         `json:"id"`
	Enabled         bool           `json:"enabled"`
	Issuer          string         `json:"issuer"`
	ClientID        string         `json:"client_id"`
	ClientSecretSet bool           `json:"client_secret_set"`
	Scopes          string         `json:"scopes"`
	RedirectURL     string         `json:"redirect_url"`
	AutoProvision   bool           `json:"auto_provision"`
	Rules           []oidcRuleResp `json:"rules"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func (h *HTTPServer) getOidcConfig(c *gin.Context) {
	cfg, err := h.svc.GetOidcConfig(c.Request.Context())
	if err != nil {
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	rules := make([]oidcRuleResp, 0, len(cfg.Rules))
	for _, r := range cfg.Rules {
		rules = append(rules, oidcRuleResp{ID: r.ID, Position: r.Position, Expression: r.Expression, RoleID: r.RoleID})
	}
	c.JSON(http.StatusOK, oidcConfigResp{
		ID:              cfg.ID,
		Enabled:         cfg.Enabled,
		Issuer:          cfg.Issuer,
		ClientID:        cfg.ClientID,
		ClientSecretSet: cfg.ClientSecret != "",
		Scopes:          cfg.Scopes,
		RedirectURL:     cfg.RedirectURL,
		AutoProvision:   cfg.AutoProvision,
		Rules:           rules,
		UpdatedAt:       cfg.UpdatedAt,
	})
}

func (h *HTTPServer) putOidcConfig(c *gin.Context) {
	var req putOidcConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	scopes := strings.TrimSpace(req.Scopes)
	if scopes == "" {
		scopes = dsmodel.DefaultOidcScopes
	}
	cfg := &dsmodel.OidcConfig{
		Enabled:       req.Enabled,
		Issuer:        strings.TrimSpace(req.Issuer),
		ClientID:      strings.TrimSpace(req.ClientID),
		Scopes:        scopes,
		RedirectURL:   strings.TrimSpace(req.RedirectURL),
		AutoProvision: req.AutoProvision,
		Rules:         make([]dsmodel.OidcRoleRule, 0, len(req.Rules)),
	}
	for i, r := range req.Rules {
		cfg.Rules = append(cfg.Rules, dsmodel.OidcRoleRule{Position: i, Expression: r.Expression, RoleID: r.RoleID})
	}
	if err := h.svc.SetOidcConfig(c.Request.Context(), cfg, req.ClientSecret); err != nil {
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	h.oidc.Invalidate()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type probeOidcReq struct {
	Issuer string `json:"issuer"`
}

// probeOidc 拉取发现文档并回显端点摘要（保存前自检连通性）。
func (h *HTTPServer) probeOidc(c *gin.Context) {
	var req probeOidcReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Issuer) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	res, err := h.oidc.Probe(c.Request.Context(), strings.TrimSpace(req.Issuer))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法获取 OIDC 发现文档：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

//---- helpers ----

// spaOrigin 从回调地址推导前端来源（scheme://host）；无法解析 → ""。
func spaOrigin(redirectURL string) string {
	u, err := url.Parse(redirectURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// redirectOidcError 回调/授权失败统一跳回登录页，用错误码让前端展示中文原因。
func (h *HTTPServer) redirectOidcError(c *gin.Context, origin, code string) {
	c.Redirect(http.StatusFound, origin+"/login?oidc_error="+url.QueryEscape(code))
}

// oidcErrorCode 把内部错误映射为前端可展示的错误码。
func oidcErrorCode(err error) string {
	switch {
	case errors.Is(err, oidcauth.ErrNotConfigured), errors.Is(err, oidcauth.ErrNotEnabled):
		return "oidc_not_configured"
	case errors.Is(err, oidcauth.ErrDiscoveryFailed):
		return "oidc_discovery_failed"
	case errors.Is(err, oidcauth.ErrStateInvalid):
		return "oidc_state_invalid"
	case errors.Is(err, oidcauth.ErrExchangeFailed):
		return "oidc_exchange_failed"
	case errors.Is(err, oidcauth.ErrNonceInvalid):
		return "oidc_nonce_invalid"
	case errors.Is(err, oidcauth.ErrClaimsInvalid):
		return "oidc_claims_invalid"
	case errors.Is(err, auth.ErrOidcRoleUnmapped):
		return "oidc_role_unmapped"
	case errors.Is(err, auth.ErrOidcUserNotFound):
		return "oidc_user_unknown"
	}
	var se *state.Error
	if errors.As(err, &se) && se.Code == "username_taken" {
		return "oidc_username_taken"
	}
	return "oidc_login_failed"
}
