package event

// ImEvent is the internal MQ event envelope used by IM services (Logic -> MQ -> Job).
// Treat this as a contract (version it when breaking changes are required).
type ImEvent struct {
	Event   string            `json:"event"`
	TraceID string            `json:"trace_id"`
	TS      int64             `json:"ts"` // unix seconds
	FromUID int64             `json:"from_uid"`
	ToUIDs  []int64           `json:"to_uids"`
	ConvID  string            `json:"conv_id"`
	Msg     Message           `json:"msg"`
	Flags   map[string]bool   `json:"flags,omitempty"`
	Meta    map[string]string `json:"meta,omitempty"`
}

type Message struct {
	MsgID       int64              `json:"msg_id"`
	ClientMsgID string             `json:"client_msg_id,omitempty"`
	MsgType     string             `json:"msg_type"`
	Seq         int64              `json:"seq"`
	Content     map[string]any      `json:"content,omitempty"`
	Extra       map[string]string   `json:"extra,omitempty"`
}
