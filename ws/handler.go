package ws

import (
	"log"
	"net/http"

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
		layer:   0,
	}

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

	/* --------------------------------------------------
	   3. JOIN ROOM
	   -------------------------------------------------- */

	H.Join(roomId, client)

	log.Println("join room", roomId, "user", client.userId)

	/* --------------------------------------------------
	   4. START IO
	   -------------------------------------------------- */

	go client.write()
	client.read()
}
