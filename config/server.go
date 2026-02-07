package config

type ServerMsg struct {
	Clock   int64      `json:"clock"`
	Payload NetworkMsg `json:"payload"`
}

type Role int

const (
	RoleGuest     Role = iota // 0
	RoleMember                // 1
	RoleModerator             // 2
	RoleOwner                 // 3
)
