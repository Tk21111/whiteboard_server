package ws

import (
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
	defer h.mu.Unlock()

	room, ok := h.rooms[roomID]
	if !ok {
		return
	}

	delete(room.clients, c)
	if len(room.clients) == 0 {
		delete(h.rooms, roomID)
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

var (
	// Ch           = make(chan config.RawEvent, 4095)
	strokeBuffer = StrokeBuffer{
		buffer: make(map[string]*buffer),
	}
)

func (c *Client) handleMsg(m config.NetworkMsg) config.ServerMsg {
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

		return config.ServerMsg{
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

		return config.ServerMsg{
			Clock:   0,
			Payload: m,
		}

	case "stroke-end":
		strokeBuffer.mu.Lock()
		b, ok := strokeBuffer.buffer[m.ID]
		if !ok {
			strokeBuffer.mu.Unlock()
			return config.ServerMsg{
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

		return config.ServerMsg{
			Clock:   0,
			Payload: m,
		}

	case "stroke-add":
		return config.ServerMsg{
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
			},
		})

		return config.ServerMsg{
			Clock:   meta.ID,
			Payload: m,
		}

	case "dom-transform":

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
		})

		return config.ServerMsg{
			Clock:   meta.ID,
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

		return config.ServerMsg{
			Clock:   meta.ID,
			Payload: m,
		}

	default:

		return config.ServerMsg{
			Clock:   0,
			Payload: m,
		}
	}
}
