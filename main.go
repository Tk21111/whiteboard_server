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
	conn *websocket.Conn
	send chan []byte
}

var (
	clients   = make(map[*Client]bool)
	broadcast = make(chan []byte)
	mu        sync.Mutex
)

func main() {
	http.HandleFunc("/ws", handlews)

	go boardcaster()

	log.Println("WS server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handlews(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade err : ", err)
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
	}

	mu.Lock()
	clients[client] = true
	mu.Unlock()

	log.Println("client created")

	go client.write()
	client.read()

}

func (client *Client) read() {
	defer func() {
		mu.Lock()
		delete(clients, client)
		mu.Unlock()
		client.conn.Close()
		log.Println("Connection Close")
	}()

	for {
		_, msg, err := client.conn.ReadMessage()
		if err != nil {
			//connection is dead just stop
			break
		}

		broadcast <- msg
	}
}

func (client *Client) write() {
	for msg := range client.send {
		if err := client.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func boardcaster() {
	for msg := range broadcast {
		mu.Lock()
		for client := range clients {
			select {
			case client.send <- msg:
			default:
				delete(clients, client)
				close(client.send)
			}
		}
		mu.Unlock()
	}
}
