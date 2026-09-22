package oidcauth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// 流程与登录码的有效期（单进程内存态：后端重启后在途登录失效，用户重新发起即可）。
const (
	flowTTL      = 10 * time.Minute
	loginCodeTTL = 60 * time.Second
	flowStoreMax = 4096
)

// Flow 是一次授权码流程的服务端状态（state → nonce/PKCE verifier）。
type Flow struct {
	Nonce     string
	Verifier  string
	ExpiresAt time.Time
}

// loginCode 是一次性登录码（回调落地页用它换取会话，避免 token 出现在 URL）。
type loginCode struct {
	UserID    uint64
	Token     string
	ExpiresAt time.Time
}

// FlowStore 保存在途授权流程与一次性登录码（带容量上限与惰性过期清理，无后台 goroutine）。
type FlowStore struct {
	mu    sync.Mutex
	flows map[string]Flow
	codes map[string]loginCode
}

// NewFlowStore 构建空的流程存储。
func NewFlowStore() *FlowStore {
	return &FlowStore{flows: map[string]Flow{}, codes: map[string]loginCode{}}
}

// randomToken 生成 32 字节随机数的 base64url 串（crypto/rand 在现代平台不会失败）。
func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// pruneLocked 删除已过期条目；仍需腾位时丢弃最早到期者（锁内调用）。
func (f *FlowStore) pruneLocked(now time.Time) {
	for k, v := range f.flows {
		if now.After(v.ExpiresAt) {
			delete(f.flows, k)
		}
	}
	for k, v := range f.codes {
		if now.After(v.ExpiresAt) {
			delete(f.codes, k)
		}
	}
}

// ensureRoomLocked 在写入前腾出容量（锁内调用）。
func (f *FlowStore) ensureRoomLocked(now time.Time) {
	f.pruneLocked(now)
	for len(f.flows) >= flowStoreMax {
		dropEarliestFlow(f.flows)
	}
	for len(f.codes) >= flowStoreMax {
		dropEarliestCode(f.codes)
	}
}

func dropEarliestFlow(m map[string]Flow) {
	oldest, has := "", false
	for k, v := range m {
		if !has || v.ExpiresAt.Before(m[oldest].ExpiresAt) {
			oldest, has = k, true
		}
	}
	if has {
		delete(m, oldest)
	}
}

func dropEarliestCode(m map[string]loginCode) {
	oldest, has := "", false
	for k, v := range m {
		if !has || v.ExpiresAt.Before(m[oldest].ExpiresAt) {
			oldest, has = k, true
		}
	}
	if has {
		delete(m, oldest)
	}
}

// Start 生成 state（32 字节随机 → base64url），登记一次流程；有效期 10 分钟。
func (f *FlowStore) Start(nonce, verifier string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	f.ensureRoomLocked(now)
	state := randomToken()
	f.flows[state] = Flow{Nonce: nonce, Verifier: verifier, ExpiresAt: now.Add(flowTTL)}
	return state
}

// Take 取出并删除流程（一次性）；不存在/已过期 → ok=false。
func (f *FlowStore) Take(state string) (Flow, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fl, ok := f.flows[state]
	if !ok {
		return Flow{}, false
	}
	delete(f.flows, state)
	if time.Now().After(fl.ExpiresAt) {
		return Flow{}, false
	}
	return fl, true
}

// IssueLoginCode 登记一次性登录码（有效期 60 秒），随 302 返回给前端换取会话。
func (f *FlowStore) IssueLoginCode(userID uint64, token string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	f.ensureRoomLocked(now)
	code := randomToken()
	f.codes[code] = loginCode{UserID: userID, Token: token, ExpiresAt: now.Add(loginCodeTTL)}
	return code
}

// RedeemLoginCode 兑换并删除登录码（一次性）。
func (f *FlowStore) RedeemLoginCode(code string) (uint64, string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	lc, ok := f.codes[code]
	if !ok {
		return 0, "", false
	}
	delete(f.codes, code)
	if time.Now().After(lc.ExpiresAt) {
		return 0, "", false
	}
	return lc.UserID, lc.Token, true
}
