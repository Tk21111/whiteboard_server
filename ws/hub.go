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
	Ch           = make(chan config.RawEvent, 4095)
	strokeBuffer = StrokeBuffer{
		buffer: make(map[string]*buffer),
	}
)

func DecodeLoop() {
	for event := range Ch {
		msgs, err := middleware.DecodeNetworkMsg(event.Msg)
		if err != nil {
			continue
		}
		for _, m := range msgs {

			switch m.Operation {

			case "stroke-start":
				event.Meta.ID = NextClock(event.Meta.RoomID)
				func() {
					strokeBuffer.mu.Lock()
					defer strokeBuffer.mu.Unlock()

					strokeBuffer.buffer[m.ID] = &buffer{
						stroke: m.Stroke,
						meta:   event.Meta,
					}
				}()

			case "stroke-update":
				func() {
					strokeBuffer.mu.Lock()
					defer strokeBuffer.mu.Unlock()

					buffer, ok := strokeBuffer.buffer[m.ID]
					if !ok {
						return
					}

					buffer.stroke.Points = append(buffer.stroke.Points, m.Points...)
				}()

			case "stroke-end":
				func() {
					strokeBuffer.mu.Lock()
					defer strokeBuffer.mu.Unlock()

					b, ok := strokeBuffer.buffer[m.ID]
					if !ok {
						return
					}

					delete(strokeBuffer.buffer, m.ID)

					// flush buffered stroke as event
					db.WriteEvent(config.Event{
						EventMeta: *b.meta,
						Op:        "stroke",
						Payload:   middleware.EncodeNetworkMsg(b.stroke),
						CreatedAt: time.Now().UnixMilli(),
					})
				}()
			case "stroke-add":
				return
			default:

				event.Meta.ID = NextClock(event.Meta.RoomID)
				db.WriteEvent(config.Event{
					EventMeta: *event.Meta,
					Op:        m.Operation,
					Payload:   middleware.EncodeNetworkMsg(m),
					CreatedAt: time.Now().UnixMilli(),
				})
			}
		}
	}

}
