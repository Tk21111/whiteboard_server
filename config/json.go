package config

type NetworkMsg struct {
	Operation string `json:"operation"`
	ID        string `json:"id"`

	// stroke
	Stroke *StrokeObjectInterface `json:"stroke,omitempty"`
	Points []Point                `json:"points,omitempty"`

	// dom
	Transform *Transform        `json:"transform,omitempty"`
	DomObject *DomObjectNetwork `json:"domObject,omitempty"`
}

type EventMeta struct {
	ID     int64  `json:"id"`
	RoomID string `json:"roomId"`
	UserID string `json:"userId"`
}

type StrokeObjectInterface struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"` // "stroke"
	Color     string  `json:"color"`
	Width     float64 `json:"width"`
	Operation string  `json:"operation"` // draw / erase
	Points    []Point `json:"points"`
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	T float64 `json:"t"`
}

type Transform struct {
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
	Rot float64 `json:"rot"`
	W   float64 `json:"w"`
	H   float64 `json:"h"`
}

type DomObjectNetwork struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"` // img, video, audio
	Transform Transform `json:"tranform"`
}
