package broadcast

import (
	"sync"

	"interview_ng/internal/state"
)

// Sink 接收房间事件扇出的回调。
// handler 层注册它，把事件推送到该房间内所有 WS 连接。
type Sink func(ev *state.Event)

// Manager 负责把 StateStore 产出的房间事件扇出到对应房间的 WS 连接。
// 它只是事件流的"分发器"：不校验状态、不落库、不决定对错。
// 任何房间在首次有成员连接时应当调用 Register 注册扇出回调。
type Manager struct {
	mu    sync.RWMutex
	sinks map[uint64]Sink // roomID -> 该房间的扇出回调
}

// New 构建广播管理器。
func New() *Manager {
	return &Manager{sinks: make(map[uint64]Sink)}
}

// Register 为某房间注册扇出回调（通常由 WS handler 在首个成员连接时调用）。
// 重复注册时后注册者将覆盖先注册者；应保持单份。
func (m *Manager) Register(roomID uint64, sink Sink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sinks[roomID] = sink
}

// Bind 为某房间绑定额外的扇出；若房间已存在则返回 false（不覆盖），
// 用于防止同一房间被多个 handler 实例重复注册。
func (m *Manager) Bind(roomID uint64, sink Sink) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sinks[roomID]; ok {
		return false
	}
	m.sinks[roomID] = sink
	return true
}

// Unregister 移除某房间的扇出回调（最后一个成员离开时调用）。
func (m *Manager) Unregister(roomID uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sinks, roomID)
}

// Publish 将一个事件扇出到目标房间。
// 由 service 层在 StateStore 返回 Event 后调用（先落库，后广播）。
func (m *Manager) Publish(ev *state.Event) {
	if ev.RoomID == 0 {
		// 全局事件（如候选人签到、候选人被拉走）不属于任何具体房间，
		// 应广播到所有已注册的房间 sink，让所有开放房间前端都能感知"待分配池"变化。
		m.PublishGlobal(ev)
		return
	}
	m.mu.RLock()
	sink, ok := m.sinks[ev.RoomID]
	m.mu.RUnlock()
	if ok && sink != nil {
		sink(ev)
	}
}

// PublishGlobal 将事件扇出到所有已注册的房间 sink（全局广播）。
// 供 RoomID==0 的"池变化"事件使用，例如候选人签到（进入待分配池）。
// 同一连接的去重由 handler 层负责（一个连接至多属于一个房间）。
func (m *Manager) PublishGlobal(ev *state.Event) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, sink := range m.sinks {
		if sink != nil {
			sink(ev)
		}
	}
}
