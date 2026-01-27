package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type ChatMsg struct {
	MsgID       int64
	SyncID      int64
	UserID      int64
	ReceiveID   int64
	GroupID     sql.NullInt64
	TalkType    string
	MsgType     string
	Content     string
	CreateTime  time.Time
	ConvID      string
	ClientMsgID string
}

type ChatMsgRepo struct {
	db *sql.DB
}

func NewChatMsgRepo(db *sql.DB) *ChatMsgRepo { return &ChatMsgRepo{db: db} }

func (r *ChatMsgRepo) InsertTx(ctx context.Context, tx *sql.Tx, m *ChatMsg) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO chat_msg
(msg_id, sync_id, user_id, receive_id, group_id, talk_type, msg_type, content, create_time, conv_id, client_msg_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW(), ?, ?)
`, m.MsgID, m.SyncID, m.UserID, m.ReceiveID, m.GroupID, m.TalkType, m.MsgType, m.Content, m.ConvID, m.ClientMsgID)
	return err
}

func (r *ChatMsgRepo) ListByConvAfterSync(ctx context.Context, uid int64, convID string, afterSync int64, limit int) ([]ChatMsg, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT msg_id, sync_id, user_id, receive_id, group_id, talk_type, msg_type, content, create_time, conv_id, client_msg_id
FROM chat_msg
WHERE conv_id = ? AND sync_id > ? AND (user_id = ? OR receive_id = ?)
ORDER BY sync_id ASC
LIMIT ?
`, convID, afterSync, uid, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ChatMsg, 0, limit)
	for rows.Next() {
		var m ChatMsg
		if err := rows.Scan(&m.MsgID, &m.SyncID, &m.UserID, &m.ReceiveID, &m.GroupID, &m.TalkType, &m.MsgType, &m.Content, &m.CreateTime, &m.ConvID, &m.ClientMsgID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func MustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
