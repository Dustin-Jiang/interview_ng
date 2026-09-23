package state

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/oidcauth"
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
		qdb = qdb.Where("student_no LIKE ? OR name LIKE ? OR profile LIKE ?", like, like, like)
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
	// 预加载 Sender.Department：归档查看与房间聊天需在消息头部标出「面试官 · 部门」；
	// sender 已删时为 nil（前端显示「已删除用户」），部门为空时前端不显示头衔。
	err := s.db.WithContext(ctx).
		Preload("Sender.Department").
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
	now := time.Now()
	updates := statusUpdates(dsmodel.StatusCheckedInPendingAssign, now)
	// 签到时刻 = 本次签到的时刻（房间拉取列表与候场大屏按它先来后到）。
	// 不能改用 UpdatedAt：资料编辑/导入都会刷新它，一边排队一边导入就会打乱顺序。
	updates["checked_in_at"] = now
	if err := s.db.WithContext(ctx).Model(c).Updates(updates).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventCandidateSignedIn, Data: struct{ CandidateID uint64 }{candidateID}}
	s.emit(0, ev)
	return ev, nil
}

// normalizeCandidateInfo 归一化候选人资料字段：学号走统一校验（非法规格 → 400 态错），
// 文本字段裁剪首尾空白（空白即空串），bool 直存。
func normalizeCandidateInfo(info CandidateInfo) (dsmodel.Candidate, error) {
	no, err := normalizeStudentNo(info.StudentNo)
	if err != nil {
		return dsmodel.Candidate{}, err
	}
	return dsmodel.Candidate{
		StudentNo:    no,
		Name:         strings.TrimSpace(info.Name),
		Profile:      strings.TrimSpace(info.Profile),
		FirstChoice:  strings.TrimSpace(info.FirstChoice),
		SecondChoice: strings.TrimSpace(info.SecondChoice),
		AcceptAdjust: info.AcceptAdjust,
		Phone:        strings.TrimSpace(info.Phone),
		QQ:           strings.TrimSpace(info.QQ),
		Email:        strings.TrimSpace(info.Email),
	}, nil
}

func (s *MemStateStore) CreateCandidate(ctx context.Context, info CandidateInfo) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := normalizeCandidateInfo(info)
	if err != nil {
		return nil, err
	}
	taken, err := s.studentNoTakenLocked(ctx, c.StudentNo, 0)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, ErrStudentNoExists
	}
	c.Status = dsmodel.StatusNotCheckedIn
	if err := s.db.WithContext(ctx).Create(&c).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventCandidateCreated, Data: CandidateRef{c.ID}}
	s.emit(0, ev)
	return ev, nil
}

// ImportCandidates 批量导入候选人。三段式：
//  1. 整批校验（一次列出全部问题行，全或无）；
//  2. 单事务 upsert（按学号命中即更新资料列，未命中即新建；批内重复后者覆盖前者）；
//  3. 事务提交后逐条广播（先落库后广播）。
//
// 只写资料列（见 CandidateInfo）—— 候选人的运行态（状态机 / 房间绑定 / 消息 /
// 录取决定 / 出价）一律不动。
func (s *MemStateStore) ImportCandidates(ctx context.Context, rows []CandidateImportRow) (*ImportReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(rows) == 0 {
		return nil, &Error{Code: "import_empty", Msg: "导入数据为空"}
	}
	if len(rows) > MaxImportRows {
		return nil, &Error{Code: "import_too_large", Msg: fmt.Sprintf("单次导入最多 %d 行", MaxImportRows)}
	}

	// 1) 整批校验 + 归一化：学号必填且纯数字、姓名必填；文本字段裁剪空白；
	// 收集全部问题行（不是遇到第一个就返回）。
	nos := make([]string, len(rows))
	cols := make([]dsmodel.Candidate, len(rows))
	var rowErrs []RowError
	for i, r := range rows {
		no, err := dsmodel.ValidateStudentNo(r.StudentNo)
		if err != nil {
			rowErrs = append(rowErrs, RowError{Index: i, Msg: err.Error()})
			continue
		}
		name := strings.TrimSpace(r.Name)
		if name == "" {
			rowErrs = append(rowErrs, RowError{Index: i, Msg: "姓名不能为空"})
			continue
		}
		nos[i] = no
		cols[i] = dsmodel.Candidate{
			StudentNo:    no,
			Name:         name,
			Profile:      strings.TrimSpace(r.Profile),
			FirstChoice:  strings.TrimSpace(r.FirstChoice),
			SecondChoice: strings.TrimSpace(r.SecondChoice),
			AcceptAdjust: r.AcceptAdjust,
			Phone:        strings.TrimSpace(r.Phone),
			QQ:           strings.TrimSpace(r.QQ),
			Email:        strings.TrimSpace(r.Email),
		}
	}
	if len(rowErrs) > 0 {
		return nil, &ImportError{Rows: rowErrs}
	}

	// 2) 单事务落库：一次预取批内涉及的既有候选人（按学号），避免逐行点查。
	report := &ImportReport{Rows: make([]ImportOutcome, 0, len(rows))}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing []dsmodel.Candidate
		if err := tx.Where("student_no IN ?", nos).Find(&existing).Error; err != nil {
			return err
		}
		byNo := make(map[string]uint64, len(existing))
		// storedAt 是【导入开始那一刻】库中记录的更新时间快照，作为「源表该行是否过期」的比较基准。
		// 刻意不在写回后更新它：否则批内后行会拿刚写入的 now() 与自己的旧时间比，被误判为过期，
		// 「批内后者覆盖前者」的既有语义就被破坏了。
		storedAt := make(map[string]time.Time, len(existing))
		for i := range existing {
			byNo[existing[i].StudentNo] = existing[i].ID
			storedAt[existing[i].StudentNo] = existing[i].UpdatedAt
		}
		inBatch := make(map[string]uint64, len(rows))
		for i := range rows {
			no := nos[i]
			id, hit := inBatch[no]
			if !hit {
				id, hit = byNo[no]
			}
			if hit {
				// 过期行：源表给的更新时间早于库中记录 → 库里这条在导入之后被改过，
				// 用旧表格覆盖等于回退改动。跳过（不写、不广播），只如实报告。
				if base, ok := storedAt[no]; ok && rows[i].UpdatedAt != nil && rows[i].UpdatedAt.Before(base) {
					stored := base
					report.Skipped++
					report.Rows = append(report.Rows, ImportOutcome{
						Index: i, Status: ImportStatusSkipped, CandidateID: id, StoredUpdatedAt: &stored,
					})
					inBatch[no] = id
					continue
				}
				if err := tx.Model(&dsmodel.Candidate{}).Where("id = ?", id).
					Updates(map[string]any{
						"name":          cols[i].Name,
						"profile":       cols[i].Profile,
						"first_choice":  cols[i].FirstChoice,
						"second_choice": cols[i].SecondChoice,
						"accept_adjust": cols[i].AcceptAdjust,
						"phone":         cols[i].Phone,
						"qq":            cols[i].QQ,
						"email":         cols[i].Email,
					}).Error; err != nil {
					return err
				}
				inBatch[no] = id
				report.Updated++
				report.Rows = append(report.Rows, ImportOutcome{Index: i, Status: ImportStatusUpdated, CandidateID: id})
				report.Events = append(report.Events, &Event{Type: EventCandidateUpdated, Data: CandidateRef{id}})
				continue
			}
			c := cols[i]
			c.Status = dsmodel.StatusNotCheckedIn
			if err := tx.Create(&c).Error; err != nil {
				return err
			}
			inBatch[no] = c.ID
			report.Created++
			report.Rows = append(report.Rows, ImportOutcome{Index: i, Status: ImportStatusCreated, CandidateID: c.ID})
			report.Events = append(report.Events, &Event{Type: EventCandidateCreated, Data: CandidateRef{c.ID}})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 3) 事务已提交，逐条广播。
	for _, ev := range report.Events {
		s.emit(0, ev)
	}
	return report, nil
}

// normalizeStudentNo 归一化并校验学号，非法规格转成带业务码的状态错误（→ 400）。
func normalizeStudentNo(raw string) (string, error) {
	no, err := dsmodel.ValidateStudentNo(raw)
	if err != nil {
		return "", &Error{Code: "student_no_invalid", Msg: err.Error()}
	}
	return no, nil
}

// studentNoTakenLocked 判断学号是否已被其他候选人占用（excludeID=0 表示不限本人）。
// 调用方须持 s.mu：单写者下"先查后写"即为权威，DB 唯一索引只作兜底。
func (s *MemStateStore) studentNoTakenLocked(ctx context.Context, studentNo string, excludeID uint64) (bool, error) {
	q := s.db.WithContext(ctx).Model(&dsmodel.Candidate{}).Where("student_no = ?", studentNo)
	if excludeID != 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
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
	updates := statusUpdates(to, time.Now())
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
	// 实时事件携带发送者展示名与部门头衔，前端不必依赖二次查询即可在消息头部标出「谁 · 哪个部门」。
	senderName, senderDepartment := "", ""
	var u dsmodel.User
	if err := s.db.WithContext(ctx).Preload("Department").First(&u, senderID).Error; err == nil {
		senderName = u.Name
		if senderName == "" {
			senderName = u.Username
		}
		if u.Department != nil {
			senderDepartment = u.Department.Name
		}
	}
	ev := &Event{Type: EventMessageAppended, RoomID: roomID, MsgID: msg.ID,
		Data: struct {
			RoomID           uint64
			CandidateID      uint64
			SenderID         uint64
			SenderName       string
			SenderDepartment string
			Content          string
		}{roomID, *room.CandidateID, senderID, senderName, senderDepartment, content}}
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
	// 用户名是登录凭证，唯一（与 UpdateUser 一致：先查重给出明确业务错误码）。
	var dup int64
	if err := s.db.WithContext(ctx).Model(&dsmodel.User{}).
		Where("username = ?", u.Username).Count(&dup).Error; err != nil {
		return 0, err
	}
	if dup > 0 {
		return 0, &Error{Code: "username_taken", Msg: "用户名已被使用"}
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

func (s *MemStateStore) CreateDepartment(ctx context.Context, name, desc string, expectedCount int) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := &dsmodel.Department{Name: name, Description: desc, ExpectedCount: expectedCount}
	if err := s.db.WithContext(ctx).Create(d).Error; err != nil {
		return 0, err
	}
	return d.ID, nil
}

func (s *MemStateStore) UpdateDepartment(ctx context.Context, id uint64, name, desc string, expectedCount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.db.WithContext(ctx).Model(&dsmodel.Department{}).Where("id = ?", id).
		Updates(map[string]any{"name": name, "description": desc, "expected_count": expectedCount}).Error; err != nil {
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

//---- 单点登录（OIDC） ----

// oidcConfigID OIDC 配置单行记录的固定主键（ID 恒为 1）。
const oidcConfigID = 1

// ensureOidcConfig 读取 OIDC 配置行，不存在时按默认值落库（幂等初始化），并填充规则。
func (s *MemStateStore) ensureOidcConfig(ctx context.Context) (*dsmodel.OidcConfig, error) {
	var cfg dsmodel.OidcConfig
	err := s.db.WithContext(ctx).First(&cfg, oidcConfigID).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		cfg = dsmodel.OidcConfig{ID: oidcConfigID, Scopes: dsmodel.DefaultOidcScopes, AutoProvision: true}
		if err := s.db.WithContext(ctx).Create(&cfg).Error; err != nil {
			return nil, err
		}
	}
	var roleRules []dsmodel.OidcRoleRule
	if err := s.db.WithContext(ctx).Order("position asc, id asc").Find(&roleRules).Error; err != nil {
		return nil, err
	}
	cfg.RoleRules = roleRules
	var deptRules []dsmodel.OidcDeptRule
	if err := s.db.WithContext(ctx).Order("position asc, id asc").Find(&deptRules).Error; err != nil {
		return nil, err
	}
	cfg.DepartmentRules = deptRules
	return &cfg, nil
}

func (s *MemStateStore) GetOidcConfig(ctx context.Context) (*dsmodel.OidcConfig, error) {
	return s.ensureOidcConfig(ctx)
}

// SetOidcConfig 覆盖保存配置与两类规则：先校验（开关打开的必填/格式 + 规则的表达式与目标角色/部门），
// 再单事务写配置行并整体替换两张规则表。不发事件（与系统状态写入一致，前端写后自拉）。
func (s *MemStateStore) SetOidcConfig(ctx context.Context, cfg *dsmodel.OidcConfig, clientSecret *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.ensureOidcConfig(ctx); err != nil {
		return err
	}
	if cfg.Enabled {
		if !strings.HasPrefix(cfg.Issuer, "http://") && !strings.HasPrefix(cfg.Issuer, "https://") {
			return &Error{Code: "oidc_issuer_invalid", Msg: "Issuer 必须是 http(s):// 开头的完整地址"}
		}
		if cfg.ClientID == "" {
			return &Error{Code: "oidc_client_id_required", Msg: "Client ID 必填"}
		}
		if !strings.HasPrefix(cfg.RedirectURL, "http://") && !strings.HasPrefix(cfg.RedirectURL, "https://") {
			return &Error{Code: "oidc_redirect_url_invalid", Msg: "回调地址必须是 http(s):// 开头的完整地址"}
		}
		hasOpenid := false
		for _, sc := range dsmodel.ParseScopes(cfg.Scopes) {
			if sc == "openid" {
				hasOpenid = true
				break
			}
		}
		if !hasOpenid {
			return &Error{Code: "oidc_scopes_invalid", Msg: "Scopes 必须包含 openid"}
		}
	}
	for i := range cfg.RoleRules {
		r := &cfg.RoleRules[i]
		r.Expression = strings.TrimSpace(r.Expression)
		if err := checkOidcRuleExpression(r.Expression, i, "规则", "oidc_rule_invalid"); err != nil {
			return err
		}
		var n int64
		if err := s.db.WithContext(ctx).Model(&dsmodel.Role{}).Where("id = ?", r.RoleID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return &Error{Code: "oidc_rule_role_missing", Msg: fmt.Sprintf("第 %d 条规则的目标角色不存在", i+1)}
		}
	}
	for i := range cfg.DepartmentRules {
		r := &cfg.DepartmentRules[i]
		r.Expression = strings.TrimSpace(r.Expression)
		if err := checkOidcRuleExpression(r.Expression, i, "部门规则", "oidc_dept_rule_invalid"); err != nil {
			return err
		}
		var n int64
		if err := s.db.WithContext(ctx).Model(&dsmodel.Department{}).Where("id = ?", r.DepartmentID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return &Error{Code: "oidc_dept_rule_department_missing", Msg: fmt.Sprintf("第 %d 条部门规则的目标部门不存在", i+1)}
		}
	}
	updates := map[string]any{
		"enabled":        cfg.Enabled,
		"issuer":         cfg.Issuer,
		"client_id":      cfg.ClientID,
		"scopes":         cfg.Scopes,
		"redirect_url":   cfg.RedirectURL,
		"auto_provision": cfg.AutoProvision,
	}
	if clientSecret != nil {
		updates["client_secret"] = *clientSecret
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&dsmodel.OidcConfig{}).Where("id = ?", oidcConfigID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Where("id > 0").Delete(&dsmodel.OidcRoleRule{}).Error; err != nil {
			return err
		}
		for i := range cfg.RoleRules {
			rule := dsmodel.OidcRoleRule{Position: i, Expression: cfg.RoleRules[i].Expression, RoleID: cfg.RoleRules[i].RoleID}
			if err := tx.Create(&rule).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("id > 0").Delete(&dsmodel.OidcDeptRule{}).Error; err != nil {
			return err
		}
		for i := range cfg.DepartmentRules {
			rule := dsmodel.OidcDeptRule{
				Position:     i,
				Expression:   cfg.DepartmentRules[i].Expression,
				DepartmentID: cfg.DepartmentRules[i].DepartmentID,
			}
			if err := tx.Create(&rule).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// checkOidcRuleExpression 校验第 i+1 条（1 起）JMESPath 规则的表达式：非空且可编译。
// kind 只进中文文案（「规则」/「部门规则」），code 为该规则类的错误码。
func checkOidcRuleExpression(expression string, i int, kind, code string) error {
	if expression == "" {
		return &Error{Code: code, Msg: fmt.Sprintf("第 %d 条%s的表达式不能为空", i+1, kind)}
	}
	if _, err := oidcauth.Compile(expression); err != nil {
		return &Error{Code: code, Msg: fmt.Sprintf("第 %d 条%s的 JMESPath 表达式无效：%v", i+1, kind, err)}
	}
	return nil
}

func (s *MemStateStore) FindUserByOidcSubject(ctx context.Context, subject string) (*dsmodel.User, error) {
	var u dsmodel.User
	if err := s.db.WithContext(ctx).Where("oidc_subject = ?", subject).First(&u).Error; err != nil {
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

// SyncOidcUser 以 IdP 为准覆盖显示名、角色与部门（name 为空串时保留原显示名；
// departmentID 为 nil 表示未命中部门规则 → 不动现有部门）。
func (s *MemStateStore) SyncOidcUser(ctx context.Context, id uint64, name string, roleIDs []uint64, departmentID *uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name != "" {
		if err := s.db.WithContext(ctx).Model(&dsmodel.User{}).Where("id = ?", id).
			UpdateColumn("name", name).Error; err != nil {
			return err
		}
	}
	if departmentID != nil {
		if err := s.ensureDepartment(ctx, *departmentID); err != nil {
			return err
		}
		if err := s.db.WithContext(ctx).Model(&dsmodel.User{}).Where("id = ?", id).
			UpdateColumn("department_id", *departmentID).Error; err != nil {
			return err
		}
	}
	return s.setRolesLocked(ctx, id, roleIDs)
}

//---- 系统状态 ----

// systemStatusID 系统状态单行配置的固定主键（ID 恒为 1）。
const systemStatusID = 1

// ensureSystemStatus 读取系统状态行，不存在时按默认面试阶段落库（幂等初始化）。
func (s *MemStateStore) ensureSystemStatus(ctx context.Context) (*dsmodel.SystemStatus, error) {
	var st dsmodel.SystemStatus
	err := s.db.WithContext(ctx).First(&st, systemStatusID).Error
	if err == nil {
		return &st, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	st = dsmodel.SystemStatus{ID: systemStatusID, Phase: dsmodel.SystemPhaseInterview, BidStep: dsmodel.DefaultBidStep}
	if err := s.db.WithContext(ctx).Create(&st).Error; err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *MemStateStore) GetSystemStatus(ctx context.Context) (*dsmodel.SystemStatus, error) {
	return s.ensureSystemStatus(ctx)
}

func (s *MemStateStore) SetSystemStatus(ctx context.Context, phase dsmodel.SystemPhase) error {
	if !phase.Valid() {
		return &Error{Code: "invalid_phase", Msg: "非法系统阶段"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.ensureSystemStatus(ctx); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Model(&dsmodel.SystemStatus{}).Where("id = ?", systemStatusID).
		Update("phase", phase).Error; err != nil {
		return err
	}
	// 进入结算阶段：先按出价批量结算全部竞拍（幂等，先落库），再归一化候选人状态。
	if phase == dsmodel.SystemPhaseSettlement {
		if err := s.settleLeftoverAllLocked(ctx); err != nil {
			return err
		}
	}
	// 切换到捡漏/结算阶段时，批量同步录取档状态（唯一录取确定 → 已录取，其余 → 待录取）。
	if phase == dsmodel.SystemPhaseLeftover || phase == dsmodel.SystemPhaseSettlement {
		if err := s.syncAdmissionStatuses(ctx); err != nil {
			return err
		}
	}
	return nil
}

// syncAdmissionStatuses 批量同步录取档候选人状态（先落库，无事件广播——由前端阶段切换后重拉）。
// 规则：恰好一家 admitted（唯一录取确定）→ ADMITTED；其余（未定/争议/无决定）→ ADMISSION_PENDING。
// 仅影响 COMPLETED / ADMISSION_PENDING / ADMITTED 三档，未完成的候选人不动。
func (s *MemStateStore) syncAdmissionStatuses(ctx context.Context) error {
	inScope := "status IN ('COMPLETED', 'ADMISSION_PENDING', 'ADMITTED')"
	settled := "SELECT COUNT(*) FROM candidate_admissions a WHERE a.candidate_id = candidates.id AND a.status = 'admitted'"
	if err := s.db.WithContext(ctx).Exec(
		"UPDATE candidates SET status = 'ADMITTED', updated_at = CURRENT_TIMESTAMP WHERE " + inScope + " AND (" + settled + ") = 1",
	).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Exec(
		"UPDATE candidates SET status = 'ADMISSION_PENDING', updated_at = CURRENT_TIMESTAMP WHERE " + inScope + " AND (" + settled + ") <> 1",
	).Error
}

// SetBidStep 设置出价步长（管理面板；≥1）。
func (s *MemStateStore) SetBidStep(ctx context.Context, step int) error {
	if step < 1 {
		return &Error{Code: "invalid_bid_step", Msg: "出价步长须为正整数"}
	}
	if _, err := s.ensureSystemStatus(ctx); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&dsmodel.SystemStatus{}).Where("id = ?", systemStatusID).
		Update("bid_step", step).Error
}

//---- 候选人管理 ----

func (s *MemStateStore) UpdateCandidate(ctx context.Context, id uint64, info CandidateInfo) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.ensureCandidate(ctx, id); err != nil {
		return nil, err
	}
	c, err := normalizeCandidateInfo(info)
	if err != nil {
		return nil, err
	}
	taken, err := s.studentNoTakenLocked(ctx, c.StudentNo, id)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, ErrStudentNoExists
	}
	// map 形式的 Updates：零值列（空串 / false）也要覆盖，资料字段全量写。
	if err := s.db.WithContext(ctx).Model(&dsmodel.Candidate{}).Where("id = ?", id).
		Updates(map[string]any{
			"student_no":    c.StudentNo,
			"name":          c.Name,
			"profile":       c.Profile,
			"first_choice":  c.FirstChoice,
			"second_choice": c.SecondChoice,
			"accept_adjust": c.AcceptAdjust,
			"phone":         c.Phone,
			"qq":            c.QQ,
			"email":         c.Email,
		}).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventCandidateUpdated, Data: CandidateRef{id}}
	s.emit(0, ev)
	return ev, nil
}

// UpdateCandidatePreferences 只改志愿与调剂三列：其余资料（学号/姓名/简介/联系方式）
// 与运行态（状态机 / 房间绑定 / 消息 / 录取决定 / 出价）一律不动。
func (s *MemStateStore) UpdateCandidatePreferences(ctx context.Context, id uint64, prefs CandidatePreferences) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.ensureCandidate(ctx, id); err != nil {
		return nil, err
	}
	// map 形式的 Updates：清空志愿（空串）、取消调剂（false）也要落库。
	if err := s.db.WithContext(ctx).Model(&dsmodel.Candidate{}).Where("id = ?", id).
		Updates(map[string]any{
			"first_choice":  strings.TrimSpace(prefs.FirstChoice),
			"second_choice": strings.TrimSpace(prefs.SecondChoice),
			"accept_adjust": prefs.AcceptAdjust,
		}).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventCandidateUpdated, Data: CandidateRef{id}}
	s.emit(0, ev)
	return ev, nil
}

// DeleteCandidate 删除候选人：解绑房间、连带删消息档案与录取/出价记录。
func (s *MemStateStore) DeleteCandidate(ctx context.Context, id uint64) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.ensureCandidate(ctx, id); err != nil {
		return nil, err
	}
	// 解绑房间（房间保留为空记录；绑定的唯一权威在 rooms.candidate_id）
	if room, err := s.roomByCandidate(ctx, id); err == nil {
		if err := s.db.WithContext(ctx).Model(&dsmodel.Room{}).Where("id = ?", room.ID).
			UpdateColumn("candidate_id", nil).Error; err != nil {
			return nil, err
		}
	}

	// 连带删消息档案（级联：删人即删其记录）
	if err := s.db.WithContext(ctx).Where("candidate_id = ?", id).Delete(&dsmodel.Message{}).Error; err != nil {
		return nil, err
	}
	// 连带删各部门的录取决定与捡漏出价
	if err := s.db.WithContext(ctx).Where("candidate_id = ?", id).Delete(&dsmodel.CandidateAdmission{}).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Where("candidate_id = ?", id).Delete(&dsmodel.Bid{}).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Delete(&dsmodel.Candidate{}, id).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventCandidateDeleted, Data: CandidateRef{id}}
	s.emit(0, ev)
	return ev, nil
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
	// 录取档（待录取/已录取）位于面试完成之后：房间已解绑，重置不需要房间绑定。
	postInterview := to == dsmodel.StatusAdmissionPending || to == dsmodel.StatusAdmitted
	backward := to == dsmodel.StatusNotCheckedIn || to == dsmodel.StatusCheckedInPendingAssign

	var roomID uint64
	updates := statusUpdates(to, time.Now())
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
	case postInterview:
		// 待录取/已录取：仅改状态，不涉及房间。
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

// ListCandidateAdmissions 返回部门对候选人的录取决定。
// departmentID 为 nil 时返回全部记录（跨部门查看）；否则仅返回指定部门的记录。
func (s *MemStateStore) ListCandidateAdmissions(ctx context.Context, departmentID *uint64) ([]*dsmodel.CandidateAdmission, error) {
	qb := s.db.WithContext(ctx).Model(&dsmodel.CandidateAdmission{})
	if departmentID != nil {
		qb = qb.Where("department_id = ?", *departmentID)
	}
	out := []*dsmodel.CandidateAdmission{}
	if err := qb.Order("candidate_id asc, department_id asc").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// UpsertCandidateAdmission 记录/更新某部门对候选人的录取决定。
func (s *MemStateStore) UpsertCandidateAdmission(ctx context.Context, candidateID, departmentID uint64, status dsmodel.AdmissionStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.ensureCandidate(ctx, candidateID); err != nil {
		return err
	}
	if err := s.ensureDepartment(ctx, departmentID); err != nil {
		return err
	}
	if !validAdmissionStatus(status) {
		return &Error{Code: "invalid_admission_status", Msg: "非法录取状态"}
	}
	var rec dsmodel.CandidateAdmission
	err := s.db.WithContext(ctx).
		Where("candidate_id = ? AND department_id = ?", candidateID, departmentID).
		First(&rec).Error
	if err == nil {
		if rec.Status == status {
			return nil // 幂等：同状态不重复写
		}
		return s.db.WithContext(ctx).Model(&rec).Update("status", status).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	rec = dsmodel.CandidateAdmission{CandidateID: candidateID, DepartmentID: departmentID, Status: status}
	return s.db.WithContext(ctx).Create(&rec).Error
}

// validAdmissionStatus 判断录取决定状态是否合法。
func validAdmissionStatus(s dsmodel.AdmissionStatus) bool {
	for _, v := range dsmodel.ValidAdmissionStatus() {
		if v == s {
			return true
		}
	}
	return false
}

//---- 捡漏阶段（按预算竞拍） ----

// admittedCount 统计部门**已确认录取**人数：只算唯一录取（封盘）的候选人。
// 争议（≥2 家 admitted）候选人尚未定归属，要靠捡漏竞拍决胜负——不能提前占名额、缩预算基数。
func (s *MemStateStore) admittedCount(ctx context.Context, departmentID uint64) (int, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&dsmodel.CandidateAdmission{}).
		Where("department_id = ? AND status = ?", departmentID, dsmodel.AdmissionAdmitted).
		Where("(SELECT COUNT(*) FROM candidate_admissions c WHERE c.candidate_id = candidate_admissions.candidate_id AND c.status = ?) = 1",
			dsmodel.AdmissionAdmitted).
		Count(&n).Error
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// deptSpent 统计部门当前出价占用（预算扣除项）。
// 只有**唯一录取（封盘）且赢家正是本部门**的出价不计入——该候选人的录取已通过 admittedCount
// 缩减预算基数，再计出价会双重扣减；争议候选人的出价仍是在价竞拍，必须占用预算。
func (s *MemStateStore) deptSpent(ctx context.Context, departmentID uint64) (int, error) {
	var n *int64
	if err := s.db.WithContext(ctx).Model(&dsmodel.Bid{}).
		Where("department_id = ? AND NOT EXISTS (SELECT 1 FROM candidate_admissions a "+
			"WHERE a.candidate_id = bids.candidate_id AND a.department_id = bids.department_id AND a.status = ? "+
			"AND (SELECT COUNT(*) FROM candidate_admissions c WHERE c.candidate_id = a.candidate_id AND c.status = ?) = 1)",
			departmentID, dsmodel.AdmissionAdmitted, dsmodel.AdmissionAdmitted).
		Select("COALESCE(SUM(amount),0)").Scan(&n).Error; err != nil {
		return 0, err
	}
	if n == nil {
		return 0, nil
	}
	return int(*n), nil
}

// LeftoverOverview 返回捡漏总览：各部门预算。
// exposeAll 为 true 时所有部门 spent/remaining 公开（持 candidates.browse_all 的管理端）；
// 否则仅本部门可见（出价保密）。
func (s *MemStateStore) LeftoverOverview(ctx context.Context, myDepartmentID *uint64, exposeAll bool) (*dsmodel.LeftoverOverview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.ensureSystemStatus(ctx)
	if err != nil {
		return nil, err
	}
	depts, err := s.ListDepartments(ctx)
	if err != nil {
		return nil, err
	}
	out := &dsmodel.LeftoverOverview{Phase: st.Phase, Departments: []dsmodel.DepartmentLeftover{}}
	for _, d := range depts {
		admitted, err := s.admittedCount(ctx, d.ID)
		if err != nil {
			return nil, err
		}
		item := &dsmodel.DepartmentLeftover{
			ID: d.ID, Name: d.Name, ExpectedCount: d.ExpectedCount,
			AdmittedCount: admitted,
			Budget:        dsmodel.LeftoverBudget(d.ExpectedCount, admitted),
		}
		own := myDepartmentID != nil && d.ID == *myDepartmentID
		if own || exposeAll {
			spent, err := s.deptSpent(ctx, d.ID)
			if err != nil {
				return nil, err
			}
			item.Spent = &spent
			rem := item.Budget - spent
			item.Remaining = &rem
			if own {
				out.My = &dsmodel.MyLeftover{DepartmentID: d.ID, Budget: item.Budget, Spent: spent, Remaining: rem}
			}
		}
		out.Departments = append(out.Departments, *item)
	}
	return out, nil
}

// ListLeftoverBids 返回出价列表：departmentID 为 nil 时跨部门（管理端），否则仅本部门。
func (s *MemStateStore) ListLeftoverBids(ctx context.Context, departmentID *uint64) ([]*dsmodel.Bid, error) {
	qb := s.db.WithContext(ctx).Model(&dsmodel.Bid{})
	if departmentID != nil {
		qb = qb.Where("department_id = ?", *departmentID)
	}
	out := []*dsmodel.Bid{}
	if err := qb.Order("candidate_id asc, department_id asc").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// UpsertLeftoverBid 记录/覆盖本部门对候选人的出价。
// 约束：仅捡漏阶段；候选人未结算；出价 ≥ 0（0 是合法出价）；新总额（含本次出价）不得超过部门预算。
func (s *MemStateStore) UpsertLeftoverBid(ctx context.Context, candidateID, departmentID uint64, amount int) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if amount < 0 {
		return nil, &Error{Code: "invalid_amount", Msg: "出价不能为负数"}
	}
	if _, err := s.ensureCandidate(ctx, candidateID); err != nil {
		return nil, err
	}
	if err := s.ensureDepartment(ctx, departmentID); err != nil {
		return nil, err
	}
	st, err := s.ensureSystemStatus(ctx)
	if err != nil {
		return nil, err
	}
	if st.Phase != dsmodel.SystemPhaseLeftover {
		return nil, &Error{Code: "not_leftover_phase", Msg: "当前不在捡漏阶段，无法出价"}
	}
	// 封盘判定：恰好一家部门 admitted（录取已确定）才封盘；
	// 0 家 = 未定可竞拍；≥2 家 = 多部门录取争议，同样进入捡漏由出价仲裁。
	admittedN, err := s.candidateAdmittedCount(ctx, candidateID)
	if err != nil {
		return nil, err
	}
	if admittedN == 1 {
		return nil, &Error{Code: "already_resolved", Msg: "候选人已确定唯一录取部门"}
	}
	var dept dsmodel.Department
	if err := s.db.WithContext(ctx).First(&dept, departmentID).Error; err != nil {
		return nil, err
	}
	admitted, err := s.admittedCount(ctx, departmentID)
	if err != nil {
		return nil, err
	}
	budget := dsmodel.LeftoverBudget(dept.ExpectedCount, admitted)
	// 当前占用（若覆盖旧出价则先扣除旧额）。用 found 而非金额判断是否存在：
	// 0 是合法出价，不能再拿 oldAmount > 0 当「是否已出价」的哨兵。
	var old dsmodel.Bid
	oldAmount := 0
	found := false
	switch err := s.db.WithContext(ctx).
		Where("candidate_id = ? AND department_id = ?", candidateID, departmentID).
		First(&old).Error; {
	case err == nil:
		oldAmount = old.Amount
		found = true
	case errors.Is(err, gorm.ErrRecordNotFound):
		// 本部门尚未对该候选人出价
	default:
		return nil, err
	}
	spent, err := s.deptSpent(ctx, departmentID)
	if err != nil {
		return nil, err
	}
	if spent-oldAmount+amount > budget {
		return nil, &Error{Code: "budget_exceeded", Msg: "超出部门剩余预算"}
	}
	var bid dsmodel.Bid
	if found {
		bid = old
		if err := s.db.WithContext(ctx).Model(&bid).Update("amount", amount).Error; err != nil {
			return nil, err
		}
	} else {
		bid = dsmodel.Bid{CandidateID: candidateID, DepartmentID: departmentID, Amount: amount}
		if err := s.db.WithContext(ctx).Create(&bid).Error; err != nil {
			return nil, err
		}
	}
	ev := &Event{Type: EventLeftoverBid, Data: LeftoverRef{CandidateID: candidateID, DepartmentID: departmentID}}
	s.emit(0, ev)
	return ev, nil
}

// settleLeftoverAllLocked 结算当前全部有出价的候选人（进入结算阶段时调用，幂等）。
// 按 candidate_id 升序保证事件顺序稳定；任一错误立即返回。调用方须持 s.mu。
func (s *MemStateStore) settleLeftoverAllLocked(ctx context.Context) error {
	var ids []uint64
	if err := s.db.WithContext(ctx).Model(&dsmodel.Bid{}).
		Distinct().Order("candidate_id asc").Pluck("candidate_id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.settleLeftoverCandidateLocked(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// settleLeftoverCandidateLocked 按出价结算单个候选人：最高价部门录取、其余出价部门放弃，
// 未出价但曾手动录取的部门一并改放弃；成交则候选人推进到已录取并 emit EventLeftoverResolved。
// 无可结算内容（无出价 / 已存在唯一录取封盘）返回 (nil, nil)。调用方须持 s.mu。
func (s *MemStateStore) settleLeftoverCandidateLocked(ctx context.Context, candidateID uint64) (*Event, error) {
	// 封盘判定与出价一致：恰好一家 admitted 才视为已确定；多家录取属争议，允许竞拍仲裁。
	settled, err := s.candidateAdmittedCount(ctx, candidateID)
	if err != nil {
		return nil, err
	}
	if settled == 1 {
		return nil, nil
	}
	bids := []*dsmodel.Bid{}
	if err := s.db.WithContext(ctx).
		Where("candidate_id = ?", candidateID).
		Order("amount desc, id asc").Find(&bids).Error; err != nil {
		return nil, err
	}
	if len(bids) == 0 {
		return nil, nil
	}
	winner := bids[0]
	for _, b := range bids {
		status := dsmodel.AdmissionWithdrawn
		if b.ID == winner.ID {
			status = dsmodel.AdmissionAdmitted
		}
		var rec dsmodel.CandidateAdmission
		err := s.db.WithContext(ctx).
			Where("candidate_id = ? AND department_id = ?", candidateID, b.DepartmentID).
			First(&rec).Error
		if err == nil {
			if err := s.db.WithContext(ctx).Model(&rec).Update("status", status).Error; err != nil {
				return nil, err
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			rec = dsmodel.CandidateAdmission{CandidateID: candidateID, DepartmentID: b.DepartmentID, Status: status}
			if err := s.db.WithContext(ctx).Create(&rec).Error; err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	// 仲裁收尾：未参与出价但曾手动录取该候选人的部门（争议来源）一并改为 withdrawn。
	if err := s.db.WithContext(ctx).Model(&dsmodel.CandidateAdmission{}).
		Where("candidate_id = ? AND department_id <> ? AND status = ?",
			candidateID, winner.DepartmentID, dsmodel.AdmissionAdmitted).
		Update("status", dsmodel.AdmissionWithdrawn).Error; err != nil {
		return nil, err
	}
	// 结算成交 → 候选人推进到已录取（仅对处于录取档/面试已结束的候选人生效）。
	if err := s.db.WithContext(ctx).Model(&dsmodel.Candidate{}).
		Where("id = ? AND status IN (?, ?)", candidateID,
			dsmodel.StatusAdmissionPending, dsmodel.StatusCompleted).
		Update("status", dsmodel.StatusAdmitted).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventLeftoverResolved, Data: LeftoverRef{CandidateID: candidateID, DepartmentID: winner.DepartmentID, Amount: winner.Amount}}
	s.emit(0, ev)
	return ev, nil
}

// candidateAdmittedCount 统计候选人的 admitted 录取记录数（封盘/争议判定用）。
func (s *MemStateStore) candidateAdmittedCount(ctx context.Context, candidateID uint64) (int64, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&dsmodel.CandidateAdmission{}).
		Where("candidate_id = ? AND status = ?", candidateID, dsmodel.AdmissionAdmitted).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// LeftoverFinalResults 只读计算各候选人的最终录取结果（不落库）：
// 每个有出价的候选人取赢家 = 最高出价部门，同额取先出价者（bid id 更小）。
// 保密语义与出价一致：默认仅返回已成交（Resolved，公开结果）或赢家为本部门的行；
// exposeAll（candidates.browse_all）才返回全部部门的进行中结果。
func (s *MemStateStore) LeftoverFinalResults(ctx context.Context, myDepartmentID *uint64, exposeAll bool) ([]*dsmodel.LeftoverFinalResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bids := []*dsmodel.Bid{}
	if err := s.db.WithContext(ctx).Order("id asc").Find(&bids).Error; err != nil {
		return nil, err
	}
	winner := map[uint64]*dsmodel.Bid{} // candidate → 当前赢家（金额最高，同额取先出价者）
	for _, b := range bids {
		if w, ok := winner[b.CandidateID]; !ok || b.Amount > w.Amount {
			winner[b.CandidateID] = b
		}
	}
	out := make([]*dsmodel.LeftoverFinalResult, 0, len(winner))
	for _, w := range winner {
		row := &dsmodel.LeftoverFinalResult{CandidateID: w.CandidateID, DepartmentID: w.DepartmentID, Amount: w.Amount}
		// Resolved = 候选人已唯一确定录取且赢家正是该部门（争议候选人未经仲裁不算）
		total, err := s.candidateAdmittedCount(ctx, w.CandidateID)
		if err != nil {
			return nil, err
		}
		var own int64
		if total == 1 {
			if err := s.db.WithContext(ctx).Model(&dsmodel.CandidateAdmission{}).
				Where("candidate_id = ? AND department_id = ? AND status = ?",
					w.CandidateID, w.DepartmentID, dsmodel.AdmissionAdmitted).
				Count(&own).Error; err != nil {
				return nil, err
			}
		}
		row.Resolved = own > 0
		// 保密过滤：未成交且赢家非本部门 → 不返回（金额对他部门保密）
		if !exposeAll && !row.Resolved &&
			(myDepartmentID == nil || w.DepartmentID != *myDepartmentID) {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CandidateID < out[j].CandidateID })
	return out, nil
}

// ListLeftoverResults 返回已结算候选人的赢家与成交金额。
// 已结算 = 恰好一家部门 admitted（录取唯一确定）；多家录取的争议候选人不在此列，
// 需进入结算阶段自动按出价仲裁（settleLeftoverCandidateLocked）后才产生结果。
func (s *MemStateStore) ListLeftoverResults(ctx context.Context) ([]*dsmodel.LeftoverResult, error) {
	admissions := []*dsmodel.CandidateAdmission{}
	if err := s.db.WithContext(ctx).
		Where("status = ?", dsmodel.AdmissionAdmitted).
		Order("candidate_id asc").Find(&admissions).Error; err != nil {
		return nil, err
	}
	counts := map[uint64]int{} // candidate → admitted 部门数
	for _, a := range admissions {
		counts[a.CandidateID]++
	}
	out := []*dsmodel.LeftoverResult{}
	for _, a := range admissions {
		if counts[a.CandidateID] != 1 {
			continue // 争议（≥2 家录取）：进捡漏，不算已结算
		}
		var bid dsmodel.Bid
		amount := 0
		err := s.db.WithContext(ctx).
			Where("candidate_id = ? AND department_id = ?", a.CandidateID, a.DepartmentID).
			First(&bid).Error
		if err == nil {
			amount = bid.Amount
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		out = append(out, &dsmodel.LeftoverResult{
			CandidateID: a.CandidateID, DepartmentID: a.DepartmentID,
			Amount: amount, UpdatedAt: a.UpdatedAt,
		})
	}
	return out, nil
}

//---- 房间管理 ----

func (s *MemStateStore) CreateRoom(ctx context.Context, name string) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := dsmodel.ValidateRoomName(name)
	if err != nil {
		return nil, &Error{Code: "room_name_invalid", Msg: err.Error()}
	}
	r := &dsmodel.Room{Name: n}
	if err := s.db.WithContext(ctx).Create(r).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventRoomCreated, Data: RoomRef{r.ID}}
	s.emit(0, ev)
	return ev, nil
}

func (s *MemStateStore) DeleteRoom(ctx context.Context, id uint64) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, err := s.ensureRoom(ctx, id)
	if err != nil {
		return nil, err
	}
	if room.CandidateID != nil {
		return nil, &Error{Code: "room_not_empty", Msg: "房间仍绑定候选人"}
	}
	var members int64
	if err := s.db.WithContext(ctx).Model(&dsmodel.RoomMember{}).Where("room_id = ?", id).Count(&members).Error; err != nil {
		return nil, err
	}
	if members > 0 {
		return nil, &Error{Code: "room_not_empty", Msg: "房间仍有成员"}
	}
	if err := s.db.WithContext(ctx).Delete(&dsmodel.Room{}, id).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventRoomDeleted, Data: RoomRef{id}}
	s.emit(0, ev)
	return ev, nil
}

// RenameRoom 修改房间名（空串=清除命名）。事件为全局（RoomID=0，载荷带房间 id）：
// 看板通道据此重拉房间列表，房间通道按载荷自取本房改名。
func (s *MemStateStore) RenameRoom(ctx context.Context, id uint64, name string) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.ensureRoom(ctx, id); err != nil {
		return nil, err
	}
	n, err := dsmodel.ValidateRoomName(name)
	if err != nil {
		return nil, &Error{Code: "room_name_invalid", Msg: err.Error()}
	}
	if err := s.db.WithContext(ctx).Model(&dsmodel.Room{}).Where("id = ?", id).
		Update("name", n).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventRoomRenamed, Data: RoomRef{id}}
	s.emit(0, ev)
	return ev, nil
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
	if err := s.db.WithContext(ctx).Model(c).
		Updates(statusUpdates(dsmodel.StatusAssigned, time.Now())).Error; err != nil {
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

// statusUpdates 组装候选人状态迁移的写库列：状态本身 + 面试计时打点。
// interview_started_at 的语义 = 当前这次面试的开始时刻（NULL = 不在面试中）：
// 进入 IN_PROGRESS 打点，其余状态一律清空；重新进入会重新打点。
// 走批量 SQL 的状态更新（syncAdmissionStatuses、捡漏结算）不在「面试中」路径上，无需打点。
func statusUpdates(to dsmodel.CandidateStatus, now time.Time) map[string]any {
	updates := map[string]any{"status": to}
	if to == dsmodel.StatusInProgress {
		updates["interview_started_at"] = now
	} else {
		updates["interview_started_at"] = nil
	}
	// 签到时刻：只有重置回「未签到」才清空，其余流转（被拉入房间 / 开始面试 / 完成 / 录取）
	// **不改它** —— 「几点到的」在这些档位里仍是事实，候场大屏要把已签到的各档按到达先后排列。
	// 打点在 CheckIn 那一步显式完成（不在这里兜底）：重新进入「已签到待分配」（如误拉后重置）
	// 不该把先到的人挪到队尾，重新排队应由「重新签到」这个动作本身表达。
	if to == dsmodel.StatusNotCheckedIn {
		updates["checked_in_at"] = nil
	}
	return updates
}
