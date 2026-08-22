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

// clientsAll 返回所有房间的全部去重客户端（全局扇出用）。
// 一个客户端至多属于一个房间，故跨房间遍历天然无重复。
func (h *hub) clientsAll() []*wsClient {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var out []*wsClient
	for _, m := range h.rooms {
		for c := range m {
			out = append(out, c)
		}
	}
	return out
}
