package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"interview_ng/internal/model"
	"interview_ng/internal/service"
	"interview_ng/internal/state"
)

// HTTPServer 承载非 WS 的 HTTP 接口（面试官浏览、签到、分配、房间查询）。
type HTTPServer struct {
	svc *service.InterviewService
	st  state.StateStore
}

// NewHTTPServer 构建 HTTP 服务器。
func NewHTTPServer(svc *service.InterviewService, st state.StateStore) *HTTPServer {
	return &HTTPServer{svc: svc, st: st}
}

// RegisterRoutes 注册 HTTP 路由。
func (h *HTTPServer) RegisterRoutes(r *gin.Engine) {
	g := r.Group("/api")
	g.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	g.GET("/candidates", h.listCandidates)
	g.GET("/candidates/:id", h.getCandidate)
	g.POST("/candidates", h.createCandidate)
	g.POST("/candidates/:id/checkin", h.checkin)
	g.POST("/candidates/:id/assign", h.assign)
	g.GET("/rooms", h.listRooms)
	g.GET("/rooms/:id", h.getRoom)
}

func (h *HTTPServer) listCandidates(c *gin.Context) {
	status := c.Query("status")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	out, err := h.st.ListCandidates(c.Request.Context(), statusLiteral(status), limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"items": out})
}

func (h *HTTPServer) getCandidate(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	out, err := h.st.GetCandidate(c.Request.Context(), id)
	if err != nil {
		c.JSON(404, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, out)
}

type createCandidateReq struct {
	Name    string `json:"name"`
	Profile string `json:"profile"`
}

func (h *HTTPServer) createCandidate(c *gin.Context) {
	var req createCandidateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(400, gin.H{"error": "name required"})
		return
	}
	id, err := h.svc.CreateCandidate(c.Request.Context(), req.Name, req.Profile)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"id": id})
}

func (h *HTTPServer) checkin(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.svc.CheckIn(c.Request.Context(), id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

type assignReq struct {
	RoomID *uint64 `json:"room_id"` // 可选；为 0/空时新建房间
}

func (h *HTTPServer) assign(c *gin.Context) {
	candID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req assignReq
	_ = c.ShouldBindJSON(&req)
	roomID := uint64(0)
	if req.RoomID != nil {
		roomID = *req.RoomID
	}
	assignedRoomID, err := h.svc.AssignCandidate(c.Request.Context(), candID, roomID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "room_id": assignedRoomID})
}

func (h *HTTPServer) getRoom(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	out, err := h.st.GetRoom(c.Request.Context(), id)
	if err != nil {
		c.JSON(404, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, out)
}

func (h *HTTPServer) listRooms(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	out, err := h.st.ListRooms(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"items": out})
}

func statusLiteral(s string) model.CandidateStatus {
	// 简单校验，避免注入非法状态。
	switch s {
	case "NOT_CHECKED_IN", "CHECKED_IN_PENDING_ASSIGN", "ASSIGNED", "IN_PROGRESS", "COMPLETED":
		return model.CandidateStatus(s)
	default:
		return ""
	}
}
