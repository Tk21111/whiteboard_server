package ws

import "sync"

type Hub struct {
	rooms map[string]map[*Client]bool
	mu    sync.Mutex
}

var H = Hub{
	rooms: make(map[string]map[*Client]bool),
}

func (h *Hub) Join(room string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.rooms[room] == nil {
		h.rooms[room] = make(map[*Client]bool)
	}
	h.rooms[room][c] = true
}

func (h *Hub) Leave(room string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.rooms[room], c)
	if len(h.rooms[room]) == 0 {
		delete(h.rooms, room)
	}
}

func (h *Hub) Broadcast(room string, msg []byte, except *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for c := range h.rooms[room] {
		if c != except {
			select {
			case c.send <- msg:
			default:
				close(c.send)
				delete(h.rooms[room], c)
			}
		}
	}
}
