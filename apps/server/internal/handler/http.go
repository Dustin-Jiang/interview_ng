package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"interview_ng/internal/auth"
	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/service"
	"interview_ng/internal/state"
)

// HTTPServer 承载非 WS 的 HTTP 接口（认证、候选人、房间、用户与角色管理）。
type HTTPServer struct {
	svc  *service.InterviewService
	st   state.StateStore
	auth *auth.Manager
}

// NewHTTPServer 构建 HTTP 服务器。
func NewHTTPServer(svc *service.InterviewService, st state.StateStore, am *auth.Manager) *HTTPServer {
	return &HTTPServer{svc: svc, st: st, auth: am}
}

// require 权限中间件简写。
func (h *HTTPServer) require(perm string) gin.HandlerFunc { return h.auth.RequirePerm(perm) }

// RegisterRoutes 注册 HTTP 路由（/api 组：health/login 公共，其余经鉴权 + 权限矩阵）。
func (h *HTTPServer) RegisterRoutes(r *gin.Engine) {
	g := r.Group("/api")
	g.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	g.POST("/auth/login", h.login)

	authed := g.Group("")
	authed.Use(h.auth.RequireAuth())
	authed.GET("/me", h.me)
	authed.POST("/auth/password", h.changePassword)

	// 候选人（浏览任意登录，操作按权限）
	authed.GET("/candidates", h.listCandidates)
	authed.GET("/candidates/:id", h.getCandidate)
	// 候选人面试记录归档（按候选人维度，完成后仍可查；任意登录用户可读 —— 查看与管理分离）
	authed.GET("/candidates/:id/messages", h.listCandidateMessages)
	authed.POST("/candidates", h.require(dsmodel.PermCandidatesCreate), h.createCandidate)
	authed.POST("/candidates/:id/checkin", h.require(dsmodel.PermCandidatesCheckin), h.checkin)
	authed.PUT("/candidates/:id", h.require(dsmodel.PermCandidatesManage), h.updateCandidate)
	authed.DELETE("/candidates/:id", h.require(dsmodel.PermCandidatesManage), h.deleteCandidate)
	authed.PUT("/candidates/:id/status", h.require(dsmodel.PermCandidatesManage), h.resetCandidateStatus)

	// 房间（浏览 rooms.view，管理 rooms.manage，拉取 candidates.assign）
	authed.GET("/rooms", h.require(dsmodel.PermRoomsView), h.listRooms)
	authed.GET("/rooms/:id", h.require(dsmodel.PermRoomsView), h.getRoom)
	authed.POST("/rooms", h.require(dsmodel.PermRoomsManage), h.createRoom)
	authed.DELETE("/rooms/:id", h.require(dsmodel.PermRoomsManage), h.deleteRoom)
	authed.POST("/rooms/:id/members", h.require(dsmodel.PermRoomsManage), h.addRoomMember)
	authed.DELETE("/rooms/:id/members/:userId", h.require(dsmodel.PermRoomsManage), h.removeRoomMember)
	authed.POST("/rooms/:id/pull_candidate", h.require(dsmodel.PermCandidatesAssign), h.pullCandidate)

	// 用户与角色（users.manage）
	authed.GET("/users", h.require(dsmodel.PermUsersManage), h.listUsers)
	authed.POST("/users", h.require(dsmodel.PermUsersManage), h.createUser)
	authed.PUT("/users/:id", h.require(dsmodel.PermUsersManage), h.updateUser)
	authed.DELETE("/users/:id", h.require(dsmodel.PermUsersManage), h.deleteUser)
	authed.POST("/users/:id/reset_password", h.require(dsmodel.PermUsersManage), h.resetUserPassword)
	authed.GET("/roles", h.require(dsmodel.PermUsersManage), h.listRoles)
	authed.POST("/roles", h.require(dsmodel.PermUsersManage), h.createRole)
	authed.PUT("/roles/:id", h.require(dsmodel.PermUsersManage), h.updateRole)
	authed.DELETE("/roles/:id", h.require(dsmodel.PermUsersManage), h.deleteRole)
}

//---- 认证 ----

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *HTTPServer) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名与密码必填"})
		return
	}
	token, uid, roles, perms, err := h.auth.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	profile, err := h.auth.Me(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "user": profile.User, "roles": roles, "permissions": perms})
}

func (h *HTTPServer) me(c *gin.Context) {
	uid := auth.UserID(c)
	profile, err := h.auth.Me(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": profile.User, "roles": profile.Roles, "permissions": profile.Permissions})
}

type changePasswordReq struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *HTTPServer) changePassword(c *gin.Context) {
	uid := auth.UserID(c)
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.auth.ChangePassword(c.Request.Context(), uid, req.OldPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

//---- 候选人 ----

func (h *HTTPServer) listCandidates(c *gin.Context) {
	status := statusLiteral(c.Query("status"))
	q := c.Query("q")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	out, err := h.st.ListCandidates(c.Request.Context(), status, q, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *HTTPServer) getCandidate(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	out, err := h.st.GetCandidate(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// listCandidateMessages 返回候选人的历史面试记录（消息按候选人归档，
// 与房间解绑无关：候选人完成/换房后仍可回看全过程）。
func (h *HTTPServer) listCandidateMessages(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if _, err := h.st.GetCandidate(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	out, err := h.st.ListMessagesAfter(c.Request.Context(), id, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

type createCandidateReq struct {
	Name    string `json:"name"`
	Profile string `json:"profile"`
}

func (h *HTTPServer) createCandidate(c *gin.Context) {
	var req createCandidateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	id, err := h.svc.CreateCandidate(c.Request.Context(), req.Name, req.Profile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *HTTPServer) checkin(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.svc.CheckIn(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type updateCandidateReq struct {
	Name    string `json:"name"`
	Profile string `json:"profile"`
}

func (h *HTTPServer) updateCandidate(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req updateCandidateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	if err := h.svc.UpdateCandidate(c.Request.Context(), id, req.Name, req.Profile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *HTTPServer) deleteCandidate(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.svc.DeleteCandidate(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type resetStatusReq struct {
	Status string `json:"status"`
}

func (h *HTTPServer) resetCandidateStatus(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req resetStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.ResetCandidateStatus(c.Request.Context(), id, dsmodel.CandidateStatus(req.Status)); err != nil {
		status, code := stateErr(err)
		c.JSON(status, gin.H{"error": code})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

//---- 房间 ----

func (h *HTTPServer) getRoom(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	out, err := h.st.GetRoom(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *HTTPServer) listRooms(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	out, err := h.st.ListRooms(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *HTTPServer) createRoom(c *gin.Context) {
	id, err := h.svc.CreateRoom(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *HTTPServer) deleteRoom(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.svc.DeleteRoom(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type addMemberReq struct {
	UserID uint64 `json:"user_id"`
}

func (h *HTTPServer) addRoomMember(c *gin.Context) {
	roomID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req addMemberReq
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
		return
	}
	if err := h.svc.AddRoomMember(c.Request.Context(), roomID, req.UserID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *HTTPServer) removeRoomMember(c *gin.Context) {
	roomID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	userID, _ := strconv.ParseUint(c.Param("userId"), 10, 64)
	if err := h.svc.RemoveRoomMember(c.Request.Context(), roomID, userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type pullCandidateReq struct {
	CandidateID uint64 `json:"candidate_id"`
}

func (h *HTTPServer) pullCandidate(c *gin.Context) {
	roomID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req pullCandidateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.CandidateID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "candidate_id required"})
		return
	}
	if err := h.svc.PullCandidate(c.Request.Context(), roomID, req.CandidateID); err != nil {
		status, code := stateErr(err)
		c.JSON(status, gin.H{"error": code})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

//---- 用户与角色管理 ----

func (h *HTTPServer) listUsers(c *gin.Context) {
	q := c.Query("q")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	out, err := h.svc.ListUsers(c.Request.Context(), q, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

type createUserReq struct {
	Username string   `json:"username"`
	Name     string   `json:"name"`
	Password string   `json:"password"`
	RoleIDs  []uint64 `json:"role_ids"`
}

func (h *HTTPServer) createUser(c *gin.Context) {
	var req createUserReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username/password required"})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	id, err := h.svc.CreateUser(c.Request.Context(), &dsmodel.User{
		Username:     req.Username,
		Name:         req.Name,
		PasswordHash: hash,
	}, req.RoleIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.auth.ReloadUser(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

type updateUserReq struct {
	Name    string   `json:"name"`
	RoleIDs []uint64 `json:"role_ids"`
}

func (h *HTTPServer) updateUser(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req updateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.UpdateUser(c.Request.Context(), id, req.Name, req.RoleIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.auth.ReloadUser(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *HTTPServer) deleteUser(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	me := auth.UserID(c)
	if me == id {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能删除自己"})
		return
	}
	if err := h.svc.DeleteUser(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = h.auth.ReloadAll()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type resetPasswordReq struct {
	NewPassword string `json:"new_password"`
}

func (h *HTTPServer) resetUserPassword(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req resetPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil || req.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new_password required"})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.ResetUserPassword(c.Request.Context(), id, hash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.auth.ReloadUser(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *HTTPServer) listRoles(c *gin.Context) {
	out, err := h.svc.ListRoles(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

type roleReq struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func (h *HTTPServer) createRole(c *gin.Context) {
	var req roleReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	for _, p := range req.Permissions {
		if !auth.ValidPerm(p) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法权限: " + p})
			return
		}
	}
	id, err := h.svc.CreateRole(c.Request.Context(), req.Name, req.Description, req.Permissions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.auth.ReloadAll(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *HTTPServer) updateRole(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req roleReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	for _, p := range req.Permissions {
		if !auth.ValidPerm(p) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法权限: " + p})
			return
		}
	}
	if err := h.svc.UpdateRole(c.Request.Context(), id, req.Name, req.Description, req.Permissions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.auth.ReloadAll(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *HTTPServer) deleteRole(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.svc.DeleteRole(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.auth.ReloadAll(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

//---- helpers ----

func statusLiteral(s string) dsmodel.CandidateStatus {
	switch s {
	case "NOT_CHECKED_IN", "CHECKED_IN_PENDING_ASSIGN", "ASSIGNED", "IN_PROGRESS", "COMPLETED":
		return dsmodel.CandidateStatus(s)
	default:
		return ""
	}
}

// stateErr 把 state 层错误映射为 HTTP 状态码与错误信息。
// 除 not_found（资源不存在 → 404）外，所有 *state.Error 业务码统一 400，其余落 500。
func stateErr(err error) (int, string) {
	if se, ok := err.(*state.Error); ok {
		if se.Code == "not_found" {
			return http.StatusNotFound, se.Msg
		}
		return http.StatusBadRequest, se.Msg
	}
	return http.StatusInternalServerError, err.Error()
}
