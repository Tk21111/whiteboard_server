package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/Tk21111/whiteboard_server/auth"
	"github.com/Tk21111/whiteboard_server/config"
	"github.com/Tk21111/whiteboard_server/db"
	"github.com/Tk21111/whiteboard_server/middleware"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func HandleWS(w http.ResponseWriter, r *http.Request) {
	roomId := r.URL.Query().Get("roomId")
	token := r.URL.Query().Get("token")

	if roomId == "" || token == "" {
		http.Error(w, "missing params", http.StatusUnauthorized)
		return
	}

	user, err := auth.VerifyIDToken(token)
	fmt.Println(user)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	role, err := db.EnsureUserInRoom(roomId, user.UserID)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}

	color := middleware.ColorFromUserID(user.UserID)

	client := &Client{
		conn:    conn,
		send:    make(chan []byte, 256),
		roomId:  roomId,
		userId:  user.UserID,
		name:    user.Name,
		profile: user.Picture,
		color:   color,
		role:    role,
		layer:   atomic.Int64{},
	}

	client.layer.Store(0)

	/* --------------------------------------------------
	   1. SEND EXISTING CLIENTS -> NEW CLIENT
	   -------------------------------------------------- */

	existingClients := H.GetClients(roomId)

	if len(existingClients) > 0 {
		msgs := make([]config.ServerMsg, 0, len(existingClients))

		for _, c := range existingClients {
			msgs = append(msgs, config.ServerMsg{
				Payload: config.NetworkMsg{
					Operation: "client-join",
					ID:        c.userId,
					ClientData: &config.ClientData{
						ID:      c.userId,
						Name:    c.name,
						Color:   c.color,
						Profile: c.profile,
					},
				},
				Clock: 0,
			})
		}

		data := middleware.EncodeNetworkMsg(msgs)
		if data != nil {
			client.send <- data
		}
	}

	replay, err := GetReplay(client.userId, client.roomId, client.layer.Load(), "0")
	if err != nil {
		return
	}
	data := middleware.EncodeNetworkMsg(replay)
	if data != nil {
		client.send <- data
	}

	/* --------------------------------------------------
	   2. BROADCAST NEW CLIENT -> OTHERS
	   -------------------------------------------------- */

	selfJoin := middleware.EncodeNetworkMsg([]config.ServerMsg{
		{
			Payload: config.NetworkMsg{
				Operation: "client-join",
				ID:        client.userId,
				ClientData: &config.ClientData{
					ID:      client.userId,
					Name:    user.Name,
					Color:   color,
					Profile: client.profile,
				},
			},
			Clock: 0,
		},
	})

	if selfJoin == nil {
		log.Println("encode client-join failed")
		return
	}

	H.Broadcast(roomId, selfJoin, nil)

	H.Join(roomId, client)
	log.Println("join room", roomId, "user", client.userId)

	go client.write()
	client.read()
}

func GetReplay(userID string, roomID string, layerIndex int64, from string) ([]config.ServerMsg, error) {
	replay := make([]config.ServerMsg, 0) // Changed from 1 to 0

	_, err := db.EnsureUserInRoom(roomID, userID)
	if err != nil {
		return nil, err
	}

	if from == "" {
		from = "0"
	}

	events, err := db.GetEvent(roomID, from, int(layerIndex))
	if err != nil {
		return nil, err
	}

	for _, e := range events {
		payload := config.NetworkMsg{
			ID:        strconv.FormatInt(e.ID, 10),
			Operation: e.Op,
		}

		switch e.Op {
		case "stroke-add":
			var decoded config.StrokeObjectInterface
			if err := json.Unmarshal(e.Payload, &decoded); err != nil {
				continue
			}
			payload.Stroke = &decoded
		}

		replay = append(replay, config.ServerMsg{
			Clock:   e.ID,
			Payload: payload,
		})
	}

	doms, err := db.GetActiveDomObjects(roomID, layerIndex)
	if err != nil {
		return replay, err
	}

	for _, d := range doms {
		replay = append(replay, config.ServerMsg{
			Clock: 0,
			Payload: config.NetworkMsg{
				ID:        d.ID,
				Operation: "dom-add",
				DomObject: &d,
			},
		})
	}

	StrokeBuffer.Mu.Lock()
	defer StrokeBuffer.Mu.Unlock()

	for _, d := range StrokeBuffer.Buffer {
		if d.Meta.RoomID != roomID || d.Stroke.LayerIndex != layerIndex {
			continue
		}

		replay = append(replay, config.ServerMsg{
			Clock: 0,
			Payload: config.NetworkMsg{
				ID:        d.Stroke.ID,
				Operation: "stroke-start",
				Stroke:    d.Stroke,
			},
		})
	}

	domLocks.Mu.Lock()
	defer domLocks.Mu.Unlock()

	for d, userId := range domLocks.buffer {
		replay = append(replay, config.ServerMsg{
			Clock: 0,
			Payload: config.NetworkMsg{
				ID:        d,
				Operation: "dom-lock",
				DomObject: &config.DomObjectNetwork{
					ID:     d,
					UserId: userId,
				},
			},
		})
	}

	return replay, nil
}
