package ws

import (
	"github.com/Tk21111/whiteboard_server/config"
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
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		Ch <- config.RawEvent{
			Msg: msg,
			Meta: &config.EventMeta{
				ID:     0,
				RoomID: c.roomId,
				UserID: c.userId,
			},
		}
		H.Broadcast(c.roomId, msg, c)

	}
}

func (c *Client) write() {
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}
