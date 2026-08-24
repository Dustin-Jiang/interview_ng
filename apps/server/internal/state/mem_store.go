package state

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	"gorm.io/gorm"

	dsmodel "interview_ng/internal/model"
)

// MemStateStore 是 StateStore 的内存实现（Q1=A）。
// - 状态权威在内存在线副本，Postgres/Gorm 是持久化层；
// - 所有写操作在一个互斥临界区内【先落库成功后】才产出 Event（先落库后广播）；
// - 每个房间维护一个事件订阅总线，供 BroadcastManager 扇出、供重连续传对齐。
type MemStateStore struct {
	mu   sync.Mutex
	db   *gorm.DB
	seq  uint64                            // 全局事件序号
	subs map[uint64]map[uint64]chan *Event // roomID -> subID -> channel
	subN uint64
}

// NewMemStateStore 构建内存 StateStore。
func NewMemStateStore(db *gorm.DB) *MemStateStore {
	return &MemStateStore{
		db:   db,
		subs: make(map[uint64]map[uint64]chan *Event),
	}
}

//---- 内部辅助 ----

// nextSeq 产生全局事件序号。
func (s *MemStateStore) nextSeq() uint64 { return atomic.AddUint64(&s.seq, 1) }

// emit 在锁内调用：生成事件并投递到该房间所有订阅者。
func (s *MemStateStore) emit(roomID uint64, ev *Event) {
	ev.RoomID = roomID
	ev.Seq = s.nextSeq()
	s.deliver(roomID, ev)
}

func (s *MemStateStore) deliver(roomID uint64, ev *Event) {
	for _, ch := range s.subs[roomID] {
		select {
		case ch <- ev:
		default: // 慢消费者丢事件（由重连续传补齐）
		}
	}
}

func (s *MemStateStore) ensureRoom(ctx context.Context, id uint64) (*dsmodel.Room, error) {
	var r dsmodel.Room
	err := s.db.WithContext(ctx).Preload("Candidate").Preload("Members.User").First(&r, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	decorateRoom(&r)
	return &r, nil
}

// decorateRoom 补齐房间内绑定候选人的 RoomID 投影（房间侧 rooms.candidate_id 是绑定的唯一权威）。
func decorateRoom(r *dsmodel.Room) {
	if r.Candidate != nil && r.Candidate.RoomID == nil {
		rid := r.ID
		r.Candidate.RoomID = &rid
	}
}

// roomByCandidate 返回当前绑定该候选人的房间；未绑定返回 ErrNotFound。
func (s *MemStateStore) roomByCandidate(ctx context.Context, candidateID uint64) (*dsmodel.Room, error) {
	var r dsmodel.Room
	err := s.db.WithContext(ctx).Where("candidate_id = ?", candidateID).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// fillRoomID 单个候选人补齐 RoomID 投影。
func (s *MemStateStore) fillRoomID(ctx context.Context, c *dsmodel.Candidate) {
	room, err := s.roomByCandidate(ctx, c.ID)
	if err == nil {
		rid := room.ID
		c.RoomID = &rid
	}
}

func (s *MemStateStore) ensureCandidate(ctx context.Context, id uint64) (*dsmodel.Candidate, error) {
	var c dsmodel.Candidate
	if err := s.db.WithContext(ctx).First(&c, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

//---- 读 ----

func (s *MemStateStore) GetCandidate(ctx context.Context, id uint64) (*dsmodel.Candidate, error) {
	c, err := s.ensureCandidate(ctx, id)
	if err != nil {
		return nil, err
	}
	s.fillRoomID(ctx, c)
	return c, nil
}

func (s *MemStateStore) GetCandidateByRoom(ctx context.Context, roomID uint64) (*dsmodel.Candidate, error) {
	room, err := s.ensureRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if room.CandidateID == nil {
		return nil, ErrNotFound
	}
	return s.ensureCandidate(ctx, *room.CandidateID)
}

func (s *MemStateStore) GetRoom(ctx context.Context, roomID uint64) (*dsmodel.Room, error) {
	return s.ensureRoom(ctx, roomID)
}

func (s *MemStateStore) ListRooms(ctx context.Context, limit, offset int) ([]*dsmodel.Room, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []*dsmodel.Room
	if err := s.db.WithContext(ctx).
		Preload("Candidate").
		Preload("Members.User").
		Order("id asc").
		Limit(limit).Offset(offset).
		Find(&out).Error; err != nil {
		return nil, err
	}
	for i := range out {
		decorateRoom(out[i])
	}
	return out, nil
}

func (s *MemStateStore) ListCandidates(ctx context.Context, status dsmodel.CandidateStatus, q string, limit, offset int) ([]*dsmodel.Candidate, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	qdb := s.db.WithContext(ctx).Model(&dsmodel.Candidate{})
	if status != "" {
		qdb = qdb.Where("status = ?", status)
	}
	if q != "" {
		like := "%" + q + "%"
		qdb = qdb.Where("name LIKE ? OR profile LIKE ?", like, like)
	}
	var out []*dsmodel.Candidate
	if err := qdb.Order("id asc").Limit(limit).Offset(offset).Find(&out).Error; err != nil {
		return nil, err
	}
	if len(out) > 0 {
		ids := make([]uint64, 0, len(out))
		for _, c := range out {
			ids = append(ids, c.ID)
		}
		var rooms []dsmodel.Room
		if err := s.db.WithContext(ctx).Select("id", "candidate_id").
			Where("candidate_id IN ?", ids).Find(&rooms).Error; err != nil {
			return nil, err
		}
		roomByCand := make(map[uint64]uint64, len(rooms))
		for _, r := range rooms {
			if r.CandidateID != nil {
				roomByCand[*r.CandidateID] = r.ID
			}
		}
		for _, c := range out {
			if rid, ok := roomByCand[c.ID]; ok {
				c.RoomID = &rid
			}
		}
	}
	return out, nil
}

func (s *MemStateStore) ListMessagesAfter(ctx context.Context, candidateID uint64, afterID uint64) ([]*dsmodel.Message, error) {
	var out []*dsmodel.Message
	// 预加载 Sender：归档查看需展示面试官姓名；sender 已删时为 nil（前端显示「已删除用户」）。
	err := s.db.WithContext(ctx).
		Preload("Sender").
		Where("candidate_id = ? AND id > ?", candidateID, afterID).
		Order("id asc").
		Find(&out).Error
	return out, err
}

//---- 写 ----

func (s *MemStateStore) CheckIn(ctx context.Context, candidateID uint64) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.ensureCandidate(ctx, candidateID)
	if err != nil {
		return nil, err
	}
	if err := guardTransition(c.Status, dsmodel.StatusCheckedInPendingAssign); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(c).Updates(map[string]any{
		"status": dsmodel.StatusCheckedInPendingAssign,
	}).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventCandidateSignedIn, Data: struct{ CandidateID uint64 }{candidateID}}
	s.emit(0, ev)
	return ev, nil
}

func (s *MemStateStore) CreateCandidate(ctx context.Context, name, profile string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := &dsmodel.Candidate{Name: name, Profile: profile, Status: dsmodel.StatusNotCheckedIn}
	if err := s.db.WithContext(ctx).Create(c).Error; err != nil {
		return 0, err
	}
	return c.ID, nil
}

func (s *MemStateStore) MovePhase(ctx context.Context, roomID, operatorID uint64, to dsmodel.CandidateStatus) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, err := s.ensureRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	// 无主持人概念：房间成员即可推进阶段。
	if !s.isMemberLocked(ctx, roomID, operatorID) {
		return nil, ErrNotMember
	}
	c, err := s.ensureCandidate(ctx, *room.CandidateID)
	if err != nil {
		return nil, err
	}
	if err := guardTransition(c.Status, to); err != nil {
		return nil, err
	}
	// 推进到 COMPLETED（完成）即自动清房：房间解绑候选人（rooms.candidate_id 置空），
	// 房间转空闲可继续拉取下一位；消息仍按候选人归档保留。
	updates := map[string]any{"status": to}
	if to == dsmodel.StatusCompleted {
		if err := s.db.WithContext(ctx).Model(&dsmodel.Room{}).Where("id = ?", roomID).
			UpdateColumn("candidate_id", nil).Error; err != nil {
			return nil, err
		}
	}
	if err := s.db.WithContext(ctx).Model(c).Updates(updates).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventRoomPhaseChanged, Data: struct {
		RoomID      uint64
		CandidateID uint64
		To          dsmodel.CandidateStatus
	}{roomID, *room.CandidateID, to}}
	s.emit(roomID, ev)
	return ev, nil
}

func (s *MemStateStore) AppendMessage(ctx context.Context, roomID, senderID uint64, content string) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, err := s.ensureRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	// 消息按候选人归属：空房间（无候选人）无消息主体，拒绝发送。
	if room.CandidateID == nil {
		return nil, ErrNotFound
	}
	if !s.isMemberLocked(ctx, roomID, senderID) {
		return nil, ErrNotMember
	}
	msg := &dsmodel.Message{CandidateID: *room.CandidateID, SenderID: &senderID, Content: content}
	if err := s.db.WithContext(ctx).Create(msg).Error; err != nil {
		return nil, err
	}
	// 实时事件携带发送者展示名，前端不必依赖二次查询即可显示面试官姓名。
	senderName := ""
	var u dsmodel.User
	if err := s.db.WithContext(ctx).First(&u, senderID).Error; err == nil {
		senderName = u.Name
		if senderName == "" {
			senderName = u.Username
		}
	}
	ev := &Event{Type: EventMessageAppended, RoomID: roomID, MsgID: msg.ID,
		Data: struct {
			RoomID      uint64
			CandidateID uint64
			SenderID    uint64
			SenderName  string
			Content     string
		}{roomID, *room.CandidateID, senderID, senderName, content}}
	s.emit(roomID, ev)
	return ev, nil
}

func (s *MemStateStore) JoinRoom(ctx context.Context, roomID, userID uint64) (*dsmodel.Room, *Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, err := s.ensureRoom(ctx, roomID)
	if err != nil {
		return nil, nil, err
	}
	// 一次一个房间：该用户不得已在其他活跃房间。
	var existing int64
	if err := s.db.WithContext(ctx).Model(&dsmodel.RoomMember{}).
		Where("user_id = ? AND room_id <> ?", userID, roomID).Count(&existing).Error; err != nil {
		return nil, nil, err
	}
	if existing > 0 {
		return nil, nil, ErrUserInRoom
	}
	// 幂等重连：若已是本房间成员，直接返回现有快照，不重复创建（避免唯一约束冲突）。
	already := s.isMemberLocked(ctx, roomID, userID)
	if already {
		room, _ = s.ensureRoom(ctx, roomID)
		return room, nil, nil
	}
	member := &dsmodel.RoomMember{RoomID: roomID, UserID: userID}
	if err := s.db.WithContext(ctx).Create(member).Error; err != nil {
		return nil, nil, err
	}
	ev := &Event{Type: EventMemberJoined, RoomID: roomID, Data: struct{ UserID uint64 }{userID}}
	s.emit(roomID, ev)
	// 返回最新快照用于首次同步。
	room, _ = s.ensureRoom(ctx, roomID)
	return room, ev, nil
}

func (s *MemStateStore) LeaveRoom(ctx context.Context, roomID, userID uint64) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.ensureRoom(ctx, roomID); err != nil {
		return nil, err
	}
	res := s.db.WithContext(ctx).Where("room_id = ? AND user_id = ?", roomID, userID).
		Delete(&dsmodel.RoomMember{})
	if res.Error != nil {
		return nil, res.Error
	}
	ev := &Event{Type: EventMemberLeft, RoomID: roomID, Data: struct{ UserID uint64 }{userID}}
	s.emit(roomID, ev)
	return ev, nil
}

//---- 用户与角色管理（RBAC） ----

func (s *MemStateStore) setRolesLocked(ctx context.Context, userID uint64, roleIDs []uint64) error {
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&dsmodel.UserRole{}).Error; err != nil {
		return err
	}
	for _, rid := range roleIDs {
		if err := s.db.WithContext(ctx).Create(&dsmodel.UserRole{UserID: userID, RoleID: rid}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *MemStateStore) fillRoles(ctx context.Context, u *dsmodel.User) error {
	var urs []dsmodel.UserRole
	if err := s.db.WithContext(ctx).Where("user_id = ?", u.ID).Find(&urs).Error; err != nil {
		return err
	}
	if len(urs) == 0 {
		return nil
	}
	ids := make([]uint64, 0, len(urs))
	for _, ur := range urs {
		ids = append(ids, ur.RoleID)
	}
	var roles []dsmodel.Role
	if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&roles).Error; err != nil {
		return err
	}
	u.Roles = roles
	return nil
}

// fillDepartment 单个用户补齐部门关联（department_id 为空则跳过）。
func (s *MemStateStore) fillDepartment(ctx context.Context, u *dsmodel.User) error {
	if u.DepartmentID == nil {
		return nil
	}
	var d dsmodel.Department
	if err := s.db.WithContext(ctx).First(&d, *u.DepartmentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // 部门已被删除，用户保持无部门展示
		}
		return err
	}
	u.Department = &d
	return nil
}

func (s *MemStateStore) CreateUser(ctx context.Context, u *dsmodel.User, roleIDs []uint64) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u.DepartmentID != nil {
		if err := s.ensureDepartment(ctx, *u.DepartmentID); err != nil {
			return 0, err
		}
	}
	if err := s.db.WithContext(ctx).Create(u).Error; err != nil {
		return 0, err
	}
	if err := s.setRolesLocked(ctx, u.ID, roleIDs); err != nil {
		return 0, err
	}
	return u.ID, nil
}

func (s *MemStateStore) GetUser(ctx context.Context, id uint64) (*dsmodel.User, error) {
	var u dsmodel.User
	if err := s.db.WithContext(ctx).First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := s.fillRoles(ctx, &u); err != nil {
		return nil, err
	}
	if err := s.fillDepartment(ctx, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *MemStateStore) ListUsers(ctx context.Context, q string, limit, offset int) ([]*dsmodel.User, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	qdb := s.db.WithContext(ctx).Model(&dsmodel.User{})
	if q != "" {
		like := "%" + q + "%"
		qdb = qdb.Where("username LIKE ? OR name LIKE ?", like, like)
	}
	var out []*dsmodel.User
	if err := qdb.Order("id asc").Limit(limit).Offset(offset).Find(&out).Error; err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.fillRoles(ctx, out[i]); err != nil {
			return nil, err
		}
		if err := s.fillDepartment(ctx, out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *MemStateStore) UpdateUser(ctx context.Context, id uint64, username, name string, departmentID *uint64, roleIDs []uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if departmentID != nil {
		if err := s.ensureDepartment(ctx, *departmentID); err != nil {
			return err
		}
	}
	// 用户名不可为空；且不可与其他用户冲突（登录凭证唯一）。
	username = strings.TrimSpace(username)
	if username == "" {
		return &Error{Code: "username_required", Msg: "用户名不能为空"}
	}
	var dup int64
	if err := s.db.WithContext(ctx).Model(&dsmodel.User{}).
		Where("username = ? AND id <> ?", username, id).Count(&dup).Error; err != nil {
		return err
	}
	if dup > 0 {
		return &Error{Code: "username_taken", Msg: "用户名已被使用"}
	}
	if err := s.db.WithContext(ctx).Model(&dsmodel.User{}).Where("id = ?", id).
		Update("username", username).Update("name", name).Update("department_id", departmentID).Error; err != nil {
		return err
	}
	return s.setRolesLocked(ctx, id, roleIDs)
}

func (s *MemStateStore) SetUserRoles(ctx context.Context, userID uint64, roleIDs []uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setRolesLocked(ctx, userID, roleIDs)
}

func (s *MemStateStore) ResetUserPassword(ctx context.Context, id uint64, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.WithContext(ctx).Model(&dsmodel.User{}).Where("id = ?", id).
		UpdateColumn("password_hash", hash).
		UpdateColumn("token_version", gorm.Expr("token_version + 1")).Error
}

func (s *MemStateStore) DeleteUser(ctx context.Context, id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var u dsmodel.User
	if err := s.db.WithContext(ctx).First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	// user_roles 显式清理；room_members 由 FK CASCADE；messages.sender 由 FK SET NULL（保留档案）。
	if err := s.db.WithContext(ctx).Where("user_id = ?", id).Delete(&dsmodel.UserRole{}).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(&dsmodel.User{}, id).Error
}

func (s *MemStateStore) setRolePermsLocked(ctx context.Context, roleID uint64, perms []string) error {
	if err := s.db.WithContext(ctx).Where("role_id = ?", roleID).Delete(&dsmodel.RolePermission{}).Error; err != nil {
		return err
	}
	for _, p := range perms {
		if err := s.db.WithContext(ctx).Create(&dsmodel.RolePermission{RoleID: roleID, Permission: p}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *MemStateStore) ListRoles(ctx context.Context) ([]*dsmodel.Role, error) {
	var out []*dsmodel.Role
	if err := s.db.WithContext(ctx).Preload("Permissions").Order("id asc").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *MemStateStore) CreateRole(ctx context.Context, name, desc string, perms []string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := &dsmodel.Role{Name: name, Description: desc}
	if err := s.db.WithContext(ctx).Create(r).Error; err != nil {
		return 0, err
	}
	if err := s.setRolePermsLocked(ctx, r.ID, perms); err != nil {
		return 0, err
	}
	return r.ID, nil
}

func (s *MemStateStore) UpdateRole(ctx context.Context, id uint64, name, desc string, perms []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.db.WithContext(ctx).Model(&dsmodel.Role{}).Where("id = ?", id).
		Updates(map[string]any{"name": name, "description": desc}).Error; err != nil {
		return err
	}
	return s.setRolePermsLocked(ctx, id, perms)
}

func (s *MemStateStore) DeleteRole(ctx context.Context, id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cnt int64
	if err := s.db.WithContext(ctx).Model(&dsmodel.UserRole{}).Where("role_id = ?", id).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return &Error{Code: "role_in_use", Msg: "角色仍被用户使用，无法删除"}
	}
	if err := s.db.WithContext(ctx).Where("role_id = ?", id).Delete(&dsmodel.RolePermission{}).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(&dsmodel.Role{}, id).Error
}

//---- 部门管理 ----

func (s *MemStateStore) ListDepartments(ctx context.Context) ([]*dsmodel.Department, error) {
	var out []*dsmodel.Department
	if err := s.db.WithContext(ctx).Order("id asc").Find(&out).Error; err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	// 部门下面试官数（按 department_id 分组统计）。
	var counts []struct {
		DepartmentID uint64
		Cnt          int64
	}
	if err := s.db.WithContext(ctx).Model(&dsmodel.User{}).
		Select("department_id, count(*) as cnt").
		Where("department_id IS NOT NULL").
		Group("department_id").Scan(&counts).Error; err != nil {
		return nil, err
	}
	byDept := make(map[uint64]int64, len(counts))
	for _, c := range counts {
		byDept[c.DepartmentID] = c.Cnt
	}
	for _, d := range out {
		d.MemberCount = byDept[d.ID]
	}
	return out, nil
}

func (s *MemStateStore) CreateDepartment(ctx context.Context, name, desc string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := &dsmodel.Department{Name: name, Description: desc}
	if err := s.db.WithContext(ctx).Create(d).Error; err != nil {
		return 0, err
	}
	return d.ID, nil
}

func (s *MemStateStore) UpdateDepartment(ctx context.Context, id uint64, name, desc string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.db.WithContext(ctx).Model(&dsmodel.Department{}).Where("id = ?", id).
		Updates(map[string]any{"name": name, "description": desc}).Error; err != nil {
		return err
	}
	return nil
}

func (s *MemStateStore) DeleteDepartment(ctx context.Context, id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cnt int64
	if err := s.db.WithContext(ctx).Model(&dsmodel.User{}).Where("department_id = ?", id).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return &Error{Code: "department_in_use", Msg: "部门仍被面试官使用，无法删除"}
	}
	if err := s.db.WithContext(ctx).Delete(&dsmodel.Department{}, id).Error; err != nil {
		return err
	}
	return nil
}

// ensureDepartment 校验部门存在（供用户创建/更新引用校验）。
func (s *MemStateStore) ensureDepartment(ctx context.Context, id uint64) error {
	var cnt int64
	if err := s.db.WithContext(ctx).Model(&dsmodel.Department{}).Where("id = ?", id).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt == 0 {
		return &Error{Code: "department_not_found", Msg: "部门不存在"}
	}
	return nil
}

//---- 候选人管理 ----

func (s *MemStateStore) UpdateCandidate(ctx context.Context, id uint64, name, profile string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.WithContext(ctx).Model(&dsmodel.Candidate{}).Where("id = ?", id).
		Updates(map[string]any{"name": name, "profile": profile}).Error
}

func (s *MemStateStore) DeleteCandidate(ctx context.Context, id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.ensureCandidate(ctx, id); err != nil {
		return err
	}
	// 解绑房间（房间保留为空记录；绑定的唯一权威在 rooms.candidate_id）
	if room, err := s.roomByCandidate(ctx, id); err == nil {
		if err := s.db.WithContext(ctx).Model(&dsmodel.Room{}).Where("id = ?", room.ID).
			UpdateColumn("candidate_id", nil).Error; err != nil {
			return err
		}
	}
	// 连带删消息档案（级联：删人即删其记录）
	if err := s.db.WithContext(ctx).Where("candidate_id = ?", id).Delete(&dsmodel.Message{}).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(&dsmodel.Candidate{}, id).Error
}

func (s *MemStateStore) ResetCandidateStatus(ctx context.Context, id uint64, to dsmodel.CandidateStatus) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.ensureCandidate(ctx, id)
	if err != nil {
		return nil, err
	}
	if to == c.Status || !validStatus(to) {
		return nil, ErrIllegalStatus
	}
	forward := to == dsmodel.StatusAssigned || to == dsmodel.StatusInProgress || to == dsmodel.StatusCompleted
	backward := to == dsmodel.StatusNotCheckedIn || to == dsmodel.StatusCheckedInPendingAssign

	var roomID uint64
	updates := map[string]any{"status": to}
	switch {
	case forward:
		room, err := s.roomByCandidate(ctx, id)
		if err != nil {
			return nil, &Error{Code: "no_room", Msg: "向前重置须先绑定房间"}
		}
		roomID = room.ID
		// 重置到 COMPLETED 与推进路径一致：完成即自动解绑房间（消息按候选人保留）。
		if to == dsmodel.StatusCompleted {
			if err := s.db.WithContext(ctx).Model(&dsmodel.Room{}).Where("id = ?", roomID).
				UpdateColumn("candidate_id", nil).Error; err != nil {
				return nil, err
			}
		}
	case backward:
		// 自动解绑房间（room 保留为空记录）；事件保持全局广播（回到排队池变化）。
		if room, err := s.roomByCandidate(ctx, id); err == nil {
			if err := s.db.WithContext(ctx).Model(&dsmodel.Room{}).Where("id = ?", room.ID).
				UpdateColumn("candidate_id", nil).Error; err != nil {
				return nil, err
			}
		}
	default:
		return nil, ErrIllegalStatus
	}
	if err := s.db.WithContext(ctx).Model(c).Updates(updates).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventRoomPhaseChanged, Data: struct {
		RoomID      uint64
		CandidateID uint64
		To          dsmodel.CandidateStatus
	}{roomID, id, to}}
	s.emit(roomID, ev)
	return ev, nil
}

//---- 房间管理 ----

func (s *MemStateStore) CreateRoom(ctx context.Context) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := &dsmodel.Room{}
	if err := s.db.WithContext(ctx).Create(r).Error; err != nil {
		return 0, err
	}
	return r.ID, nil
}

func (s *MemStateStore) DeleteRoom(ctx context.Context, id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, err := s.ensureRoom(ctx, id)
	if err != nil {
		return err
	}
	if room.CandidateID != nil {
		return &Error{Code: "room_not_empty", Msg: "房间仍绑定候选人"}
	}
	var members int64
	if err := s.db.WithContext(ctx).Model(&dsmodel.RoomMember{}).Where("room_id = ?", id).Count(&members).Error; err != nil {
		return err
	}
	if members > 0 {
		return &Error{Code: "room_not_empty", Msg: "房间仍有成员"}
	}
	return s.db.WithContext(ctx).Delete(&dsmodel.Room{}, id).Error
}

func (s *MemStateStore) PullCandidate(ctx context.Context, roomID, candidateID uint64) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, err := s.ensureRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if room.CandidateID != nil {
		return nil, ErrRoomFull
	}
	c, err := s.ensureCandidate(ctx, candidateID)
	if err != nil {
		return nil, err
	}
	if err := guardTransition(c.Status, dsmodel.StatusAssigned); err != nil {
		return nil, err
	}
	if _, err := s.roomByCandidate(ctx, candidateID); err == nil {
		return nil, ErrAlreadyAssigned
	}
	if err := s.db.WithContext(ctx).Model(&dsmodel.Room{}).Where("id = ?", roomID).
		UpdateColumn("candidate_id", candidateID).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(c).Update("status", dsmodel.StatusAssigned).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventCandidateAssigned, Data: struct {
		CandidateID uint64
		RoomID      uint64
	}{candidateID, roomID}}
	s.emit(roomID, ev)
	return ev, nil
}

//---- 订阅 ----

func (s *MemStateStore) Subscribe(roomID uint64, startAfterSeq uint64) (<-chan *Event, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan *Event, 256)
	s.subN++
	id := s.subN
	if s.subs[roomID] == nil {
		s.subs[roomID] = make(map[uint64]chan *Event)
	}
	s.subs[roomID][id] = ch
	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if m, ok := s.subs[roomID]; ok {
			delete(m, id)
			if len(m) == 0 {
				delete(s.subs, roomID)
			}
		}
	}
	return ch, cancel
}

//---- 内部辅助 ----

func (s *MemStateStore) isMemberLocked(ctx context.Context, roomID, userID uint64) bool {
	var count int64
	s.db.WithContext(ctx).Model(&dsmodel.RoomMember{}).
		Where("room_id = ? AND user_id = ?", roomID, userID).Count(&count)
	return count > 0
}

func guardTransition(from, to dsmodel.CandidateStatus) error {
	if from == to {
		// 严格状态机：重复提交同一转移判为非法，客户端不应重复投递同一转移。
		return ErrIllegalStatus
	}
	if !dsmodel.CanTransition(from, to) {
		return ErrIllegalStatus
	}
	return nil
}

// validStatus 判断是否为五档状态机中的合法状态。
func validStatus(s dsmodel.CandidateStatus) bool {
	for _, v := range dsmodel.ValidCandidateStatus() {
		if v == s {
			return true
		}
	}
	return false
}
