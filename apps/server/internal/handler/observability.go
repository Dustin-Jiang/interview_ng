package handler

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/observability"
)

//---- 可观测性（OTLP → GreptimeDB）配置（users.manage）----

type observabilityConfigResp struct {
	Enabled     bool      `json:"enabled"`
	Endpoint    string    `json:"endpoint"`
	Database    string    `json:"database"`
	Username    string    `json:"username"`
	PasswordSet bool      `json:"password_set"`
	ServiceName string    `json:"service_name"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// getObservabilityConfig 读取可观测性配置；密码只回发「是否已配置」。
func (h *HTTPServer) getObservabilityConfig(c *gin.Context) {
	cfg, err := h.svc.GetObservabilityConfig(c.Request.Context())
	if err != nil {
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, observabilityConfigResp{
		Enabled:     cfg.Enabled,
		Endpoint:    cfg.Endpoint,
		Database:    cfg.Database,
		Username:    cfg.Username,
		PasswordSet: cfg.Password != "",
		ServiceName: cfg.ServiceName,
		UpdatedAt:   cfg.UpdatedAt,
	})
}

type putObservabilityConfigReq struct {
	Enabled     bool    `json:"enabled"`
	Endpoint    string  `json:"endpoint"`
	Database    string  `json:"database"`
	Username    string  `json:"username"`
	Password    *string `json:"password"` // nil=保持不变，""=清除，非空=覆盖
	ServiceName string  `json:"service_name"`
}

// putObservabilityConfig 覆盖保存配置并令其即时生效：重配失败仍会保存成功但回
// applied=false，前端据此提示「已保存但观测当前关闭」。未启用 → 传空配置走关闭分支
// （flush 旧遥测后停推）。
func (h *HTTPServer) putObservabilityConfig(c *gin.Context) {
	var req putObservabilityConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	cfg := &dsmodel.ObservabilityConfig{
		Enabled:     req.Enabled,
		Endpoint:    strings.TrimSpace(req.Endpoint),
		Database:    strings.TrimSpace(req.Database),
		Username:    strings.TrimSpace(req.Username),
		ServiceName: strings.TrimSpace(req.ServiceName),
	}
	if err := h.svc.SetObservabilityConfig(c.Request.Context(), cfg, req.Password); err != nil {
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	saved, err := h.svc.GetObservabilityConfig(c.Request.Context())
	if err != nil {
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	applied := true
	if saved.Enabled {
		applied, err = observability.Apply(c.Request.Context(), observability.Config{
			Endpoint:    saved.Endpoint,
			Database:    saved.Database,
			Username:    saved.Username,
			Password:    saved.Password,
			ServiceName: saved.ServiceName,
		})
	} else {
		applied, err = observability.Apply(c.Request.Context(), observability.Config{})
	}
	if err != nil || !applied {
		slog.Error("observability apply failed after save", slog.Any("error", err))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "applied": applied})
}
