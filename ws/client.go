package ws

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Tk21111/whiteboard_server/config"
	"github.com/Tk21111/whiteboard_server/middleware"
	"github.com/gorilla/websocket"
)

type Client struct {
	conn    *websocket.Conn
	send    chan []byte
	roomId  string
	userId  string
	profile string
	color   string
	name    string

	closeOnce sync.Once
}

func (c *Client) read() {
	defer c.close()

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		msgs, err := middleware.DecodeNetworkMsg(raw)
		if err != nil {
			continue
		}

		// Use the concrete type for the slice
		var responses []config.ServerMsg
		for _, m := range msgs {
			res := c.handleMsg(m)
			if res != nil {
				responses = append(responses, *res)
			}
		}

		if len(responses) == 0 {
			continue
		}

		data, err := json.Marshal(responses)
		if err != nil {
			continue
		}

		H.Broadcast(c.roomId, data, c)
	}
}

func (c *Client) write() {
	ticker := time.NewTicker(54 * time.Second) // Must be less than browser timeout
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				fmt.Println("ping fail force close")
				return
			}
		}
	}
}

func (c *Client) close() {
	c.closeOnce.Do(func() {
		H.Leave(c.roomId, c)
		close(c.send)
		_ = c.conn.Close()
	})
}
