package oidcauth

import (
	"reflect"
	"strings"

	"github.com/jmespath/go-jmespath"

	dsmodel "interview_ng/internal/model"
)

// Compile 校验表达式可解析（保存配置时校验用）。
func Compile(expression string) (*jmespath.JMESPath, error) {
	return jmespath.Compile(expression)
}

// Hit 判定表达式结果是否命中：result != nil 且不属于 {false, "", 空数组, 空对象}。
// 数字（含 0）视为显式命中；JMESPath 未命中返回 nil → 未命中。
func Hit(result any) bool {
	if result == nil {
		return false
	}
	switch v := result.(type) {
	case bool:
		return v
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case []string:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	case map[string]string:
		return len(v) > 0
	}
	// 其余标量（数字、时间等）视为显式命中；指针/接口为 nil 时未命中。
	rv := reflect.ValueOf(result)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len() > 0
	case reflect.Ptr, reflect.Interface:
		return !rv.IsNil()
	}
	return true
}

// MatchRole 按 Position 升序逐条求值，返回首个命中的角色 id；
// 无命中 → (0, false, nil)；表达式执行出错 → error。
// 规则顺序即切片顺序（store 已按 position 升序返回，此处不重排）。
func MatchRole(rules []dsmodel.OidcRoleRule, claims map[string]any) (uint64, bool, error) {
	for i := range rules {
		node, err := Compile(rules[i].Expression)
		if err != nil {
			return 0, false, err
		}
		out, err := node.Search(claims)
		if err != nil {
			return 0, false, err
		}
		if Hit(out) {
			return rules[i].RoleID, true, nil
		}
	}
	return 0, false, nil
}

// DeriveUsername 取用户名：preferred_username → 邮箱 @ 前缀 → sub；
// 仅保留 [A-Za-z0-9._-]，截断至 64 字符；全空时回落 "oidc-user"。
func DeriveUsername(claims map[string]any, subject string) string {
	for _, cand := range []string{ClaimString(claims, "preferred_username"), emailPrefix(ClaimString(claims, "email")), subject} {
		if u := sanitizeUsername(cand); u != "" {
			return u
		}
	}
	return "oidc-user"
}

func emailPrefix(email string) string {
	if i := strings.IndexByte(email, '@'); i > 0 {
		return email[:i]
	}
	return ""
}

// sanitizeUsername 只保留 [A-Za-z0-9._-]，并按 64 字符截断。
func sanitizeUsername(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		}
		if b.Len() >= 64 {
			break
		}
	}
	return b.String()
}

// ClaimString 取字符串声明（缺失/类型不符返回 ""）。
func ClaimString(claims map[string]any, key string) string {
	if v, ok := claims[key].(string); ok {
		return v
	}
	return ""
}
