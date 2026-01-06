package ws

import (
	"log"
	"net/http"

	"github.com/Tk21111/whiteboard_server/auth"
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

	userId, err := auth.VerifyIDToken(token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}

	client := &Client{
		conn:   conn,
		send:   make(chan []byte, 256),
		roomId: roomId,
		userId: userId,
	}

	H.Join(roomId, client)
	log.Println("join room", roomId, "user", userId)

	go client.write()
	client.read()
}
