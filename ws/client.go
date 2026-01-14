package ws

import (
	"encoding/json"
	"time"

	"github.com/Tk21111/whiteboard_server/config"
	"github.com/Tk21111/whiteboard_server/middleware"
	"github.com/gorilla/websocket"
)

type Client struct {
	conn   *websocket.Conn
	send   chan []byte
	roomId string
	userId string
}

func (c *Client) read() {
	defer func() {
		H.Leave(c.roomId, c)
		c.conn.Close()
	}()

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
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				// Hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			// Send a Ping to keep the connection alive
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
