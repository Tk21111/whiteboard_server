package ws

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tk21111/whiteboard_server/config"
	"github.com/Tk21111/whiteboard_server/db"
	"github.com/Tk21111/whiteboard_server/middleware"
)

type Room struct {
	clients map[*Client]bool
	clock   atomic.Int64
}

func NextClock(roomId string) int64 {
	return H.rooms[roomId].clock.Add(1)
}

type Hub struct {
	rooms map[string]*Room
	mu    sync.Mutex
}

var H = Hub{
	rooms: make(map[string]*Room),
}

func (h *Hub) Join(roomID string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room, ok := h.rooms[roomID]
	if !ok {
		room = &Room{
			clients: make(map[*Client]bool),
		}
		h.rooms[roomID] = room
	}

	room.clients[c] = true
}

func (h *Hub) Leave(roomID string, c *Client) {
	h.mu.Lock()
	room, ok := h.rooms[roomID]
	if !ok {
		h.mu.Unlock()
		return
	}

	delete(room.clients, c)
	isEmpty := len(room.clients) == 0
	if isEmpty {
		delete(h.rooms, roomID)
	}
	h.mu.Unlock() // Unlock Hub early to allow Broadcast to run safely

	if isEmpty {
		return // No one left to broadcast to
	}

	// 1. Identify which objects need unlocking
	var unlockedIDs []string
	domLocks.mu.Lock()
	for id, ownerID := range domLocks.buffer {
		if ownerID == c.userId {
			unlockedIDs = append(unlockedIDs, id)
			delete(domLocks.buffer, id)
		}
	}
	domLocks.mu.Unlock()

	// 2. Broadcast the unlock events to the room
	if len(unlockedIDs) > 0 {
		var unlockMsgs []config.ServerMsg
		for _, id := range unlockedIDs {
			unlockMsgs = append(unlockMsgs, config.ServerMsg{
				Clock: 0,
				Payload: config.NetworkMsg{
					Operation: "dom-unlock",
					ID:        id,
				},
			})
		}

		data, err := json.Marshal(unlockMsgs)
		if err == nil {
			h.Broadcast(roomID, data, nil) // Send to everyone remaining
		}
	}
}

func (h *Hub) Broadcast(roomID string, msg []byte, except *Client) {
	h.mu.Lock()
	room, ok := h.rooms[roomID]
	defer h.mu.Unlock()

	if !ok {
		return
	}

	for c := range room.clients {
		if c != except {
			select {
			case c.send <- msg:
			default:
				close(c.send)
				delete(room.clients, c)
			}
		}
	}
}

type buffer struct {
	stroke *config.StrokeObjectInterface
	meta   *config.EventMeta
}

type StrokeBuffer struct {
	buffer map[string]*buffer
	mu     sync.Mutex
}

type DomLock struct {
	buffer map[string]string
	mu     sync.Mutex
}

var (
	// Ch           = make(chan config.RawEvent, 4095)
	strokeBuffer = StrokeBuffer{
		buffer: make(map[string]*buffer),
	}
	domLocks = DomLock{
		buffer: make(map[string]string),
	}
)

func (c *Client) handleMsg(m config.NetworkMsg) *config.ServerMsg {
	meta := &config.EventMeta{
		ID:     0, // assigned per event
		RoomID: c.roomId,
		UserID: c.userId,
	}

	switch m.Operation {

	case "stroke-start":
		meta.ID = NextClock(meta.RoomID)

		strokeBuffer.mu.Lock()
		strokeBuffer.buffer[m.ID] = &buffer{
			stroke: m.Stroke,
			meta:   meta,
		}
		strokeBuffer.mu.Unlock()

		return &config.ServerMsg{
			Clock:   meta.ID,
			Payload: m,
		}

	case "stroke-update":
		strokeBuffer.mu.Lock()
		b, ok := strokeBuffer.buffer[m.ID]
		if ok {
			b.stroke.Points = append(b.stroke.Points, m.Points...)
		}
		strokeBuffer.mu.Unlock()

		return &config.ServerMsg{
			Clock:   0,
			Payload: m,
		}

	case "stroke-end":
		strokeBuffer.mu.Lock()
		b, ok := strokeBuffer.buffer[m.ID]
		if !ok {
			strokeBuffer.mu.Unlock()
			return &config.ServerMsg{
				Clock:   0,
				Payload: m,
			}
		}
		delete(strokeBuffer.buffer, m.ID)
		strokeBuffer.mu.Unlock()

		// flush buffered stroke
		// fmt.Printf("%#v\n", b.stroke)

		db.WriteEvent(config.Event{
			EventMeta: *b.meta,
			Op:        "stroke-add",
			Payload:   middleware.EncodeNetworkMsg(b.stroke),
			CreatedAt: time.Now().UnixMilli(),
		})

		return &config.ServerMsg{
			Clock:   0,
			Payload: m,
		}

	case "stroke-add":
		return &config.ServerMsg{
			Clock:   0,
			Payload: m,
		}

	case "dom-add":

		meta.ID = NextClock(meta.RoomID)
		db.WriteEvent(config.Event{
			EventMeta: *meta,
			Op:        "dom-add",
			Payload:   middleware.EncodeNetworkMsg(m),
			CreatedAt: time.Now().UnixMilli(),
		})
		db.WriteDom(config.DomEvent{
			RoomID:    c.roomId,
			UserID:    c.userId,
			CreatedAt: time.Now().UnixMilli(),
			UpdatedAt: time.Now().UnixMilli(),
			DomObjectNetwork: config.DomObjectNetwork{
				ID:        m.ID,
				Kind:      m.DomObject.Kind,
				Transform: m.DomObject.Transform,
				Payload:   m.DomObject.Payload,
			},
		}, 0)

		return &config.ServerMsg{
			Clock:   meta.ID,
			Payload: m,
		}
	case "dom-lock":
		domLocks.mu.Lock()
		defer domLocks.mu.Unlock()

		currentOwner, exists := domLocks.buffer[m.ID]
		// If someone else has it, deny the lock
		if exists && currentOwner != c.userId {
			return &config.ServerMsg{}
		}

		// Grant or refresh the lock
		domLocks.buffer[m.ID] = c.userId

		return &config.ServerMsg{Clock: 0, Payload: m}

	case "dom-unlock":
		domLocks.mu.Lock()
		if domLocks.buffer[m.ID] == c.userId {
			delete(domLocks.buffer, m.ID)
		}
		domLocks.mu.Unlock()
		return &config.ServerMsg{Clock: 0, Payload: m}

	case "dom-transform":

		domLocks.mu.Lock()
		ownerID, exists := domLocks.buffer[m.ID]
		domLocks.mu.Unlock()

		// If locked by someone else, return empty struct
		if exists && ownerID != c.userId {
			return &config.ServerMsg{}
		}

		meta.ID = NextClock(meta.RoomID)
		db.WriteEvent(config.Event{
			EventMeta: *meta,
			Op:        "dom-transform",
			Payload:   middleware.EncodeNetworkMsg(m),
			CreatedAt: time.Now().UnixMilli(),
		})
		db.WriteDom(config.DomEvent{
			RoomID:    c.roomId,
			UserID:    c.userId,
			UpdatedAt: time.Now().UnixMilli(),
			DomObjectNetwork: config.DomObjectNetwork{
				ID:        m.ID,
				Transform: *m.Transform,
			},
		}, 1)

		return &config.ServerMsg{
			Clock:   meta.ID,
			Payload: m,
		}

	case "dom-payload":

		db.WriteDom(config.DomEvent{
			RoomID:    c.roomId,
			UserID:    c.userId,
			UpdatedAt: time.Now().UnixMilli(),
			DomObjectNetwork: config.DomObjectNetwork{
				ID:      m.ID,
				Payload: *m.Payload,
			},
		}, 2)

		return &config.ServerMsg{
			Clock:   0,
			Payload: m,
		}

	case "dom-remove":

		meta.ID = NextClock(meta.RoomID)
		db.WriteEvent(config.Event{
			EventMeta: *meta,
			Op:        "dom-remove",
			Payload:   middleware.EncodeNetworkMsg(m),
			CreatedAt: time.Now().UnixMilli(),
		})

		db.RemoveDom(m.ID, c.roomId)

		return &config.ServerMsg{
			Clock:   meta.ID,
			Payload: m,
		}

	case "cursor-update":
		return &config.ServerMsg{
			Clock:   0,
			Payload: m,
		}

	default:
		return &config.ServerMsg{
			Clock:   0,
			Payload: m,
		}
	}
}
