package oidcauth

import (
	"errors"
	"fmt"
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

// ErrRuleEval 规则表达式**求值**失败：表达式本身合法（保存时已编译校验过），但声明缺失或类型不符
// （典型：ID token 里没有 groups 声明，`length(groups[? …])` 对 null 取 length 直接报错）。
// 与「未命中任何规则」（ErrOidcRoleUnmapped）区分开：前者是配置写错，后者是用户不在任何组里。
// 角色规则与部门规则求值失败走同一个错误：都表示「无法判定账号的授权/归属」，一律拒绝登录。
var ErrRuleEval = errors.New("oidc rule evaluation failed")

// rule 是求值单元的归一形态：角色规则与部门规则共用同一套「顺序求值、首个命中生效」语义。
type rule struct {
	expression string
	targetID   uint64
}

// match 按切片顺序逐条求值，返回首个命中规则的目标 id；无命中 → (0, false, nil)。
// 表达式**求值**出错 → 包 ErrRuleEval 的错误（含第几条、规则种类与表达式原文，
// 便于服务端日志与管理员定位）。kind 形如「角色规则」「部门规则」，只进错误文案。
func match(rules []rule, kind string, claims map[string]any) (uint64, bool, error) {
	for i := range rules {
		node, err := Compile(rules[i].expression)
		if err != nil {
			return 0, false, fmt.Errorf("%w: 第 %d 条%s（%s）编译失败：%v", ErrRuleEval, i+1, kind, rules[i].expression, err)
		}
		out, err := node.Search(claims)
		if err != nil {
			return 0, false, fmt.Errorf("%w: 第 %d 条%s（%s）求值失败：%v", ErrRuleEval, i+1, kind, rules[i].expression, err)
		}
		if Hit(out) {
			return rules[i].targetID, true, nil
		}
	}
	return 0, false, nil
}

// MatchRole 按顺序逐条求值角色规则，返回首个命中的角色 id；无命中 → (0, false, nil)。
// 规则顺序即切片顺序（store 已按 position 升序返回，此处不重排）。
func MatchRole(rules []dsmodel.OidcRoleRule, claims map[string]any) (uint64, bool, error) {
	converted := make([]rule, len(rules))
	for i := range rules {
		converted[i] = rule{expression: rules[i].Expression, targetID: rules[i].RoleID}
	}
	return match(converted, "角色规则", claims)
}

// MatchDepartment 同 MatchRole，但作用在部门规则上：返回首个命中的部门 id。
// 无命中 → (0, false, nil)，由调用方决定语义（当前为「不改变账号现有部门」，而非拒绝登录）。
func MatchDepartment(rules []dsmodel.OidcDeptRule, claims map[string]any) (uint64, bool, error) {
	converted := make([]rule, len(rules))
	for i := range rules {
		converted[i] = rule{expression: rules[i].Expression, targetID: rules[i].DepartmentID}
	}
	return match(converted, "部门规则", claims)
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
