package state

import (
	"context"
	"time"

	dsmodel "interview_ng/internal/model"
)

// StateStore 是"系统内部状态唯一性"的唯一权威层。
// 暴露业务级原子操作（Q2=B），返回不可变变更事件（Q3=A3）；
// 当前实现为内存版（Q1=A），接口形态保证未来可替换为 Redis 实现而不动调用方。
//
// 持久化顺序约定：每个写操作必须【先落库成功后】才返回 Event（先落库后广播），
// 保证重连增量一定存在于库中，杜绝"客户端看到但库没有"造成的状态不同步。
type StateStore interface {
	// ---- 读：给瞬时一致快照（Q3=B1） ----

	// GetCandidate 返回候选人当前快照。
	GetCandidate(ctx context.Context, id uint64) (*dsmodel.Candidate, error)
	// GetCandidateByRoom 返回某房间绑定的候选人。
	GetCandidateByRoom(ctx context.Context, roomID uint64) (*dsmodel.Candidate, error)
	// GetRoom 返回房间快照（含成员、候选人）。
	GetRoom(ctx context.Context, roomID uint64) (*dsmodel.Room, error)
	// ListRooms 分页列出房间（面试官浏览；每项附带绑定候选人与消息条数）。
	ListRooms(ctx context.Context, limit, offset int) ([]*dsmodel.Room, error)
	// ListCandidates 分页列出候选人（面试官浏览），可按状态与关键词（学号/姓名/简介）筛选。
	ListCandidates(ctx context.Context, status dsmodel.CandidateStatus, q string, limit, offset int) ([]*dsmodel.Candidate, error)
	// ListWaitingCandidates 分页列出候场大屏名单：只含「未定局」的档位
	// （见 model.CandidateStatus.Terminal：面试已结束及其后的录取档不在其中），可按关键词筛选。
	ListWaitingCandidates(ctx context.Context, q string, limit, offset int) ([]*dsmodel.Candidate, error)
	// ListMessagesAfter 返回房间内 id>afterID 的消息（断线续传增量）。
	ListMessagesAfter(ctx context.Context, roomID uint64, afterID uint64) ([]*dsmodel.Message, error)

	// ---- 写：原子业务操作（Q2=B），先落库后返回事件 ----

	// CheckIn 候选人签到：NOT_CHECKED_IN -> CHECKED_IN_PENDING_ASSIGN。
	CheckIn(ctx context.Context, candidateID uint64) (*Event, error)
	// CreateCandidate 新建候选人（初始状态 NOT_CHECKED_IN），返回创建事件（载荷 CandidateRef）。
	// 学号必填且唯一（纯数字，见 model.ValidateStudentNo）；重复 → ErrStudentNoExists。
	CreateCandidate(ctx context.Context, info CandidateInfo) (*Event, error)
	// ImportCandidates 批量导入候选人（管理员数据导入的落库端）：按学号 upsert，
	// 单事务【全或无】——任一行不合法即整批不落库并返回行级错误报告（*ImportError）；
	// 批内重复学号后者覆盖前者；只写资料字段（CandidateInfo）；
	// 候选人的运行态（状态机 / 房间绑定 / 消息 / 录取决定 / 出价）一律不动。
	// 行带 UpdatedAt 且早于库中记录的 updated_at 时为【过期行】：跳过不覆盖（报告 status=skipped），
	// 避免重复导入旧表格冲掉系统内的改动。
	ImportCandidates(ctx context.Context, rows []CandidateImportRow) (*ImportReport, error)
	// MovePhase 推进阶段：ASSIGNED -> IN_PROGRESS -> COMPLETED。
	// 由当前房间成员调用（无主持人概念，成员即可推进）。
	// 推进到面试结档档位（COMPLETED 及其后的录取档）自动解绑房间（rooms.candidate_id 置空，
	// 绑定唯一权威）并留档「在哪间房间面的」；房间转空闲可拉取下一候选人，
	// 消息仍按候选人归档保留、此后只读（见 AppendMessage）。
	MovePhase(ctx context.Context, roomID, operatorID uint64, to dsmodel.CandidateStatus) (*Event, error)
	// AppendMessage 在房间内追加一条聊天消息，返回事件(带 MsgID)。
	// 消息只写给「正在面试」（ASSIGNED / IN_PROGRESS）的候选人：空房间 → ErrNotFound，
	// 面试已结档（COMPLETED 及其后的录取档）→ ErrInterviewFinished（记录只读归档）。
	// 内容空白 → ErrInvalidContent。房间内的实时会话走这里；归档补充走 AppendCandidateMessage。
	AppendMessage(ctx context.Context, roomID, senderID uint64, content string) (*Event, error)
	// AppendCandidateMessage 直接向候选人的面试记录归档追加一条消息（返回事件，带 MsgID）。
	// 与 AppendMessage 的区别是**不需要房间与在场成员**：面试结档后房间已解绑，
	// 但记录仍可在候选人查看页补充（这正是归档存在的意义），故只要求候选人存在。
	// 候选人不存在 → ErrNotFound；内容空白 → ErrInvalidContent。
	// 候选人当前若仍在房间内，事件按该房间扇出（房间页同步可见），否则全局扇出。
	AppendCandidateMessage(ctx context.Context, candidateID, senderID uint64, content string) (*Event, error)
	// EditMessage 编辑一条面试记录：**仅发送者本人**，且距发送不超过 MessageModifyWindow。
	// content 为新正文（TrimSpace；空白 → ErrInvalidContent）。窗口自消息创建时刻起算，编辑不延长窗口。
	// 消息不属于该候选人 / 不存在 → ErrNotFound；非本人 → ErrMessageNotOwner；超窗口 → ErrMessageWindowExpired。
	EditMessage(ctx context.Context, candidateID, messageID, editorID uint64, content string) (*Event, error)
	// DeleteMessage 撤回一条面试记录（物理删除，归档与实时两路都会收到 message_deleted）。
	// 权限与窗口同 EditMessage；编辑/撤回都不改发送者与创建时刻（窗口口径稳定）。
	DeleteMessage(ctx context.Context, candidateID, messageID, operatorID uint64) (*Event, error)
	// SetMessageReaction 开关一条记录的表情回复（on=true 加上、false 撤回；两者都幂等，
	// 重复提交同一状态不产生变化也不广播）。表情须在 model.ReactionEmojis 允许集内
	// （否则 ErrInvalidReaction）；不设时间窗口——回复是档案之外的旁注，任何档位、任何时候都可加。
	// 消息不属于该候选人 / 不存在 → ErrNotFound。
	SetMessageReaction(ctx context.Context, candidateID, messageID, userID uint64, emoji string, on bool) (*Event, error)
	// JoinRoom 面试官加入房间（返回房间快照用于首次同步 + 成员变更事件）。
	JoinRoom(ctx context.Context, roomID, userID uint64) (*dsmodel.Room, *Event, error)
	// LeaveRoom 面试官离开房间（返回最后一个离开者时会额外产出成员变更事件）。
	LeaveRoom(ctx context.Context, roomID, userID uint64) (*Event, error)

	// ---- 用户与角色管理（RBAC，先落库后由调用方重载 RBAC 缓存） ----

	// CreateUser 新建面试官（含角色分配），返回用户 id。
	CreateUser(ctx context.Context, u *dsmodel.User, roleIDs []uint64) (uint64, error)
	// GetUser 返回用户（含角色、部门）。
	GetUser(ctx context.Context, id uint64) (*dsmodel.User, error)
	// ListUsers 分页列出用户（可按 username/name 关键词搜索），每项含角色、部门。
	ListUsers(ctx context.Context, q string, limit, offset int) ([]*dsmodel.User, error)
	// UpdateUser 更新用户名（登录凭证）、显示名、部门与角色分配。
	UpdateUser(ctx context.Context, id uint64, username, name string, departmentID *uint64, roleIDs []uint64) error
	// SetUserRoles 覆盖用户的角色分配。
	SetUserRoles(ctx context.Context, userID uint64, roleIDs []uint64) error
	// ResetUserPassword 重置用户密码（bump token_version，旧 token 即时失效）。
	ResetUserPassword(ctx context.Context, id uint64, hash string) error
	// DeleteUser 删除面试官（room_members 由 FK CASCADE；消息因归候选人不受影响）。
	DeleteUser(ctx context.Context, id uint64) error

	// ListRoles 列出全部角色（含权限组）。
	ListRoles(ctx context.Context) ([]*dsmodel.Role, error)
	// CreateRole 新建角色（权限组）。
	CreateRole(ctx context.Context, name, desc string, perms []string) (uint64, error)
	// UpdateRole 更新角色名与权限组（权限变更后需 ReloadAll 缓存）。
	UpdateRole(ctx context.Context, id uint64, name, desc string, perms []string) error
	// DeleteRole 删除角色（被用户引用时拒绝）。
	DeleteRole(ctx context.Context, id uint64) error

	// ---- 单点登录（OIDC）配置 ----

	// GetOidcConfig 返回 OIDC 单行配置（不存在时按默认值懒建）。
	// RoleRules / DepartmentRules 均按 position 升序填充。
	GetOidcConfig(ctx context.Context) (*dsmodel.OidcConfig, error)
	// SetOidcConfig 覆盖保存 OIDC 配置与两类规则（单事务，先落库成功）。
	// clientSecret 为 nil 表示保持原密钥不变，"" 表示清除，非空表示覆盖。
	// 校验失败返回 *Error：oidc_issuer_invalid / oidc_client_id_required /
	// oidc_redirect_url_invalid / oidc_scopes_invalid / oidc_rule_invalid / oidc_rule_role_missing /
	// oidc_dept_rule_invalid / oidc_dept_rule_department_missing。
	SetOidcConfig(ctx context.Context, cfg *dsmodel.OidcConfig, clientSecret *string) error
	// FindUserByOidcSubject 按 IdP 主体标识查用户；不存在 → ErrNotFound。
	FindUserByOidcSubject(ctx context.Context, subject string) (*dsmodel.User, error)
	// SyncOidcUser 同步 OIDC 用户的显示名、角色与部门（IdP 为权威，每次登录覆盖）。
	// departmentID 为 nil 表示未命中部门规则 → 不改动账号现有部门。
	SyncOidcUser(ctx context.Context, id uint64, name string, roleIDs []uint64, departmentID *uint64) error

	// ---- 部门管理 ----

	// ListDepartments 列出全部部门（含预期人数与面试官数）。
	ListDepartments(ctx context.Context) ([]*dsmodel.Department, error)
	// CreateDepartment 新建部门（name 必填，expectedCount 为预期人数），返回 id。
	CreateDepartment(ctx context.Context, name, desc string, expectedCount int) (uint64, error)
	// UpdateDepartment 更新部门名称、描述与预期人数。
	UpdateDepartment(ctx context.Context, id uint64, name, desc string, expectedCount int) error
	// DeleteDepartment 删除部门（仍有面试官归属时拒绝）。
	DeleteDepartment(ctx context.Context, id uint64) error

	// ---- 可观测性（OTLP → GreptimeDB） ----

	// GetObservabilityConfig 返回可观测性（OTLP → GreptimeDB）单行配置（不存在时按默认值懒建）。
	GetObservabilityConfig(ctx context.Context) (*dsmodel.ObservabilityConfig, error)
	// SetObservabilityConfig 覆盖保存可观测性配置；password 为 nil 表示保持原密码不变，
	// "" 表示清除，非空表示覆盖。校验失败返回 *Error：observability_endpoint_invalid。
	// 不发事件（配置面向管理员面板，前端写后自拉）。
	SetObservabilityConfig(ctx context.Context, cfg *dsmodel.ObservabilityConfig, password *string) error

	// AdjustWaitingPriority 候场队列手动调序：dir = "up"/"down"，与同档相邻一位交换先后
	// （waiting_priority：null 视为未调整、排在显式序之后；首次调序把该档固化成 1..n，
	// 整档固化在单事务内完成）。只允许「已签到待分配」档，其余档位返回 *Error{invalid_status}；
	// 已在档首/档尾是幂等 no-op（moved = false、不写库不广播）。
	// moved 表示是否真的换了位；ev 为 nil 表示没有状态变更（不广播）。
	AdjustWaitingPriority(ctx context.Context, id uint64, dir string) (moved bool, ev *Event, err error)
	// GetSystemStatus 返回当前系统阶段（面试/录取/捡漏）。
	GetSystemStatus(ctx context.Context) (*dsmodel.SystemStatus, error)
	// SetSystemStatus 切换系统阶段（仅面试/录取/捡漏/结算四档）。
	SetSystemStatus(ctx context.Context, phase dsmodel.SystemPhase) error
	// SetBidStep 设置出价步长（≥1）。
	SetBidStep(ctx context.Context, step int) error

	// ---- 候选人管理 ----

	// UpdateCandidate 编辑候选人资料字段（CandidateInfo 全量覆盖），返回更新事件（载荷 CandidateRef）。
	// 学号可改（改到他人已占用的学号 → ErrStudentNoExists）；学号是身份键，
	// 修改不影响候选人的运行态（房间绑定 / 消息 / 录取决定 / 出价均随 id 保留）。
	UpdateCandidate(ctx context.Context, id uint64, info CandidateInfo) (*Event, error)
	// UpdateCandidatePreferences 只更新志愿与调剂三列（不触碰其他资料与运行态），
	// 返回更新事件（载荷 CandidateRef）；候选人不存在 → ErrNotFound。
	UpdateCandidatePreferences(ctx context.Context, id uint64, prefs CandidatePreferences) (*Event, error)
	// DeleteCandidate 删除候选人：连带删其消息档案并解绑房间（房间保留为空记录），
	// 返回删除事件（载荷 CandidateRef）。
	DeleteCandidate(ctx context.Context, id uint64) (*Event, error)
	// ResetCandidateStatus 重置候选人到状态机任意档：
	// 向后档（未签到/已签到待分配）自动解绑房间；向前档须已有房间绑定。
	// 面试结档档位（COMPLETED 及其后的待录取 / 已录取）一律自动解绑房间（候选人与房间解除关联），
	// 使得房间腾出来、面试记录转为只读归档；向 COMPLETED 时额外留档「在哪间房间面的」。
	ResetCandidateStatus(ctx context.Context, id uint64, to dsmodel.CandidateStatus) (*Event, error)
	// ListCandidateAdmissions 返回部门对候选人的录取决定。
	// departmentID 为 nil 时返回所有部门的记录（跨部门查看）；否则仅返回指定部门。
	ListCandidateAdmissions(ctx context.Context, departmentID *uint64) ([]*dsmodel.CandidateAdmission, error)
	// UpsertCandidateAdmission 记录/更新某部门对候选人的录取决定（按 candidate+department upsert）。
	UpsertCandidateAdmission(ctx context.Context, candidateID, departmentID uint64, status dsmodel.AdmissionStatus) error

	// ---- 捡漏阶段（按预算竞拍） ----

	// LeftoverOverview 返回捡漏总览：各部门预算。
	// myDepartmentID 为当前用户部门（nil 表示无部门，my 返回 nil）；
	// exposeAll 为 true 时所有部门的 spent/remaining 公开（持 candidates.browse_all 的管理端），
	// 否则仅本部门可见（出价保密）。
	LeftoverOverview(ctx context.Context, myDepartmentID *uint64, exposeAll bool) (*dsmodel.LeftoverOverview, error)
	// ListLeftoverBids 返回出价列表：departmentID 为 nil 时返回全部部门（管理端跨部门查看），
	// 否则仅返回指定部门。
	ListLeftoverBids(ctx context.Context, departmentID *uint64) ([]*dsmodel.Bid, error)
	// UpsertLeftoverBid 记录/覆盖本部门对候选人的出价（仅捡漏阶段、受剩余预算约束）。
	// 返回事件（载荷 LeftoverRef，不含金额）。
	UpsertLeftoverBid(ctx context.Context, candidateID, departmentID uint64, amount int) (*Event, error)
	// LeftoverFinalResults 只读计算各候选人的最终录取结果（赢家 = 最高出价部门，
	// 同额取先出价者；Resolved 标记是否已正式落库）。
	// 保密语义与出价一致：默认仅返回已成交或赢家为本部门的行；exposeAll 全量。
	LeftoverFinalResults(ctx context.Context, myDepartmentID *uint64, exposeAll bool) ([]*dsmodel.LeftoverFinalResult, error)
	// ListLeftoverResults 返回全部已结算候选人的赢家与成交金额（全员可见）。
	ListLeftoverResults(ctx context.Context) ([]*dsmodel.LeftoverResult, error)

	// ---- 房间管理 ----

	// CreateRoom 手动创建房间（name 可选别名，空串=未命名），返回创建事件（载荷 RoomRef）。
	CreateRoom(ctx context.Context, name string) (*Event, error)
	// DeleteRoom 删除空房间（无候选人绑定、无成员时允许），返回删除事件（载荷 RoomRef）。
	DeleteRoom(ctx context.Context, id uint64) (*Event, error)
	// RenameRoom 修改房间名（空串=清除命名），返回改名事件（载荷 RoomRef）。
	RenameRoom(ctx context.Context, id uint64, name string) (*Event, error)
	// PullCandidate 房间内面试官拉取候选人：CHECKED_IN_PENDING_ASSIGN -> ASSIGNED 并绑定房间。
	PullCandidate(ctx context.Context, roomID, candidateID uint64) (*Event, error)

	// ---- 事件游标 & 订阅（重连/广播用） ----

	// Subscribe 订阅某个房间自 startAfterSeq 之后的事件流。
	// 返回接收 channel 与取消函数；客户端据此续传对齐。
	Subscribe(roomID uint64, startAfterSeq uint64) (<-chan *Event, func())
}

// Error 提供带业务语义的状态错误。
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Code + ": " + e.Msg }

// 常用状态错误。
var (
	ErrNotFound        = &Error{Code: "not_found", Msg: "resource not found"}
	ErrIllegalStatus   = &Error{Code: "illegal_status", Msg: "illegal status transition"}
	ErrRoomFull        = &Error{Code: "room_full", Msg: "room is full"}
	ErrNotMember       = &Error{Code: "not_member", Msg: "operator is not a room member"}
	ErrAlreadyAssigned = &Error{Code: "already_assigned", Msg: "candidate already assigned"}
	ErrUserInRoom      = &Error{Code: "user_in_room", Msg: "user already in an active room"}
	// ErrInterviewFinished 这段面试已结档（候选人已完成及其后的录取档），房间消息通道关闭，
	// 面试记录只读归档（归档补充走 AppendCandidateMessage，不受此限）。
	ErrInterviewFinished = &Error{Code: "interview_finished", Msg: "面试已结束，不能再发送消息"}
	// ErrInvalidContent 消息内容为空（去除首尾空白后）。
	ErrInvalidContent = &Error{Code: "invalid_content", Msg: "消息内容不能为空"}
	// ErrMessageNotOwner 只能编辑/撤回自己发送的消息（发送者已被删除的消息同属此列，无从归属）。
	ErrMessageNotOwner = &Error{Code: "message_not_owner", Msg: "只能编辑或撤回自己发送的消息"}
	// ErrMessageWindowExpired 超过 MessageModifyWindow 后不能再编辑/撤回。
	ErrMessageWindowExpired = &Error{Code: "message_window_expired", Msg: "超过 2 分钟，不能再编辑或撤回"}
	// ErrInvalidReaction 表情不在允许集内（model.ReactionEmojis）。
	ErrInvalidReaction = &Error{Code: "invalid_reaction", Msg: "不支持的表情回复"}
	// ErrStudentNoExists 学号已被其他候选人占用（候选人身份键唯一，编辑/新增均返回此错）。
	ErrStudentNoExists = &Error{Code: "student_no_exists", Msg: "学号已存在"}
)

// MessageModifyWindow 面试记录的编辑/撤回窗口：自发送时刻起 2 分钟，超时后记录定格。
// 权威判定在 EditMessage / DeleteMessage；前端 `domain/messages.ts#MESSAGE_MODIFY_WINDOW_MS`
// 是同一口径的镜像，只用于是否显示入口（服务端永远复核）。
const MessageModifyWindow = 2 * time.Minute

// MaxMessageContentLen 单条消息（实时与归档共用）正文的字节上限。
// 与 WS 帧限 maxMsgSize 同档：REST 录入路径没有 WS 帧限兜底，此处统一把关。
const MaxMessageContentLen = 4096

// MaxImportRows 单次导入的行数上限（前端亦按此预检，服务端兜底）。
const MaxImportRows = 2000

// CandidateInfo 是候选人的资料字段全集（新建/编辑/导入共用；运行态不在此列）。
// 学号为身份键（必填、纯数字、唯一）；志愿与联系方式均为可选，落库前统一 TrimSpace。
type CandidateInfo struct {
	StudentNo    string `json:"student_no"`
	Name         string `json:"name"`
	Profile      string `json:"profile"`
	FirstChoice  string `json:"first_choice"`  // 第一志愿
	SecondChoice string `json:"second_choice"` // 第二志愿
	AcceptAdjust bool   `json:"accept_adjust"` // 是否接受调剂
	Phone        string `json:"phone"`         // 手机号
	QQ           string `json:"qq"`            // QQ 号
	Email        string `json:"email"`         // 邮箱
}

// CandidateImportRow 是批量导入的一行输入：前端在浏览器内解析 Excel 并映射后的规范化字段
// （服务端不解析表格，只做校验与落库）。内嵌 CandidateInfo，JSON 仍为扁平字段。
type CandidateImportRow struct {
	CandidateInfo
	// UpdatedAt 源表给出的「该行更新时间」（可选，RFC3339）：仅当它【早于】库中记录的
	// updated_at 时才说明这行是旧数据 —— 该行跳过不覆盖，避免用旧表格冲掉系统内的改动。
	// 零值（源表没有这一列 / 单元格为空）→ 不做比较，与历史行为一致：全量覆盖。
	UpdatedAt *time.Time `json:"updated_at"`
}

// CandidatePreferences 是候选人的志愿与调剂三项（独立于资料全量编辑：
// 持 candidates.preferences 的面试官可改这三项，无法触碰姓名/学号/联系方式与运行态）。
type CandidatePreferences struct {
	FirstChoice  string `json:"first_choice"`  // 第一志愿
	SecondChoice string `json:"second_choice"` // 第二志愿
	AcceptAdjust bool   `json:"accept_adjust"` // 是否接受调剂
}

// ImportOutcome 单行导入结果（status: created / updated / skipped）。
type ImportOutcome struct {
	Index       int    `json:"index"`
	Status      string `json:"status"`
	CandidateID uint64 `json:"candidate_id"`
	// StoredUpdatedAt 库中该候选人当前的更新时间：仅 skipped 时有值（说明为何跳过）。
	StoredUpdatedAt *time.Time `json:"stored_updated_at,omitempty"`
}

// 导入行状态取值。
const (
	ImportStatusCreated = "created" // 新建
	ImportStatusUpdated = "updated" // 命中既有学号，覆盖资料
	// ImportStatusSkipped 命中既有学号，但源表该行的更新时间早于库中记录 → 跳过：
	// 不写库、不广播事件（该行在报告里带上库中的更新时间供界面说明）。
	ImportStatusSkipped = "skipped"
)

// ImportReport 批量导入的落库报告。
// Events 承载本批已落库事件（json:"-"），由 service 层负责【落库后】扇出。
type ImportReport struct {
	Created int             `json:"created"`
	Updated int             `json:"updated"`
	Skipped int             `json:"skipped"`
	Rows    []ImportOutcome `json:"rows"`
	Events  []*Event        `json:"-"`
}

// RowError 导入的行级错误（Index 为请求体中的行下标）。
type RowError struct {
	Index int    `json:"index"`
	Msg   string `json:"error"`
}

// ImportError 批量导入的校验失败（全或无：任一行不合法即整批不落库，
// 报告一次列出全部问题行，便于改完重传）。
type ImportError struct {
	Rows []RowError
}

func (e *ImportError) Error() string { return "导入数据校验未通过" }
