package handler

import "sync"

// hub 管理每个房间正在连接的 WS 客户端集合。
// 广播扇出时遍历房间内所有 client 写入其写队列。
type hub struct {
	mu    sync.RWMutex
	rooms map[uint64]map[*wsClient]struct{}
}

func newHub() *hub {
	return &hub{rooms: make(map[uint64]map[*wsClient]struct{})}
}

func (h *hub) add(roomID uint64, c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[roomID] == nil {
		h.rooms[roomID] = make(map[*wsClient]struct{})
	}
	h.rooms[roomID][c] = struct{}{}
}

func (h *hub) remove(roomID uint64, c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m, ok := h.rooms[roomID]; ok {
		delete(m, c)
		if len(m) == 0 {
			delete(h.rooms, roomID)
		}
	}
}

func (h *hub) clientsIn(roomID uint64) []*wsClient {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var out []*wsClient
	if m, ok := h.rooms[roomID]; ok {
		for c := range m {
			out = append(out, c)
		}
	}
	return out
}

// clientsAllExcept 返回所有键（含 boardHubKey 看板集合）的客户端，
// 但排除指定键 —— 全局事件扇出用：看板客户端由 BindGlobal 单独投递，不重复。
func (h *hub) clientsAllExcept(exclude uint64) []*wsClient {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var out []*wsClient
	for key, m := range h.rooms {
		if key == exclude {
			continue
		}
		for c := range m {
			out = append(out, c)
		}
	}
	return out
}
