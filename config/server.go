package config

type RawEvent struct {
	Meta *EventMeta
	Msg  []byte
}
