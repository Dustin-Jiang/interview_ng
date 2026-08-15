package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// 最小实现 HS256 JWT（stdlib 自足，不引入外部依赖）。
// claims 仅含 user_id(sub) + ver(token_version 吊销计数) + iat + exp。
// 权限不落 token（Q10=A）：每次请求经 RBAC 缓存即时解析；ver 供改密/重置后踢掉旧 token（Q29）。

type claims struct {
	Sub uint64 `json:"sub"`
	Ver uint64 `json:"ver"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

var headerB64 = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// IssueToken 签发 HS256 JWT。
func IssueToken(secret string, userID, tokenVersion uint64, ttl time.Duration) (string, error) {
	now := time.Now()
	payload, err := json.Marshal(&claims{Sub: userID, Ver: tokenVersion, Iat: now.Unix(), Exp: now.Add(ttl).Unix()})
	if err != nil {
		return "", err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := headerB64 + "." + payloadB64
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig, nil
}

// ParseToken 校验签名与过期，返回 userID + token_version。
func ParseToken(secret, token string) (uint64, uint64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, 0, errors.New("invalid token format")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expect := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(expect, got) {
		return 0, 0, errors.New("invalid token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, 0, errors.New("invalid token payload")
	}
	var c claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return 0, 0, errors.New("invalid token claims")
	}
	if time.Now().Unix() >= c.Exp {
		return 0, 0, errors.New("token expired")
	}
	if c.Sub == 0 {
		return 0, 0, errors.New("invalid subject")
	}
	return c.Sub, c.Ver, nil
}

// TokenFromBearer 从 Authorization 头解析 Bearer token；缺头返回空串。
func TokenFromBearer(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}
