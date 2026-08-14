package state

import (
	"context"
	"errors"
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

func (s *MemStateStore) ensureRoom(ctx context.Context, id uint64) (*dsmodel.Room, error) {
	var r dsmodel.Room
	err := s.db.WithContext(ctx).Preload("Candidate").Preload("Members.User").First(&r, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &r, nil
}

//---- 读 ----

func (s *MemStateStore) GetCandidate(ctx context.Context, id uint64) (*dsmodel.Candidate, error) {
	return s.ensureCandidate(ctx, id)
}

func (s *MemStateStore) GetCandidateByRoom(ctx context.Context, roomID uint64) (*dsmodel.Candidate, error) {
	room, err := s.ensureRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	return s.ensureCandidate(ctx, room.CandidateID)
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
	return out, nil
}

func (s *MemStateStore) ListCandidates(ctx context.Context, status dsmodel.CandidateStatus, limit, offset int) ([]*dsmodel.Candidate, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := s.db.WithContext(ctx).Model(&dsmodel.Candidate{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var out []*dsmodel.Candidate
	if err := q.Order("id asc").Limit(limit).Offset(offset).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *MemStateStore) ListMessagesAfter(ctx context.Context, roomID uint64, afterID uint64) ([]*dsmodel.Message, error) {
	var out []*dsmodel.Message
	err := s.db.WithContext(ctx).
		Where("room_id = ? AND id > ?", roomID, afterID).
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

func (s *MemStateStore) AssignCandidate(ctx context.Context, candidateID, roomID uint64) (*Event, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.ensureCandidate(ctx, candidateID)
	if err != nil {
		return nil, 0, err
	}
	if err := guardTransition(c.Status, dsmodel.StatusAssigned); err != nil {
		return nil, 0, err
	}
	if c.RoomID != nil {
		return nil, 0, ErrAlreadyAssigned
	}
	// 分配：候选人与房间一对一；若未传房间则新建。
	if roomID == 0 {
		room := &dsmodel.Room{CandidateID: candidateID}
		if err := s.db.WithContext(ctx).Create(room).Error; err != nil {
			return nil, 0, err
		}
		roomID = room.ID
	}
	if err := s.db.WithContext(ctx).Model(c).Updates(map[string]any{
		"room_id": roomID,
		"status":  dsmodel.StatusAssigned,
	}).Error; err != nil {
		return nil, 0, err
	}
	ev := &Event{Type: EventCandidateAssigned, Data: struct {
		CandidateID uint64
		RoomID      uint64
	}{candidateID, roomID}}
	s.emit(roomID, ev)
	return ev, roomID, nil
}

func (s *MemStateStore) MovePhase(ctx context.Context, roomID, operatorID uint64, to dsmodel.CandidateStatus) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, err := s.ensureRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if room.CurrentInterviewerID != 0 && room.CurrentInterviewerID != operatorID {
		return nil, ErrNotMember
	}
	c, err := s.ensureCandidate(ctx, room.CandidateID)
	if err != nil {
		return nil, err
	}
	if err := guardTransition(c.Status, to); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(c).Update("status", to).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventRoomPhaseChanged, Data: struct {
		RoomID      uint64
		CandidateID uint64
		To          dsmodel.CandidateStatus
	}{roomID, room.CandidateID, to}}
	s.emit(roomID, ev)
	return ev, nil
}

func (s *MemStateStore) AppendMessage(ctx context.Context, roomID, senderID uint64, content string) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.ensureRoom(ctx, roomID); err != nil {
		return nil, err
	}
	if !s.isMemberLocked(ctx, roomID, senderID) {
		return nil, ErrNotMember
	}
	msg := &dsmodel.Message{RoomID: roomID, SenderID: senderID, Content: content}
	if err := s.db.WithContext(ctx).Create(msg).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventMessageAppended, RoomID: roomID, MsgID: msg.ID,
		Data: struct {
			RoomID   uint64
			SenderID uint64
			Content  string
		}{roomID, senderID, content}}
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
	var count int64
	_ = s.db.WithContext(ctx).Model(&dsmodel.RoomMember{}).
		Where("room_id = ?", roomID).Count(&count)
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

func (s *MemStateStore) SetCurrentInterviewer(ctx context.Context, roomID, operatorID, newID uint64) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, err := s.ensureRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if room.CurrentInterviewerID != 0 && room.CurrentInterviewerID != operatorID {
		return nil, ErrNotMember
	}
	if err := s.db.WithContext(ctx).Model(room).Update("current_interviewer_id", newID).Error; err != nil {
		return nil, err
	}
	ev := &Event{Type: EventRoomPhaseChanged, RoomID: roomID, Data: struct {
		RoomID      uint64
		Interviewer uint64
	}{roomID, newID}}
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
