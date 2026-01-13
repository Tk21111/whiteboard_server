package config

import (
	"encoding/json"
)

type Event struct {
	EventMeta

	EntityID string `json:"entityId"`
	Op       string `json:"op"`

	Payload   json.RawMessage `json:"payload"`
	CreatedAt int64           `json:"ts"`
}

type DomEvent struct {
	DomObjectNetwork

	RoomID    string `json:"roomId"`
	UserID    string `json:"userId"`
	CreatedAt int64  `json:"ts"`
	UpdatedAt int64  `json:"update_at"`
}
