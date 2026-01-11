package middleware

import (
	"encoding/json"

	"github.com/Tk21111/whiteboard_server/config"
)

func DecodeNetworkMsg(msg []byte) ([]config.NetworkMsg, error) {
	var m []config.NetworkMsg

	if err := json.Unmarshal(msg, &m); err != nil {
		return nil, err
	}

	return m, nil
}

func EncodeNetworkMsg(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
