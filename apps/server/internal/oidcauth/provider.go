package oidcauth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	dsmodel "interview_ng/internal/model"
)

// providerCacheTTL 发现文档与 provider 的缓存时长（配置保存成功后主动失效）。
const providerCacheTTL = 10 * time.Minute

// Identity 是校验通过后的 IdP 身份（含全部 ID token 声明，供 JMESPath 求值）。
type Identity struct {
	Subject string
	Claims  map[string]any
}

// ProbeResult 是连通性检测结果（仅暴露只读端点，供界面展示）。
type ProbeResult struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURL               string `json:"jwks_uri"`
}

var (
	ErrNotConfigured   = errors.New("oidc not configured")
	ErrNotEnabled      = errors.New("oidc not enabled")
	ErrDiscoveryFailed = errors.New("oidc discovery failed")
	ErrStateInvalid    = errors.New("oidc state invalid or expired")
	ErrExchangeFailed  = errors.New("oidc code exchange failed")
	ErrNonceInvalid    = errors.New("oidc nonce mismatch")
	ErrClaimsInvalid   = errors.New("oidc id token claims invalid")
)

type cachedProvider struct {
	provider  *oidc.Provider
	probe     ProbeResult
	fetchedAt time.Time
}

// Service 承载 OIDC 发现、授权跳转、回调换取与连通性探测。
// provider 缓存（issuer → provider）与流程/登录码同为单进程内存态。
type Service struct {
	flows *FlowStore

	mu        sync.Mutex
	providers map[string]*cachedProvider
}

// New 构建 OIDC 服务。
func New() *Service {
	return &Service{flows: NewFlowStore(), providers: map[string]*cachedProvider{}}
}

// providerFor 返回 issuer 的 provider（缓存 10 分钟），发现失败 → ErrDiscoveryFailed。
func (s *Service) providerFor(ctx context.Context, issuer string) (*cachedProvider, error) {
	s.mu.Lock()
	cp, ok := s.providers[issuer]
	if ok && time.Since(cp.fetchedAt) < providerCacheTTL {
		s.mu.Unlock()
		return cp, nil
	}
	s.mu.Unlock()

	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDiscoveryFailed, err)
	}
	var meta struct {
		Issuer                string `json:"issuer"`
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
		JWKSURL               string `json:"jwks_uri"`
	}
	if err := p.Claims(&meta); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDiscoveryFailed, err)
	}
	cp = &cachedProvider{
		provider: p,
		probe: ProbeResult{
			Issuer:                meta.Issuer,
			AuthorizationEndpoint: meta.AuthorizationEndpoint,
			TokenEndpoint:         meta.TokenEndpoint,
			JWKSURL:               meta.JWKSURL,
		},
		fetchedAt: time.Now(),
	}
	s.mu.Lock()
	s.providers[issuer] = cp
	s.mu.Unlock()
	return cp, nil
}

// oauth2Config 组装授权配置（PKCE S256 始终启用）。
func oauth2Config(cfg *dsmodel.OidcConfig, ep oauth2.Endpoint) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     ep,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       dsmodel.ParseScopes(cfg.Scopes),
	}
}

// AuthorizeURL 生成跳转地址：发现 → 生成 nonce/verifier/state → 返回 IdP 授权 URL。
func (s *Service) AuthorizeURL(ctx context.Context, cfg *dsmodel.OidcConfig) (string, error) {
	if cfg == nil || cfg.Issuer == "" || cfg.ClientID == "" {
		return "", ErrNotConfigured
	}
	if !cfg.Enabled {
		return "", ErrNotEnabled
	}
	cp, err := s.providerFor(ctx, cfg.Issuer)
	if err != nil {
		return "", err
	}
	verifier := oauth2.GenerateVerifier()
	nonce := randomToken()
	state := s.flows.Start(nonce, verifier)
	return oauth2Config(cfg, cp.provider.Endpoint()).
		AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), nil
}

// Exchange 用回调的 code+state 换 token 并校验 ID token：
// state 一次性（Take）、签名/iss/aud/exp（go-oidc Verify）、nonce 手工比对（库不做该校验）。
func (s *Service) Exchange(ctx context.Context, cfg *dsmodel.OidcConfig, state, code string) (*Identity, error) {
	if cfg == nil || cfg.Issuer == "" || cfg.ClientID == "" {
		return nil, ErrNotConfigured
	}
	if !cfg.Enabled {
		return nil, ErrNotEnabled
	}
	flow, ok := s.flows.Take(state)
	if !ok {
		return nil, ErrStateInvalid
	}
	cp, err := s.providerFor(ctx, cfg.Issuer)
	if err != nil {
		return nil, err
	}
	tok, err := oauth2Config(cfg, cp.provider.Endpoint()).
		Exchange(ctx, code, oauth2.VerifierOption(flow.Verifier))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrExchangeFailed, err)
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok || raw == "" {
		return nil, ErrClaimsInvalid
	}
	idToken, err := cp.provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}).Verify(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrExchangeFailed, err)
	}
	if idToken.Nonce != flow.Nonce {
		return nil, ErrNonceInvalid
	}
	if idToken.Subject == "" {
		return nil, ErrClaimsInvalid
	}
	claims := map[string]any{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, ErrClaimsInvalid
	}
	return &Identity{Subject: idToken.Subject, Claims: claims}, nil
}

// Probe 拉取发现文档并返回端点摘要（保存前自检）。
func (s *Service) Probe(ctx context.Context, issuer string) (*ProbeResult, error) {
	cp, err := s.providerFor(ctx, issuer)
	if err != nil {
		return nil, err
	}
	res := cp.probe
	return &res, nil
}

// Invalidate 清空 provider 缓存（配置保存成功后调用）。
func (s *Service) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers = map[string]*cachedProvider{}
}

// IssueLoginCode 登记一次性登录码（回调成功后半程，避免 token 出现在 URL）。
func (s *Service) IssueLoginCode(userID uint64, token string) string {
	return s.flows.IssueLoginCode(userID, token)
}

// RedeemLoginCode 兑换并删除登录码（一次性）。
func (s *Service) RedeemLoginCode(code string) (uint64, string, bool) {
	return s.flows.RedeemLoginCode(code)
}
