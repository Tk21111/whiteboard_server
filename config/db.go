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

type RoomEvent struct {
	RoomID string
	UserID string
	Now    int64
	Public int8
	Role   Role
	Result chan error
}

type UserEvent struct {
	UserID string
	Role   Role

	Name      string
	GivenName string
	Email     string

	Created_at int64
}

type Area struct {
	X    int `json:"x"`
	Y    int `json:"y"`
	Size int `json:"size"`
}

type LayerEvent struct {
	RoomID     string
	UserID     string
	Name       string
	Public     int
	Now        int64
	Result     chan error
	LayerIndex chan int64
}
