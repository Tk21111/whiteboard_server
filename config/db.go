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
