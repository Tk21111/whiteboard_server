package main

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // allow all origins (OK for dev)
	},
}

type Client struct {
	conn   *websocket.Conn
	send   chan []byte
	roomId string
}

type Room struct {
	id        string
	clients   map[*Client]bool
	broadcast chan []byte
	mu        sync.Mutex
}

var (
	rooms   = make(map[string]*Room)
	roomsMu sync.Mutex
)

func main() {
	http.HandleFunc("/ws", handleWS)

	log.Println("WS server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	roomId := r.URL.Query().Get("roomId")
	if roomId == "" {
		http.Error(w, "roomId , require", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("[ws] upgrade err :")
		return
	}

	room := getOrCreateRoom(roomId)

	client := &Client{
		conn:   conn,
		send:   make(chan []byte),
		roomId: roomId,
	}

	room.mu.Lock()
	room.clients[client] = true
	room.mu.Unlock()

	log.Println("client joined room:", roomId)

	go client.write()
	client.read(room)
}

func getOrCreateRoom(roomId string) *Room {
	roomsMu.Lock()
	defer roomsMu.Unlock()

	room, ok := rooms[roomId]
	if !ok {
		room = &Room{
			id:        roomId,
			clients:   make(map[*Client]bool),
			broadcast: make(chan []byte, 256),
		}
		rooms[roomId] = room
		go room.run()
	}

	return room
}

func (room *Room) run() {
	for msg := range room.broadcast {
		room.mu.Lock()
		for client := range room.clients {
			select {
			case client.send <- msg:
			default:
				delete(room.clients, client)
				close(client.send)
			}
		}
		room.mu.Unlock()
	}
}

func (client *Client) read(room *Room) {
	defer func() {
		room.mu.Lock()
		delete(room.clients, client)
		room.mu.Unlock()

		client.conn.Close()
		log.Println("client left room:", client.roomId)
	}()

	for {
		_, msg, err := client.conn.ReadMessage()
		if err != nil {
			break
		}

		room.broadcast <- msg
	}
}

func (client *Client) write() {
	for msg := range client.send {
		if err := client.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}
