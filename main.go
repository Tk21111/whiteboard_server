package main

import (
	"log"
	"net/http"

	"github.com/Tk21111/whiteboard_server/ws"
)

func main() {
	http.HandleFunc("/ws", ws.HandleWS)

	log.Println("WS running :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
