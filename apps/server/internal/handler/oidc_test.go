package handler_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

//---- 测试辅助 ----

// rawRequest 发起一次请求并返回原始 recorder（doJSON 不暴露响应头/原始体）。
func rawRequest(t *testing.T, r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != "" {
		buf.WriteString(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// rawGET 无体 GET（读 302 的 Location 用）。
func rawGET(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	return rawRequest(t, r, "GET", path, "", "")
}

// findRole 按角色名查 id（测试辅助）。
func findRole(t *testing.T, r *gin.Engine, token, name string) int {
	t.Helper()
	code, out := doJSON(t, r, "GET", "/api/roles", "", token)
	if code != http.StatusOK {
		t.Fatalf("角色列表: %d %v", code, out)
	}
	for _, item := range out["items"].([]any) {
		m, _ := item.(map[string]any)
		if m["name"] == name {
			return int(m["id"].(float64))
		}
	}
	t.Fatalf("角色 %s 未找到", name)
	return 0
}

// fakeIdp 是最小可用的 OIDC 提供方：发现文档 + JWKS + 授权 + token 端点（RS256 真签名）。
type fakeIdp struct {
	server   *httptest.Server
	clientID string
	groups   []string

	lastNonce    string
	lastVerifier string // 证明 PKCE 的 code_verifier 真的发到了 token 端点
}

func newFakeIdp(t *testing.T, clientID string) *fakeIdp {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("生成 RSA 密钥: %v", err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: priv},
		(&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatalf("构建签名器: %v", err)
	}

	f := &fakeIdp{clientID: clientID, groups: []string{"interview-interviewers"}}
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                f.server.URL,
			"authorization_endpoint":                f.server.URL + "/authorize",
			"token_endpoint":                        f.server.URL + "/token",
			"jwks_uri":                              f.server.URL + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key: &priv.PublicKey, KeyID: "test-key", Algorithm: "RS256", Use: "sig",
		}}})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f.lastNonce = q.Get("nonce")
		u, err := url.Parse(q.Get("redirect_uri"))
		if err != nil {
			http.Error(w, "bad redirect_uri", http.StatusBadRequest)
			return
		}
		qq := u.Query()
		qq.Set("code", "test-code")
		qq.Set("state", q.Get("state"))
		u.RawQuery = qq.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.lastVerifier = r.Form.Get("code_verifier")
		if r.Form.Get("code") != "test-code" {
			http.Error(w, "bad code", http.StatusBadRequest)
			return
		}
		now := time.Now()
		tok, err := jwt.Signed(signer).Claims(jwt.Claims{
			Issuer:   f.server.URL,
			Audience: jwt.Audience{clientID},
			Subject:  "oidc-1",
			IssuedAt: jwt.NewNumericDate(now),
			Expiry:   jwt.NewNumericDate(now.Add(time.Hour)),
		}).Claims(map[string]any{
			"nonce":              f.lastNonce,
			"groups":             f.groups,
			"preferred_username": "oidc-user",
			"name":               "OIDC 面试官",
		}).Serialize()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": tok})
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// oidcRedirect 是我们服务端 302 到假 IdP 的授权地址 + 假 IdP 回调带回的 code/state。
type oidcRedirect struct {
	authorizeURL string
	code         string
	state        string
}

// startOidc 走完「我方 /api/oidc/authorization → 假 IdP /authorize」，返回回调参数。
func startOidc(t *testing.T, r *gin.Engine, idp *fakeIdp) oidcRedirect {
	t.Helper()
	w := rawGET(t, r, "/api/oidc/authorization")
	if w.Code != http.StatusFound {
		t.Fatalf("授权跳转应 302: %d %s", w.Code, w.Body.String())
	}
	authorizeURL := w.Header().Get("Location")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(authorizeURL)
	if err != nil {
		t.Fatalf("请求假 IdP authorize: %v", err)
	}
	defer resp.Body.Close()
	cb, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("解析回调地址: %v", err)
	}
	return oidcRedirect{authorizeURL: authorizeURL, code: cb.Query().Get("code"), state: cb.Query().Get("state")}
}

// oidcConfigBody 组装启用态配置请求体。
func oidcConfigBody(issuer, rules string) string {
	return `{"enabled":true,"issuer":"` + issuer + `","client_id":"interview-ng","client_secret":"s3cret",` +
		`"scopes":"openid profile email","redirect_url":"http://app.example/api/oidc/sessions",` +
		`"auto_provision":true,"rules":[` + rules + `]}`
}

//---- 用例 ----

// TestAuthenticationOptions 公共登录方式开关：未配置 → 仅密码；启用后 → OIDC 可见。
func TestAuthenticationOptions(t *testing.T) {
	r := newTestApp(t)

	code, out := doJSON(t, r, "GET", "/api/authentication", "", "")
	if code != http.StatusOK || out["password"] != true {
		t.Fatalf("未配置时应仅密码登录: %d %v", code, out)
	}
	if oidc, _ := out["oidc"].(map[string]any); oidc["enabled"] != false {
		t.Fatalf("未配置时 oidc.enabled 应为 false: %v", out)
	}

	admin := adminToken(t, r)
	if code, out := doJSON(t, r, "PUT", "/api/oidc/config",
		oidcConfigBody("https://sso.example.com/realms/interview", ""), admin); code != http.StatusOK {
		t.Fatalf("启用 OIDC 配置: %d %v", code, out)
	}
	if _, out := doJSON(t, r, "GET", "/api/authentication", "", ""); out["oidc"].(map[string]any)["enabled"] != true {
		t.Fatalf("启用后 oidc.enabled 应为 true: %v", out)
	}
}

// TestOidcConfigAdminOnlyAndSecretMasking 配置读写需 users.manage，且明文密钥永不下发。
func TestOidcConfigAdminOnlyAndSecretMasking(t *testing.T) {
	r := newTestApp(t)
	admin := adminToken(t, r)
	itvRole := findRole(t, r, admin, "interviewer")
	doJSON(t, r, "POST", "/api/users",
		`{"username":"itv9","name":"无权","password":"pass","role_ids":[`+itoa(itvRole)+`]}`, admin)
	_, sess := doJSON(t, r, "POST", "/api/sessions", `{"username":"itv9","password":"pass"}`, "")
	itv, _ := sess["token"].(string)
	if itv == "" {
		t.Fatalf("面试官登录失败: %v", sess)
	}

	body := oidcConfigBody("https://sso.example.com/realms/interview", "")
	if code, _ := doJSON(t, r, "PUT", "/api/oidc/config", body, itv); code != http.StatusForbidden {
		t.Fatalf("无 users.manage 应 403: %d", code)
	}
	if code, _ := doJSON(t, r, "GET", "/api/oidc/config", "", itv); code != http.StatusForbidden {
		t.Fatalf("无 users.manage 读取应 403: %d", code)
	}

	if code, out := doJSON(t, r, "PUT", "/api/oidc/config", body, admin); code != http.StatusOK {
		t.Fatalf("保存配置: %d %v", code, out)
	}
	w := rawRequest(t, r, "GET", "/api/oidc/config", "", admin)
	if w.Code != http.StatusOK {
		t.Fatalf("读取配置: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "s3cret") {
		t.Fatalf("明文密钥不应下发: %s", w.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if _, has := got["client_secret"]; has {
		t.Fatalf("响应不应含 client_secret 键: %s", w.Body.String())
	}
	if got["client_secret_set"] != true {
		t.Fatalf("client_secret_set 应为 true: %v", got)
	}

	// 省略 client_secret → 保持原密钥
	keep := `{"enabled":true,"issuer":"https://sso.example.com/realms/interview","client_id":"interview-ng",` +
		`"scopes":"openid profile email","redirect_url":"http://app.example/api/oidc/sessions","auto_provision":true,"rules":[]}`
	if code, out := doJSON(t, r, "PUT", "/api/oidc/config", keep, admin); code != http.StatusOK {
		t.Fatalf("省略密钥保存: %d %v", code, out)
	}
	if _, out := doJSON(t, r, "GET", "/api/oidc/config", "", admin); out["client_secret_set"] != true {
		t.Fatalf("省略密钥应保持原值: %v", out)
	}

	// 传 ""（显式 null/空）→ 清除
	clear := strings.Replace(keep, `"client_id"`, `"client_secret":"","client_id"`, 1)
	if code, out := doJSON(t, r, "PUT", "/api/oidc/config", clear, admin); code != http.StatusOK {
		t.Fatalf("清除密钥: %d %v", code, out)
	}
	if _, out := doJSON(t, r, "GET", "/api/oidc/config", "", admin); out["client_secret_set"] != false {
		t.Fatalf("空串应清除密钥: %v", out)
	}
}

// TestOidcConfigValidation 规则表达式与目标角色的校验错误（400 + 中文原因）。
func TestOidcConfigValidation(t *testing.T) {
	r := newTestApp(t)
	admin := adminToken(t, r)

	code, out := doJSON(t, r, "PUT", "/api/oidc/config",
		oidcConfigBody("https://sso.example.com/realms/interview",
			`{"expression":"groups[","role_id":1}`), admin)
	if code != http.StatusBadRequest || !strings.Contains(out["error"].(string), "第 1 条规则") {
		t.Fatalf("非法表达式应 400 且指明第 1 条: %d %v", code, out)
	}

	code, out = doJSON(t, r, "PUT", "/api/oidc/config",
		oidcConfigBody("https://sso.example.com/realms/interview",
			`{"expression":"@","role_id":99999}`), admin)
	if code != http.StatusBadRequest || !strings.Contains(out["error"].(string), "目标角色不存在") {
		t.Fatalf("未知角色应 400 且指明目标角色: %d %v", code, out)
	}
}

// TestOidcLoginEndToEnd 全链路：授权（PKCE S256 + nonce）→ 假 IdP 回调 →
// 规则命中 → 自动开通 → 一次性登录码换 JWT → /api/me 可见角色。
func TestOidcLoginEndToEnd(t *testing.T) {
	r := newTestApp(t)
	admin := adminToken(t, r)
	idp := newFakeIdp(t, "interview-ng")
	itvRole := findRole(t, r, admin, "interviewer")

	body := oidcConfigBody(idp.server.URL,
		`{"expression":"groups[?@ == 'interview-interviewers'] | [0]","role_id":`+itoa(itvRole)+`}`)
	if code, out := doJSON(t, r, "PUT", "/api/oidc/config", body, admin); code != http.StatusOK {
		t.Fatalf("保存配置: %d %v", code, out)
	}

	start := startOidc(t, r, idp)
	au, err := url.Parse(start.authorizeURL)
	if err != nil {
		t.Fatalf("解析授权地址: %v", err)
	}
	q := au.Query()
	if !strings.Contains(au.Host, "127.0.0.1") {
		t.Fatalf("应跳转到假 IdP: %s", start.authorizeURL)
	}
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("授权地址应带 PKCE S256: %s", start.authorizeURL)
	}
	if q.Get("nonce") == "" || q.Get("state") == "" {
		t.Fatalf("授权地址应带 nonce/state: %s", start.authorizeURL)
	}
	if idp.lastNonce != q.Get("nonce") {
		t.Fatalf("nonce 应原样发往 IdP: %q vs %q", idp.lastNonce, q.Get("nonce"))
	}

	// 回调 → 302 回前端并携带一次性登录码
	w := rawGET(t, r, "/api/oidc/sessions?code="+start.code+"&state="+start.state)
	if w.Code != http.StatusFound {
		t.Fatalf("回调应 302: %d %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "http://app.example/login?oidc_code=") {
		t.Fatalf("回调应跳回登录页并带登录码: %s", loc)
	}
	cb, _ := url.Parse(loc)
	loginCode := cb.Query().Get("oidc_code")
	if loginCode == "" {
		t.Fatalf("登录码缺失: %s", loc)
	}
	if idp.lastVerifier == "" {
		t.Fatalf("token 端点应收到 PKCE code_verifier")
	}

	// 换取会话
	code, out := doJSON(t, r, "POST", "/api/oidc/sessions", `{"code":"`+loginCode+`"}`, "")
	if code != http.StatusOK {
		t.Fatalf("换取会话: %d %v", code, out)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatalf("会话 token 缺失: %v", out)
	}
	user, _ := out["user"].(map[string]any)
	if user["username"] != "oidc-user" || user["name"] != "OIDC 面试官" {
		t.Fatalf("自动开通的用户资料不符: %v", user)
	}
	roles, _ := out["roles"].([]any)
	if len(roles) != 1 || roles[0] != "interviewer" {
		t.Fatalf("角色应为命中规则的 interviewer: %v", out["roles"])
	}
	if perms, _ := out["permissions"].([]any); len(perms) == 0 {
		t.Fatalf("权限应已由 RBAC 缓存解析: %v", out["permissions"])
	}
	firstID := int(user["id"].(float64))

	// 签发的 JWT 可用于受保护接口，且角色一致
	code, me := doJSON(t, r, "GET", "/api/me", "", token)
	if code != http.StatusOK {
		t.Fatalf("OIDC 签发的 token 应可用: %d %v", code, me)
	}
	if meRoles, _ := me["roles"].([]any); len(meRoles) != 1 || meRoles[0] != "interviewer" {
		t.Fatalf("/api/me 角色不符: %v", me["roles"])
	}

	// 登录码与 state 均为一次性
	if code, _ := doJSON(t, r, "POST", "/api/oidc/sessions", `{"code":"`+loginCode+`"}`, ""); code != http.StatusUnauthorized {
		t.Fatalf("登录码应一次性: %d", code)
	}
	if w := rawGET(t, r, "/api/oidc/sessions?code="+start.code+"&state="+start.state); !strings.Contains(w.Header().Get("Location"), "oidc_error=oidc_state_invalid") {
		t.Fatalf("state 应一次性: %s", w.Header().Get("Location"))
	}

	// 同一 sub 再次登录：不重复建号（同 id），且旧 token 仍有效
	start2 := startOidc(t, r, idp)
	w2 := rawGET(t, r, "/api/oidc/sessions?code="+start2.code+"&state="+start2.state)
	cb2, _ := url.Parse(w2.Header().Get("Location"))
	code, again := doJSON(t, r, "POST", "/api/oidc/sessions", `{"code":"`+cb2.Query().Get("oidc_code")+`"}`, "")
	if code != http.StatusOK {
		t.Fatalf("二次登录: %d %v", code, again)
	}
	if id := int(again["user"].(map[string]any)["id"].(float64)); id != firstID {
		t.Fatalf("同一 sub 应复用既有账号: %d vs %d", id, firstID)
	}
}

// TestOidcLoginUnmappedRole 未命中任何规则 → 拒绝登录（决策 3）。
func TestOidcLoginUnmappedRole(t *testing.T) {
	r := newTestApp(t)
	admin := adminToken(t, r)
	idp := newFakeIdp(t, "interview-ng")
	itvRole := findRole(t, r, admin, "interviewer")

	body := oidcConfigBody(idp.server.URL, `{"expression":"groups[?@ == 'nobody'] | [0]","role_id":`+itoa(itvRole)+`}`)
	if code, out := doJSON(t, r, "PUT", "/api/oidc/config", body, admin); code != http.StatusOK {
		t.Fatalf("保存配置: %d %v", code, out)
	}
	start := startOidc(t, r, idp)
	w := rawGET(t, r, "/api/oidc/sessions?code="+start.code+"&state="+start.state)
	if !strings.Contains(w.Header().Get("Location"), "oidc_error=oidc_role_unmapped") {
		t.Fatalf("未映射角色应拒绝并回错误码: %s", w.Header().Get("Location"))
	}
}

// TestOidcProbe 连通性检测：可达 → 端点摘要；不可达 → 400。
func TestOidcProbe(t *testing.T) {
	r := newTestApp(t)
	admin := adminToken(t, r)
	idp := newFakeIdp(t, "interview-ng")

	code, out := doJSON(t, r, "POST", "/api/oidc/probes", `{"issuer":"`+idp.server.URL+`"}`, admin)
	if code != http.StatusOK {
		t.Fatalf("探测假 IdP: %d %v", code, out)
	}
	if out["authorization_endpoint"] != idp.server.URL+"/authorize" ||
		out["token_endpoint"] != idp.server.URL+"/token" ||
		out["jwks_uri"] != idp.server.URL+"/keys" {
		t.Fatalf("发现文档端点不符: %v", out)
	}

	code, out = doJSON(t, r, "POST", "/api/oidc/probes", `{"issuer":"http://127.0.0.1:1/nope"}`, admin)
	if code != http.StatusBadRequest || !strings.Contains(out["error"].(string), "无法获取 OIDC 发现文档") {
		t.Fatalf("不可达应 400: %d %v", code, out)
	}
}
