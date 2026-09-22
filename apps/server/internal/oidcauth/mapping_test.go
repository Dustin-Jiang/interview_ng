package oidcauth

import (
	"errors"
	"strings"
	"testing"

	dsmodel "interview_ng/internal/model"
)

func TestHit(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want bool
	}{
		{"true 命中", true, true},
		{"false 未命中", false, false},
		{"空串未命中", "", false},
		{"非空串命中", "x", true},
		{"空数组未命中", []any{}, false},
		{"非空数组命中", []any{"g"}, true},
		{"空字符串切片未命中", []string{}, false},
		{"空对象未命中", map[string]any{}, false},
		{"非空对象命中", map[string]any{"a": 1}, true},
		{"数字 0 命中", 0.0, true},
		{"非零数字命中", 3.0, true},
		{"nil 未命中", nil, false},
	}
	for _, tc := range cases {
		if got := Hit(tc.in); got != tc.want {
			t.Errorf("%s: Hit(%v) = %v, want %v", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestCompileRejectsBadExpression(t *testing.T) {
	if _, err := Compile("groups["); err == nil {
		t.Fatalf("非法表达式应报错")
	}
	if _, err := Compile("groups[?@ == 'x'] | [0]"); err != nil {
		t.Fatalf("合法表达式不应报错: %v", err)
	}
}

func TestMatchRoleFirstHitWins(t *testing.T) {
	rules := []dsmodel.OidcRoleRule{
		{Position: 0, Expression: "groups[?@ == 'interview-interviewers'] | [0]", RoleID: 5},
		{Position: 1, Expression: "@", RoleID: 7},
	}

	// claims 无 groups → 第 1 条求值为 nil（未命中），落到第 2 条恒真。
	roleID, hit, err := MatchRole(rules, map[string]any{"sub": "u1"})
	if err != nil {
		t.Fatalf("MatchRole: %v", err)
	}
	if !hit || roleID != 7 {
		t.Fatalf("无 groups 应命中第 2 条: hit=%v role=%d", hit, roleID)
	}

	// claims 含目标组 → 第 1 条命中（首个命中生效）。
	roleID, hit, err = MatchRole(rules, map[string]any{"groups": []any{"interview-interviewers", "other"}})
	if err != nil {
		t.Fatalf("MatchRole: %v", err)
	}
	if !hit || roleID != 5 {
		t.Fatalf("应命中第 1 条: hit=%v role=%d", hit, roleID)
	}
}

func TestMatchRoleNoRulesAndNoHit(t *testing.T) {
	if roleID, hit, err := MatchRole(nil, map[string]any{"a": 1}); err != nil || hit || roleID != 0 {
		t.Fatalf("空规则应无命中: hit=%v role=%d err=%v", hit, roleID, err)
	}
	rules := []dsmodel.OidcRoleRule{{Expression: "groups[?@ == 'nobody'] | [0]", RoleID: 5}}
	if _, hit, err := MatchRole(rules, map[string]any{"groups": []any{"x"}}); err != nil || hit {
		t.Fatalf("不匹配应无命中: hit=%v err=%v", hit, err)
	}
}

func TestMatchDepartmentFirstHitAndEvalFailure(t *testing.T) {
	rules := []dsmodel.OidcDeptRule{
		{Position: 0, Expression: "groups[?@ == 'interview-tech'] | [0]", DepartmentID: 3},
		{Position: 1, Expression: "@", DepartmentID: 9},
	}
	// 与角色规则同一套语义：顺序求值、首个命中生效（声明缺失时落到下一条）。
	if id, hit, err := MatchDepartment(rules, map[string]any{"sub": "u1"}); err != nil || !hit || id != 9 {
		t.Fatalf("无 groups 应落到第 2 条: hit=%v dept=%d err=%v", hit, id, err)
	}
	if id, hit, err := MatchDepartment(rules, map[string]any{"groups": []any{"interview-tech"}}); err != nil || !hit || id != 3 {
		t.Fatalf("应命中第 1 条: hit=%v dept=%d err=%v", hit, id, err)
	}

	// 空规则 → 无命中（调用方据此「不改动现有部门」）。
	if id, hit, err := MatchDepartment(nil, map[string]any{"a": 1}); err != nil || hit || id != 0 {
		t.Fatalf("空规则应无命中: hit=%v dept=%d err=%v", hit, id, err)
	}

	// 缺声明 + 未做类型守卫 → 求值失败；文案要点明是**部门规则**（否则日志里认不出是哪一类规则）
	fragile := []dsmodel.OidcDeptRule{
		{Expression: "length(groups[? ends_with(@, '-tech')]) > \x600\x60", DepartmentID: 3},
	}
	if _, _, err := MatchDepartment(fragile, map[string]any{"sub": "u1"}); !errors.Is(err, ErrRuleEval) ||
		!strings.Contains(err.Error(), "第 1 条部门规则") {
		t.Fatalf("缺少声明应报求值失败且指明部门规则: %v", err)
	}
}

func TestDeriveUsername(t *testing.T) {
	cases := []struct {
		name    string
		claims  map[string]any
		subject string
		want    string
	}{
		{"preferred_username 优先", map[string]any{"preferred_username": "alice", "email": "bob@example.com"}, "sub-1", "alice"},
		{"回落邮箱前缀", map[string]any{"email": "bob@example.com"}, "sub-1", "bob"},
		{"回落 sub", map[string]any{}, "auth0|abc", "auth0abc"},
		{"剔除非法字符", map[string]any{"preferred_username": "a b/c@d"}, "s", "abcd"},
		{"全空回落 oidc-user", map[string]any{"preferred_username": "///"}, "", "oidc-user"},
	}
	for _, tc := range cases {
		if got := DeriveUsername(tc.claims, tc.subject); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}

	long := DeriveUsername(map[string]any{"preferred_username": strings.Repeat("a", 100)}, "s")
	if len(long) != 64 {
		t.Fatalf("应截断至 64 字符: %d", len(long))
	}
}

func TestClaimString(t *testing.T) {
	claims := map[string]any{"name": "张三", "n": 1}
	if got := ClaimString(claims, "name"); got != "张三" {
		t.Fatalf("name: %q", got)
	}
	if got := ClaimString(claims, "n"); got != "" {
		t.Fatalf("非字符串应返回空串: %q", got)
	}
	if got := ClaimString(claims, "missing"); got != "" {
		t.Fatalf("缺失应返回空串: %q", got)
	}
}
