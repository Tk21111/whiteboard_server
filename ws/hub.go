package ws

import (
	"encoding/json"
	"fmt"
	"log"
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

		maxId, err := db.GetMaxIdByRoom(roomID)
		if err != nil {
			fmt.Println("[db] get max id err")
		}
		room = &Room{
			clients: make(map[*Client]bool),
		}
		room.clock.Store(maxId)

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
	h.mu.Unlock()

	if isEmpty {
		return
	}

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
	var msgs []config.ServerMsg
	if len(unlockedIDs) > 0 {
		for _, id := range unlockedIDs {
			msgs = append(msgs, config.ServerMsg{
				Clock: 0,
				Payload: config.NetworkMsg{
					Operation: "dom-unlock",
					ID:        id,
				},
			})
		}
	}

	msgs = append(msgs, config.ServerMsg{
		Clock: 0,
		Payload: config.NetworkMsg{
			Operation: "client-leave",
			ID:        c.userId,
		},
	})

	data, err := json.Marshal(msgs)
	if err == nil {
		h.Broadcast(roomID, data, nil) // Send to everyone remaining
	}

	log.Println("leave room", c.roomId, "user", c.userId)
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
				delete(room.clients, c)
			}
		}
	}
}

type bufferStruct struct {
	Stroke *config.StrokeObjectInterface
	Meta   *config.EventMeta
	TTL    int64
}

const StrokeTTL = 10 * time.Minute

type StrokeBufferStruct struct {
	Buffer map[string]*bufferStruct
	Mu     sync.Mutex
}

type DomLock struct {
	buffer map[string]string
	mu     sync.Mutex
}

var (
	// Ch           = make(chan config.RawEvent, 4095)
	StrokeBuffer = StrokeBufferStruct{
		Buffer: make(map[string]*bufferStruct),
	}
	domLocks = DomLock{
		buffer: make(map[string]string),
	}
)

func (c *Client) handleMsg(m config.NetworkMsg) *config.ServerMsg {
	meta := &config.EventMeta{
		ID:         0, // assigned per event
		RoomID:     c.roomId,
		UserID:     c.userId,
		LayerIndex: c.layer,
	}

	switch m.Operation {

	case "stroke-start":
		meta.ID = NextClock(meta.RoomID)
		m.Stroke.LayerIndex = c.layer
		StrokeBuffer.Mu.Lock()
		StrokeBuffer.Buffer[m.ID] = &bufferStruct{
			Stroke: m.Stroke,
			Meta:   meta,
			TTL:    time.Now().Add(StrokeTTL).UnixMilli(),
		}
		StrokeBuffer.Mu.Unlock()

		return &config.ServerMsg{
			Clock:   meta.ID,
			Payload: m,
		}

	case "stroke-update":
		StrokeBuffer.Mu.Lock()
		b, ok := StrokeBuffer.Buffer[m.ID]
		if ok {
			b.Stroke.Points = append(b.Stroke.Points, m.Points...)
			b.TTL = time.Now().Add(StrokeTTL).UnixMilli()
		}
		StrokeBuffer.Mu.Unlock()

		return &config.ServerMsg{
			Clock:   0,
			Payload: m,
		}

	case "stroke-end":
		StrokeBuffer.Mu.Lock()
		b, ok := StrokeBuffer.Buffer[m.ID]

		if !ok {
			StrokeBuffer.Mu.Unlock()
			return &config.ServerMsg{
				Clock:   0,
				Payload: m,
			}
		}
		delete(StrokeBuffer.Buffer, m.ID)
		StrokeBuffer.Mu.Unlock()

		// flush buffered stroke
		// fmt.Printf("%#v\n", b.Stroke)

		db.WriteEvent(config.Event{
			EventMeta: *b.Meta,
			Op:        "stroke-add",
			Payload:   middleware.EncodeNetworkMsg(b.Stroke),
			CreatedAt: time.Now().UnixMilli(),
			EntityID:  b.Stroke.ID,
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
		m.DomObject.LayerIndex = c.layer
		db.WriteEvent(config.Event{
			EventMeta: *meta,
			Op:        "dom-add",
			Payload:   middleware.EncodeNetworkMsg(m),
			CreatedAt: time.Now().UnixMilli(),
			EntityID:  m.ID,
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
			EntityID:  m.ID,
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
			EntityID:  m.ID,
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

	case "change-layer":

		targetLayer := m.Layer

		ok, err := db.CheckCanUseLayer(
			c.roomId,
			targetLayer.Index,
			c.userId,
		)

		if err != nil {
			log.Println("layer check error:", err)
			return nil
		}

		if !ok {
			deny := middleware.EncodeNetworkMsg([]config.ServerMsg{
				{
					Payload: config.NetworkMsg{
						Operation: "change-layer-denied",
						Layer: config.Layer{
							Index: c.layer,
						},
					},
					Clock: 0,
				},
			})
			if deny != nil {
				c.send <- deny
			}
			return nil
		}

		c.layer = targetLayer.Index

		ack := middleware.EncodeNetworkMsg([]config.ServerMsg{
			{
				Payload: config.NetworkMsg{
					Operation: "change-layer-accept",
					Layer: config.Layer{
						Index: c.layer,
					},
				},
				Clock: 0,
			},
		})

		if ack != nil {
			c.send <- ack
		}
		return nil

	default:
		return &config.ServerMsg{
			Clock:   0,
			Payload: m,
		}
	}
}

func (h *Hub) GetClients(roomId string) []*Client {
	h.mu.Lock()
	defer h.mu.Unlock()

	room, ok := h.rooms[roomId]
	if !ok {
		return nil
	}

	clients := make([]*Client, 0, len(room.clients))
	for c := range room.clients {
		clients = append(clients, c)
	}

	return clients
}

func StartStrokeTTLGC() {
	ticker := time.NewTicker(1 * time.Minute)

	go func() {
		for range ticker.C {
			now := time.Now().UnixMilli()

			StrokeBuffer.Mu.Lock()
			for id, b := range StrokeBuffer.Buffer {
				if b.TTL > 0 && b.TTL <= now {
					// Drop expired stroke (never ended properly)
					delete(StrokeBuffer.Buffer, id)
				}
			}
			StrokeBuffer.Mu.Unlock()
		}
	}()
}
