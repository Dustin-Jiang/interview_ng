package state_test

import (
	"context"
	"testing"

	dsmodel "interview_ng/internal/model"
)

// TestObservabilityConfigLazyDefaults 首次读取即落库一行默认配置（关闭 + 空 endpoint）。
func TestObservabilityConfigLazyDefaults(t *testing.T) {
	st := newTestStore(t)
	cfg, err := st.GetObservabilityConfig(context.Background())
	if err != nil {
		t.Fatalf("GetObservabilityConfig: %v", err)
	}
	if cfg.Enabled || cfg.Endpoint != "" {
		t.Fatalf("默认应为关闭且 endpoint 为空: %+v", cfg)
	}
	if again, _ := st.GetObservabilityConfig(context.Background()); again.ID != cfg.ID {
		t.Fatalf("懒建应幂等: %d vs %d", again.ID, cfg.ID)
	}
}

// TestSetObservabilityConfigPasswordSemantics 密码三段语义：nil 保持、空串清除、非空覆盖。
func TestSetObservabilityConfigPasswordSemantics(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	base := func() *dsmodel.ObservabilityConfig {
		return &dsmodel.ObservabilityConfig{Enabled: true, Endpoint: "http://greptime:4000"}
	}

	secret := "p1"
	if err := st.SetObservabilityConfig(ctx, base(), &secret); err != nil {
		t.Fatalf("首次保存: %v", err)
	}
	if got, _ := st.GetObservabilityConfig(ctx); got.Password != "p1" {
		t.Fatalf("应写入密码: %q", got.Password)
	}

	// nil = 保持不变
	if err := st.SetObservabilityConfig(ctx, base(), nil); err != nil {
		t.Fatalf("nil 保存: %v", err)
	}
	if got, _ := st.GetObservabilityConfig(ctx); got.Password != "p1" {
		t.Fatalf("nil 语义未保持密码: %q", got.Password)
	}

	// "" = 清除
	empty := ""
	if err := st.SetObservabilityConfig(ctx, base(), &empty); err != nil {
		t.Fatalf("清除: %v", err)
	}
	if got, _ := st.GetObservabilityConfig(ctx); got.Password != "" {
		t.Fatalf("空串未清除密码: %q", got.Password)
	}
}

// TestSetObservabilityConfigValidation 启用时 endpoint 必须是 http(s) 完整地址；
// 缺省回退：database/username/service_name 空串回落各自默认值。
func TestSetObservabilityConfigValidation(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if err := st.SetObservabilityConfig(ctx, &dsmodel.ObservabilityConfig{Enabled: true, Endpoint: "greptime:4000"}, nil); err == nil {
		t.Fatal("启用时非法 endpoint 应被拒绝")
	}

	cfg := &dsmodel.ObservabilityConfig{Enabled: true, Endpoint: "http://greptime:4000"}
	if err := st.SetObservabilityConfig(ctx, cfg, nil); err != nil {
		t.Fatalf("合法保存: %v", err)
	}
	got, _ := st.GetObservabilityConfig(ctx)
	if got.Database != "interview_ng" || got.Username != "greptime" || got.ServiceName != "interview_ng" {
		t.Fatalf("默认缺席者未被补齐: %+v", got)
	}
}
