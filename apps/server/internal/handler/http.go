package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"interview_ng/internal/auth"
	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/oidcauth"
	"interview_ng/internal/service"
	"interview_ng/internal/state"
)

// HTTPServer 承载非 WS 的 HTTP 接口（认证、候选人、房间、用户与角色管理）。
type HTTPServer struct {
	svc  *service.InterviewService
	st   state.StateStore
	auth *auth.Manager
	oidc *oidcauth.Service
}

// NewHTTPServer 构建 HTTP 服务器。
func NewHTTPServer(svc *service.InterviewService, st state.StateStore, am *auth.Manager, oidc *oidcauth.Service) *HTTPServer {
	return &HTTPServer{svc: svc, st: st, auth: am, oidc: oidc}
}

// require 权限中间件简写。
func (h *HTTPServer) require(perm string) gin.HandlerFunc { return h.auth.RequirePerm(perm) }

// RegisterRoutes 注册 HTTP 路由（/api 组：health/sessions 公共，其余经鉴权 + 权限矩阵）。
func (h *HTTPServer) RegisterRoutes(r *gin.Engine) {
	g := r.Group("/api")
	g.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	g.POST("/sessions", h.login)

	// 登录方式与单点登录（OIDC）公开入口：授权跳转 + IdP 回调 + 一次性登录码换会话。
	g.GET("/authentication", h.getAuthentication)
	g.GET("/oidc/authorization", h.startOidcAuthorization)
	g.GET("/oidc/sessions", h.oidcCallback)
	g.POST("/oidc/sessions", h.createOidcSession)

	authed := g.Group("")
	authed.Use(h.auth.RequireAuth())
	authed.GET("/me", h.me)
	authed.PUT("/me/password", h.changePassword)

	// 候选人（浏览任意登录，操作按权限）
	authed.GET("/candidates", h.listCandidates)
	authed.GET("/candidates/:id", h.getCandidate)
	// 候选人面试记录归档（按候选人维度，完成后仍可查；任意登录用户可读 —— 查看与管理分离）
	authed.GET("/candidates/:id/messages", h.listCandidateMessages)
	// 归档补充一条记录（面试结档后仍可写；权限与房间聊天一致）：
	// 房间通道只服务进行中的面试，归档补充不依赖房间与在场成员。
	authed.POST("/candidates/:id/messages", h.require(dsmodel.PermRoomsChat), h.appendCandidateMessage)
	// 编辑 / 撤回自己的记录（2 分钟内，仅发送者本人 —— 服务端按消息发送者与创建时刻复核）
	authed.PATCH("/candidates/:id/messages/:messageId", h.require(dsmodel.PermRoomsChat), h.editCandidateMessage)
	authed.DELETE("/candidates/:id/messages/:messageId", h.require(dsmodel.PermRoomsChat), h.deleteCandidateMessage)
	// 表情回复：一条记录上的一个表情（PUT 加上 / DELETE 撤回，均幂等；表情限允许集）
	authed.PUT("/candidates/:id/messages/:messageId/reactions/:emoji", h.require(dsmodel.PermRoomsChat), h.setMessageReaction(true))
	authed.DELETE("/candidates/:id/messages/:messageId/reactions/:emoji", h.require(dsmodel.PermRoomsChat), h.setMessageReaction(false))
	authed.POST("/candidates", h.require(dsmodel.PermCandidatesCreate), h.createCandidate)
	// 批量导入（管理员数据导入）：请求体为浏览器侧解析+映射后的行数组，服务端单事务全或无落库。
	authed.POST("/candidates/imports", h.require(dsmodel.PermCandidatesManage), h.importCandidates)
	authed.PUT("/candidates/:id/check-in", h.require(dsmodel.PermCandidatesCheckin), h.checkin)
	authed.PUT("/candidates/:id", h.require(dsmodel.PermCandidatesManage), h.updateCandidate)
	authed.DELETE("/candidates/:id", h.require(dsmodel.PermCandidatesManage), h.deleteCandidate)
	authed.PUT("/candidates/:id/status", h.require(dsmodel.PermCandidatesManage), h.resetCandidateStatus)
	// 志愿与调剂：独立于资料全量编辑（面试官默认持有），只改这三项。
	authed.PATCH("/candidates/:id/preferences", h.require(dsmodel.PermCandidatesPreferences), h.updateCandidatePreferences)

	// 房间（浏览 rooms.view，管理 rooms.manage，拉取 candidates.assign）
	authed.GET("/rooms", h.require(dsmodel.PermRoomsView), h.listRooms)
	authed.GET("/rooms/:id", h.require(dsmodel.PermRoomsView), h.getRoom)
	authed.POST("/rooms", h.require(dsmodel.PermRoomsManage), h.createRoom)
	authed.PATCH("/rooms/:id", h.require(dsmodel.PermRoomsManage), h.renameRoom)
	authed.DELETE("/rooms/:id", h.require(dsmodel.PermRoomsManage), h.deleteRoom)
	authed.POST("/rooms/:id/members", h.require(dsmodel.PermRoomsManage), h.addRoomMember)
	authed.DELETE("/rooms/:id/members/:userId", h.require(dsmodel.PermRoomsManage), h.removeRoomMember)
	authed.PUT("/rooms/:id/candidate", h.require(dsmodel.PermCandidatesAssign), h.pullCandidate)

	// 用户与角色（users.manage）
	authed.GET("/users", h.require(dsmodel.PermUsersManage), h.listUsers)
	authed.POST("/users", h.require(dsmodel.PermUsersManage), h.createUser)
	authed.PUT("/users/:id", h.require(dsmodel.PermUsersManage), h.updateUser)
	authed.DELETE("/users/:id", h.require(dsmodel.PermUsersManage), h.deleteUser)
	authed.PUT("/users/:id/password", h.require(dsmodel.PermUsersManage), h.resetUserPassword)
	authed.GET("/roles", h.require(dsmodel.PermUsersManage), h.listRoles)
	authed.POST("/roles", h.require(dsmodel.PermUsersManage), h.createRole)
	authed.PUT("/roles/:id", h.require(dsmodel.PermUsersManage), h.updateRole)
	authed.DELETE("/roles/:id", h.require(dsmodel.PermUsersManage), h.deleteRole)

	// 单点登录配置（users.manage）：读取配置 / 覆盖保存 / 连通性检测。
	authed.GET("/oidc/config", h.require(dsmodel.PermUsersManage), h.getOidcConfig)
	authed.PUT("/oidc/config", h.require(dsmodel.PermUsersManage), h.putOidcConfig)
	authed.POST("/oidc/probes", h.require(dsmodel.PermUsersManage), h.probeOidc)

	// 部门：浏览任意登录（志愿选择器需要部门名单），增删改 users.manage
	authed.GET("/departments", h.listDepartments)
	authed.POST("/departments", h.require(dsmodel.PermUsersManage), h.createDepartment)
	authed.PUT("/departments/:id", h.require(dsmodel.PermUsersManage), h.updateDepartment)
	authed.DELETE("/departments/:id", h.require(dsmodel.PermUsersManage), h.deleteDepartment)

	// 系统状态（读取任意登录：录取阶段 UI 需全员可见；切换仅 users.manage）
	authed.GET("/system/status", h.getSystemStatus)
	authed.PATCH("/system/status", h.require(dsmodel.PermUsersManage), h.patchSystemStatus)

	// 录取状态（浏览任意登录：默认本部门，跨部门需 browse_all；记录需 admissions.record）
	authed.GET("/admissions", h.listAdmissions)
	authed.PUT("/admissions/:candidateId", h.require(dsmodel.PermAdmissionRecord), h.upsertCandidateAdmission)

	// 捡漏竞拍（浏览任意登录；出价需 admissions.record 且仅本部门可见；结算由「进入结算阶段」自动触发）
	authed.GET("/leftover", h.leftoverOverview)
	authed.GET("/leftover/bids", h.listLeftoverBids)
	authed.GET("/leftover/projections", h.leftoverFinal)
	authed.PUT("/leftover/bids/:candidateId", h.require(dsmodel.PermAdmissionRecord), h.upsertLeftoverBid)
	authed.GET("/leftover/results", h.leftoverResults)
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
	h.writeSession(c, token, uid, roles, perms)
}

// writeSession 输出与 POST /api/sessions 完全一致的会话响应（密码登录与 OIDC 兑换共用）。
func (h *HTTPServer) writeSession(c *gin.Context, token string, uid uint64, roles, perms []string) {
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

// appendCandidateMessageReq 归档补充的请求体。
type appendCandidateMessageReq struct {
	Content string `json:"content"`
}

// appendCandidateMessage 向候选人面试记录归档补充一条消息（候选人查看页的补充入口）：
// 不需要房间与在场成员，故面试结档（房间已解绑）后仍可补充；权限与房间聊天一致（rooms.chat）。
// 响应只回新记录 id，内容由前端按归档重拉（与 GET /candidates/:id/messages 同一权威口径）。
func (h *HTTPServer) appendCandidateMessage(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req appendCandidateMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	ev, err := h.svc.AppendCandidateMessage(c.Request.Context(), id, auth.UserID(c), req.Content)
	if err != nil {
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": ev.MsgID})
}

// editCandidateMessage 编辑自己刚发出的记录（2 分钟内，仅发送者本人；服务端复核窗口与归属）。
func (h *HTTPServer) editCandidateMessage(c *gin.Context) {
	candID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	msgID, _ := strconv.ParseUint(c.Param("messageId"), 10, 64)
	var req appendCandidateMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.EditMessage(c.Request.Context(), candID, msgID, auth.UserID(c), req.Content); err != nil {
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// deleteCandidateMessage 撤回自己刚发出的记录（物理删除；窗口与归属同上）。
func (h *HTTPServer) deleteCandidateMessage(c *gin.Context) {
	candID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	msgID, _ := strconv.ParseUint(c.Param("messageId"), 10, 64)
	if err := h.svc.DeleteMessage(c.Request.Context(), candID, msgID, auth.UserID(c)); err != nil {
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// setMessageReaction 返回表情回复的开关处理（PUT = 加上、DELETE = 撤回）：幂等。
// 表情取自路径段（前端按 URL 编码提交），服务端按 model.ReactionEmojis 复核。
func (h *HTTPServer) setMessageReaction(on bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		candID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		msgID, _ := strconv.ParseUint(c.Param("messageId"), 10, 64)
		emoji := c.Param("emoji")
		err := h.svc.SetMessageReaction(c.Request.Context(), candID, msgID, auth.UserID(c), emoji, on)
		if err != nil {
			status, msg := stateErr(err)
			c.JSON(status, gin.H{"error": msg})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// createCandidateReq 新建候选人：请求体即扁平的资料字段全集（state.CandidateInfo）。
type createCandidateReq struct {
	state.CandidateInfo
}

func (h *HTTPServer) createCandidate(c *gin.Context) {
	var req createCandidateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	ev, err := h.svc.CreateCandidate(c.Request.Context(), req.CandidateInfo)
	if err != nil {
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": state.CandidateIDOf(ev)})
}

type importCandidatesReq struct {
	Rows []state.CandidateImportRow `json:"rows"`
}

// importCandidates 批量导入候选人：请求体只承载"已解析并映射好"的行，
// 服务端负责校验、单事务全或无落库与行级报告（解析在浏览器内完成）。
func (h *HTTPServer) importCandidates(c *gin.Context) {
	var req importCandidatesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	report, err := h.svc.ImportCandidates(c.Request.Context(), req.Rows)
	if err != nil {
		// 校验失败的整批拒绝：一次返回全部问题行（行号 + 原因），便于改完重传。
		if ie, ok := err.(*state.ImportError); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": ie.Error(), "rows": ie.Rows})
			return
		}
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, report)
}

func (h *HTTPServer) checkin(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.svc.CheckIn(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// updateCandidateReq 编辑候选人：请求体即扁平的资料字段全集（state.CandidateInfo，全量覆盖）。
type updateCandidateReq struct {
	state.CandidateInfo
}

func (h *HTTPServer) updateCandidate(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req updateCandidateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	if err := h.svc.UpdateCandidate(c.Request.Context(), id, req.CandidateInfo); err != nil {
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// updateCandidatePreferencesReq 志愿与调剂三项：bool 无法区分「缺省」，
// 因此请求体须给出这三项的完整取值（不做部分省略）。
type updateCandidatePreferencesReq struct {
	FirstChoice  string `json:"first_choice"`
	SecondChoice string `json:"second_choice"`
	AcceptAdjust bool   `json:"accept_adjust"`
}

// updateCandidatePreferences 修改候选人志愿与调剂（独立权限，不动其他资料）。
func (h *HTTPServer) updateCandidatePreferences(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req updateCandidatePreferencesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	err := h.svc.UpdateCandidatePreferences(c.Request.Context(), id, state.CandidatePreferences{
		FirstChoice:  req.FirstChoice,
		SecondChoice: req.SecondChoice,
		AcceptAdjust: req.AcceptAdjust,
	})
	if err != nil {
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
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

// createRoomReq 建房间请求体：name 可选（空/缺省=未命名）。
type createRoomReq struct {
	Name string `json:"name"`
}

func (h *HTTPServer) createRoom(c *gin.Context) {
	// 请求体允许缺省（旧客户端无体调用），绑定失败按未命名处理。
	var req createRoomReq
	_ = c.ShouldBindJSON(&req)
	ev, err := h.svc.CreateRoom(c.Request.Context(), req.Name)
	if err != nil {
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": state.RoomIDOf(ev)})
}

type renameRoomReq struct {
	Name string `json:"name"`
}

// renameRoom 修改房间名（PATCH 部分更新；空串=清除命名）。
func (h *HTTPServer) renameRoom(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req renameRoomReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.RenameRoom(c.Request.Context(), id, req.Name); err != nil {
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
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
	Username     string   `json:"username"`
	Name         string   `json:"name"`
	Password     string   `json:"password"`
	RoleIDs      []uint64 `json:"role_ids"`
	DepartmentID *uint64  `json:"department_id"`
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
		DepartmentID: req.DepartmentID,
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
	Username     string   `json:"username"`
	Name         string   `json:"name"`
	RoleIDs      []uint64 `json:"role_ids"`
	DepartmentID *uint64  `json:"department_id"`
}

func (h *HTTPServer) updateUser(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req updateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.UpdateUser(c.Request.Context(), id, req.Username, req.Name, req.DepartmentID, req.RoleIDs); err != nil {
		status, code := stateErr(err)
		c.JSON(status, gin.H{"error": code})
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

//---- 部门管理 ----

func (h *HTTPServer) listDepartments(c *gin.Context) {
	out, err := h.svc.ListDepartments(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

type departmentReq struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	ExpectedCount *int   `json:"expected_count"` // 预期人数（缺省 0，非负校验）
}

func (h *HTTPServer) createDepartment(c *gin.Context) {
	var req departmentReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	expected := 0
	if req.ExpectedCount != nil {
		if *req.ExpectedCount < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "expected_count must be non-negative"})
			return
		}
		expected = *req.ExpectedCount
	}
	id, err := h.svc.CreateDepartment(c.Request.Context(), req.Name, req.Description, expected)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *HTTPServer) updateDepartment(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req departmentReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	expected := 0
	if req.ExpectedCount != nil {
		if *req.ExpectedCount < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "expected_count must be non-negative"})
			return
		}
		expected = *req.ExpectedCount
	}
	if err := h.svc.UpdateDepartment(c.Request.Context(), id, req.Name, req.Description, expected); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *HTTPServer) deleteDepartment(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.svc.DeleteDepartment(c.Request.Context(), id); err != nil {
		status, code := stateErr(err)
		c.JSON(status, gin.H{"error": code})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

//---- 系统状态 ----

func (h *HTTPServer) getSystemStatus(c *gin.Context) {
	st, err := h.svc.GetSystemStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, st)
}

type patchSystemStatusReq struct {
	Phase   *string `json:"phase"`
	BidStep *int    `json:"bid_step"`
}

// patchSystemStatus 部分更新系统状态：phase 与/或 bid_step，至少一项（users.manage）。
func (h *HTTPServer) patchSystemStatus(c *gin.Context) {
	var req patchSystemStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if req.Phase == nil && req.BidStep == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if req.Phase != nil {
		if err := h.svc.SetSystemStatus(c.Request.Context(), dsmodel.SystemPhase(*req.Phase)); err != nil {
			status, code := stateErr(err)
			c.JSON(status, gin.H{"error": code})
			return
		}
	}
	if req.BidStep != nil {
		if err := h.svc.SetBidStep(c.Request.Context(), *req.BidStep); err != nil {
			status, code := stateErr(err)
			c.JSON(status, gin.H{"error": code})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

//---- 录取状态 ----

// listAdmissions 返回当前用户可见的录取决定：
// 默认仅本部门记录；持 candidates.browse_all 可跨部门查看全部部门的录取决定。
func (h *HTTPServer) listAdmissions(c *gin.Context) {
	uid := auth.UserID(c)
	if h.auth.HasPermission(uid, dsmodel.PermCandidatesBrowseAll) {
		out, err := h.svc.ListCandidateAdmissions(c.Request.Context(), nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": out})
		return
	}
	// 无跨部门权限：仅返回本部门记录；未归属部门则无可视录取决定。
	me, err := h.auth.Me(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var out []*dsmodel.CandidateAdmission
	if me.User.DepartmentID != nil {
		records, err := h.svc.ListCandidateAdmissions(c.Request.Context(), me.User.DepartmentID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = records
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

type upsertAdmissionReq struct {
	Status string `json:"status"`
}

func (h *HTTPServer) upsertCandidateAdmission(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("candidateId"), 10, 64)
	uid := auth.UserID(c)
	var req upsertAdmissionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	me, err := h.auth.Me(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if me.User.DepartmentID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "当前用户未归属部门，无法记录录取决定"})
		return
	}
	if err := h.svc.UpsertCandidateAdmission(c.Request.Context(), id, *me.User.DepartmentID, dsmodel.AdmissionStatus(req.Status)); err != nil {
		status, code := stateErr(err)
		c.JSON(status, gin.H{"error": code})
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
// not_found → 404（资源不存在）；student_no_exists → 409（唯一性冲突）；
// 其余 *state.Error 业务码统一 400，非业务错误落 500。
func stateErr(err error) (int, string) {
	if se, ok := err.(*state.Error); ok {
		switch se.Code {
		case "not_found":
			return http.StatusNotFound, se.Msg
		case "student_no_exists":
			return http.StatusConflict, se.Msg
		case "message_not_owner":
			return http.StatusForbidden, se.Msg
		case "message_window_expired":
			// 409：请求与当前状态冲突（窗口已关闭，记录已定格）。
			return http.StatusConflict, se.Msg
		}
		return http.StatusBadRequest, se.Msg
	}
	return http.StatusInternalServerError, err.Error()
}

//---- 捡漏竞拍 ----

// myDepartmentID 当前登录用户归属部门（nil 表示未归属部门）。
func (h *HTTPServer) myDepartmentID(c *gin.Context) (*uint64, error) {
	uid := auth.UserID(c)
	me, err := h.auth.Me(c.Request.Context(), uid)
	if err != nil {
		return nil, err
	}
	return me.User.DepartmentID, nil
}

// leftoverOverview 捡漏总览：各部门预算与当前阶段。
// spent/remaining 默认仅本部门可见；持 candidates.browse_all 的管理端全部门可见。
func (h *HTTPServer) leftoverOverview(c *gin.Context) {
	deptID, err := h.myDepartmentID(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	exposeAll := h.auth.HasPermission(auth.UserID(c), dsmodel.PermCandidatesBrowseAll)
	out, err := h.svc.LeftoverOverview(c.Request.Context(), deptID, exposeAll)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// listLeftoverBids 返回出价列表：默认仅本部门（出价保密）；
// 持 candidates.browse_all 的管理端返回全部部门；未归属部门且无跨部门权限 → 空。
func (h *HTTPServer) listLeftoverBids(c *gin.Context) {
	if h.auth.HasPermission(auth.UserID(c), dsmodel.PermCandidatesBrowseAll) {
		out, err := h.svc.ListLeftoverBids(c.Request.Context(), nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": out})
		return
	}
	deptID, err := h.myDepartmentID(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := []*dsmodel.Bid{}
	if deptID != nil {
		out, err = h.svc.ListLeftoverBids(c.Request.Context(), deptID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

type upsertLeftoverBidReq struct {
	Amount int `json:"amount"`
}

// upsertLeftoverBid 记录/覆盖本部门对候选人的出价（仅捡漏阶段、受预算约束）。
func (h *HTTPServer) upsertLeftoverBid(c *gin.Context) {
	candidateID, _ := strconv.ParseUint(c.Param("candidateId"), 10, 64)
	var req upsertLeftoverBidReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	deptID, err := h.myDepartmentID(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if deptID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "当前用户未归属部门，无法出价"})
		return
	}
	if err := h.svc.UpsertLeftoverBid(c.Request.Context(), candidateID, *deptID, req.Amount); err != nil {
		status, msg := stateErr(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// leftoverResults 已结算候选人的赢家与成交金额（全员可见）。
func (h *HTTPServer) leftoverResults(c *gin.Context) {
	out, err := h.svc.ListLeftoverResults(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// leftoverFinal 结算阶段最终录取结果（只读计算：赢家 = 最高出价部门）。
// 保密语义与出价一致：默认仅已成交或本部门的进行中结果；browse_all 管理端全量。
func (h *HTTPServer) leftoverFinal(c *gin.Context) {
	deptID, err := h.myDepartmentID(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	exposeAll := h.auth.HasPermission(auth.UserID(c), dsmodel.PermCandidatesBrowseAll)
	out, err := h.svc.LeftoverFinalResults(c.Request.Context(), deptID, exposeAll)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}
