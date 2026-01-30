package repo

import (
	"database/sql"
	"time"
)

// ChatMsg is the DB model for im_msg (or legacy chat_msg).
// Nullable fields use sql.Null* to avoid ambiguity.
type ChatMsg struct {
	MsgID      int64
	SyncID     int64
	UserID     int64
	ReceiveID  sql.NullInt64
	GroupID    sql.NullInt64
	TalkType   string
	MsgType    string
	Content    string
	CreateTime time.Time

	ConvID      sql.NullString
	ClientMsgID sql.NullString
}
