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
	Payload   *string           `json:"payload,omitempty"`

	ClientData *ClientData `json:"clientData,omitempty"`
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
	Operation string  `json:"operation"` // draw / erase
	Opacity   float64 `json:"opacity"`
	Size      int64   `json:"size"`
	Points    []Point `json:"points"`
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	P float64 `json:"pressure"`
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
	Transform Transform `json:"transform"`
	Payload   string    `json:"payload"`
}

type ClientData struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
	Profile string `json:"profile"`
}
