package ws

import (
	"encoding/json"

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

		var msg []config.ServerMsg
		for _, m := range msgs {
			msg = append(msg, c.handleMsg(m))
		}

		data, err := json.Marshal(msg)
		if err != nil {
			return
		}

		H.Broadcast(c.roomId, data, c)
	}
}

func (c *Client) write() {
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}
