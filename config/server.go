package config

type ServerMsg struct {
	Clock   int64      `json:"clock"`
	Payload NetworkMsg `json:"payload"`
}
