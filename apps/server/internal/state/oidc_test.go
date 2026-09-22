package state_test

import (
	"context"
	"errors"
	"testing"

	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/state"
)

// roleID 建一个测试角色并返回其 id。
func roleID(t *testing.T, ctx context.Context, st state.StateStore, name string) uint64 {
	t.Helper()
	id, err := st.CreateRole(ctx, name, "", nil)
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	return id
}

// deptID 建一个测试部门并返回其 id。
func deptID(t *testing.T, ctx context.Context, st state.StateStore, name string) uint64 {
	t.Helper()
	id, err := st.CreateDepartment(ctx, name, "", 0)
	if err != nil {
		t.Fatalf("CreateDepartment: %v", err)
	}
	return id
}

// TestOidcConfigLazyDefaults 首次读取即落库一行默认配置。
func TestOidcConfigLazyDefaults(t *testing.T) {
	st := newTestStore(t)
	cfg, err := st.GetOidcConfig(context.Background())
	if err != nil {
		t.Fatalf("GetOidcConfig: %v", err)
	}
	if cfg.Scopes != dsmodel.DefaultOidcScopes || !cfg.AutoProvision || cfg.Enabled {
		t.Fatalf("默认值不符: %+v", cfg)
	}
	if cfg.RoleRules == nil || cfg.DepartmentRules == nil {
		t.Fatalf("规则应为空切片而非 nil")
	}
	if again, _ := st.GetOidcConfig(context.Background()); again.ID != cfg.ID {
		t.Fatalf("懒建应幂等: %d vs %d", again.ID, cfg.ID)
	}
}

// TestSetOidcConfigSecretSemantics 密钥三段语义：nil 保持、"" 清除、非空覆盖。
func TestSetOidcConfigSecretSemantics(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	cfg := &dsmodel.OidcConfig{Scopes: dsmodel.DefaultOidcScopes, AutoProvision: true}

	secret := "s3cret"
	if err := st.SetOidcConfig(ctx, cfg, &secret); err != nil {
		t.Fatalf("首次保存: %v", err)
	}
	if got, _ := st.GetOidcConfig(ctx); got.ClientSecret != "s3cret" {
		t.Fatalf("应写入密钥: %q", got.ClientSecret)
	}

	// nil → 保持
	if err := st.SetOidcConfig(ctx, cfg, nil); err != nil {
		t.Fatalf("nil 密钥保存: %v", err)
	}
	if got, _ := st.GetOidcConfig(ctx); got.ClientSecret != "s3cret" {
		t.Fatalf("nil 应保持原密钥: %q", got.ClientSecret)
	}

	// "" → 清除
	empty := ""
	if err := st.SetOidcConfig(ctx, cfg, &empty); err != nil {
		t.Fatalf("清除密钥: %v", err)
	}
	if got, _ := st.GetOidcConfig(ctx); got.ClientSecret != "" {
		t.Fatalf("空串应清除密钥: %q", got.ClientSecret)
	}
}

// TestSetOidcConfigReplacesRules 两类规则均整体替换并按 position 升序读回。
func TestSetOidcConfigReplacesRules(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	r1 := roleID(t, ctx, st, "r1")
	r2 := roleID(t, ctx, st, "r2")
	d1 := deptID(t, ctx, st, "研发部")

	cfg := &dsmodel.OidcConfig{
		Scopes: dsmodel.DefaultOidcScopes, AutoProvision: true,
		RoleRules: []dsmodel.OidcRoleRule{
			{Position: 0, Expression: "@", RoleID: r1},
			{Position: 1, Expression: "groups[?@ == 'x'] | [0]", RoleID: r2},
		},
		DepartmentRules: []dsmodel.OidcDeptRule{
			{Position: 0, Expression: "groups[?@ == 'tech'] | [0]", DepartmentID: d1},
		},
	}
	if err := st.SetOidcConfig(ctx, cfg, nil); err != nil {
		t.Fatalf("保存规则: %v", err)
	}
	got, _ := st.GetOidcConfig(ctx)
	if len(got.RoleRules) != 2 || got.RoleRules[0].RoleID != r1 || got.RoleRules[1].RoleID != r2 {
		t.Fatalf("角色规则读回不符: %+v", got.RoleRules)
	}
	if got.RoleRules[0].Position != 0 || got.RoleRules[1].Position != 1 {
		t.Fatalf("position 应重排为下标: %+v", got.RoleRules)
	}
	if len(got.DepartmentRules) != 1 || got.DepartmentRules[0].DepartmentID != d1 {
		t.Fatalf("部门规则读回不符: %+v", got.DepartmentRules)
	}

	// 再次保存 → 旧规则被整体替换（角色剩 1 条、部门清空）
	if err := st.SetOidcConfig(ctx, &dsmodel.OidcConfig{
		Scopes: dsmodel.DefaultOidcScopes, AutoProvision: true,
		RoleRules: []dsmodel.OidcRoleRule{{Expression: "@", RoleID: r2}},
	}, nil); err != nil {
		t.Fatalf("替换规则: %v", err)
	}
	got, _ = st.GetOidcConfig(ctx)
	if len(got.RoleRules) != 1 || got.RoleRules[0].RoleID != r2 {
		t.Fatalf("角色规则应被整体替换: %+v", got.RoleRules)
	}
	if len(got.DepartmentRules) != 0 {
		t.Fatalf("部门规则应被整体替换为空: %+v", got.DepartmentRules)
	}
}

// TestSetOidcConfigValidation 启用态的必填/格式校验与规则校验的错误码。
func TestSetOidcConfigValidation(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	valid := &dsmodel.OidcConfig{
		Enabled: true, Issuer: "https://sso.example.com", ClientID: "interview-ng",
		Scopes: dsmodel.DefaultOidcScopes, RedirectURL: "http://app.example/api/oidc/sessions",
		AutoProvision: true,
	}

	cases := []struct {
		name string
		cfg  *dsmodel.OidcConfig
		code string
	}{
		{"Issuer 非法", &dsmodel.OidcConfig{Enabled: true, Issuer: "sso.example.com", ClientID: "c",
			Scopes: "openid", RedirectURL: "http://a.example/cb"}, "oidc_issuer_invalid"},
		{"ClientID 缺失", &dsmodel.OidcConfig{Enabled: true, Issuer: "https://s.example",
			Scopes: "openid", RedirectURL: "http://a.example/cb"}, "oidc_client_id_required"},
		{"回调地址非法", &dsmodel.OidcConfig{Enabled: true, Issuer: "https://s.example", ClientID: "c",
			Scopes: "openid", RedirectURL: "/callback"}, "oidc_redirect_url_invalid"},
		{"Scopes 缺 openid", &dsmodel.OidcConfig{Enabled: true, Issuer: "https://s.example", ClientID: "c",
			Scopes: "profile email", RedirectURL: "http://a.example/cb"}, "oidc_scopes_invalid"},
	}
	for _, tc := range cases {
		err := st.SetOidcConfig(ctx, tc.cfg, nil)
		var se *state.Error
		if !errors.As(err, &se) || se.Code != tc.code {
			t.Errorf("%s: got %v, want code %s", tc.name, err, tc.code)
		}
	}

	// 未启用：不完整配置可保存（含空 Issuer）
	if err := st.SetOidcConfig(ctx, &dsmodel.OidcConfig{Scopes: dsmodel.DefaultOidcScopes}, nil); err != nil {
		t.Fatalf("未启用应允许保存不完整配置: %v", err)
	}

	// 表达式非法：无论开关
	bad := *valid
	bad.RoleRules = []dsmodel.OidcRoleRule{{Expression: "groups[", RoleID: roleID(t, ctx, st, "r3")}}
	var se *state.Error
	if err := st.SetOidcConfig(ctx, &bad, nil); !errors.As(err, &se) || se.Code != "oidc_rule_invalid" {
		t.Fatalf("非法表达式应被拒: %v", err)
	}

	// 空表达式
	blank := *valid
	blank.RoleRules = []dsmodel.OidcRoleRule{{Expression: "   ", RoleID: 1}}
	if err := st.SetOidcConfig(ctx, &blank, nil); !errors.As(err, &se) || se.Code != "oidc_rule_invalid" {
		t.Fatalf("空表达式应被拒: %v", err)
	}

	// 角色不存在
	missing := *valid
	missing.RoleRules = []dsmodel.OidcRoleRule{{Expression: "@", RoleID: 99999}}
	if err := st.SetOidcConfig(ctx, &missing, nil); !errors.As(err, &se) || se.Code != "oidc_rule_role_missing" {
		t.Fatalf("未知角色应被拒: %v", err)
	}

	// 部门规则：非法表达式与未知部门的错误码与角色规则平行
	badDept := *valid
	badDept.DepartmentRules = []dsmodel.OidcDeptRule{{Expression: "groups[", DepartmentID: deptID(t, ctx, st, "d1")}}
	if err := st.SetOidcConfig(ctx, &badDept, nil); !errors.As(err, &se) || se.Code != "oidc_dept_rule_invalid" {
		t.Fatalf("部门规则非法表达式应被拒: %v", err)
	}
	missingDept := *valid
	missingDept.DepartmentRules = []dsmodel.OidcDeptRule{{Expression: "@", DepartmentID: 99999}}
	if err := st.SetOidcConfig(ctx, &missingDept, nil); !errors.As(err, &se) || se.Code != "oidc_dept_rule_department_missing" {
		t.Fatalf("未知部门应被拒: %v", err)
	}

	// 校验失败不落库：配置仍为上一次成功保存的状态（未启用、无规则）
	got, _ := st.GetOidcConfig(ctx)
	if got.Enabled || len(got.RoleRules) != 0 || len(got.DepartmentRules) != 0 {
		t.Fatalf("校验失败不应落库: %+v", got)
	}
}

// TestOidcUserLookupAndSync 按 sub 查号与显示名/角色/部门的 IdP 权威覆盖。
func TestOidcUserLookupAndSync(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	r1 := roleID(t, ctx, st, "rA")
	r2 := roleID(t, ctx, st, "rB")
	d1 := deptID(t, ctx, st, "研发部")
	d2 := deptID(t, ctx, st, "市场部")

	sub := "oidc-1"
	uid, err := st.CreateUser(ctx, &dsmodel.User{Username: "oidc-user", Name: "旧名", OidcSubject: &sub}, []uint64{r1})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	u, err := st.FindUserByOidcSubject(ctx, sub)
	if err != nil || u.ID != uid {
		t.Fatalf("按 sub 查号失败: %v %+v", err, u)
	}
	if _, err := st.FindUserByOidcSubject(ctx, "nope"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("未知 sub 应 ErrNotFound: %v", err)
	}

	if err := st.SyncOidcUser(ctx, uid, "OIDC 面试官", []uint64{r2}, &d1); err != nil {
		t.Fatalf("SyncOidcUser: %v", err)
	}
	u, _ = st.FindUserByOidcSubject(ctx, sub)
	if u.Name != "OIDC 面试官" || len(u.Roles) != 1 || u.Roles[0].ID != r2 {
		t.Fatalf("同步后展示不符: %+v", u)
	}
	if u.DepartmentID == nil || *u.DepartmentID != d1 {
		t.Fatalf("同步应写入部门 %d: %+v", d1, u.DepartmentID)
	}

	// name 为空串 → 保留原显示名
	if err := st.SyncOidcUser(ctx, uid, "", []uint64{r2}, &d2); err != nil {
		t.Fatalf("SyncOidcUser: %v", err)
	}
	u, _ = st.FindUserByOidcSubject(ctx, sub)
	if u.Name != "OIDC 面试官" {
		t.Fatalf("空显示名应保留原值: %q", u.Name)
	}
	if u.DepartmentID == nil || *u.DepartmentID != d2 {
		t.Fatalf("命中部门的规则应覆盖旧部门: %+v", u.DepartmentID)
	}

	// departmentID 为 nil（未命中部门规则）→ 保持现有部门不动
	if err := st.SyncOidcUser(ctx, uid, "", []uint64{r2}, nil); err != nil {
		t.Fatalf("SyncOidcUser: %v", err)
	}
	if u, _ = st.FindUserByOidcSubject(ctx, sub); u.DepartmentID == nil || *u.DepartmentID != d2 {
		t.Fatalf("未命中部门规则不应清空现有部门: %+v", u.DepartmentID)
	}
}

// TestCreateUserDuplicateUsername 建号撞名给出明确业务码（OIDC 自动开通的兜底重试依赖它）。
func TestCreateUserDuplicateUsername(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	if _, err := st.CreateUser(ctx, &dsmodel.User{Username: "dup"}, nil); err != nil {
		t.Fatalf("首个建号: %v", err)
	}
	_, err := st.CreateUser(ctx, &dsmodel.User{Username: "dup"}, nil)
	var se *state.Error
	if !errors.As(err, &se) || se.Code != "username_taken" {
		t.Fatalf("撞名应返回 username_taken: %v", err)
	}
}
