package outbox

import (
	"context"
	"database/sql"
	"time"
)

type Record struct {
	ID          int64
	Event       string
	MsgID       int64
	ConvID      string
	SyncID      int64
	Topic       string
	Tag         string
	PayloadJSON string
	Status      int
	RetryCount  int
	NextRetryAt time.Time
	LastError   string
}

type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// EnqueueTx is idempotent by UNIQUE(event, msg_id). It returns outbox_id.
func (r *Repo) EnqueueTx(ctx context.Context, tx *sql.Tx, event string, msgID int64, convID string, syncID int64, topic, tag, payloadJSON string) (int64, error) {
	if tag == "" {
		tag = "*"
	}

	res, err := tx.ExecContext(ctx, `
INSERT INTO im_outbox (event, msg_id, conv_id, sync_id, topic, tag, payload_json, status, retry_count, next_retry_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, NOW())
ON DUPLICATE KEY UPDATE
  payload_json=VALUES(payload_json),
  topic=VALUES(topic),
  tag=VALUES(tag),
  next_retry_at=LEAST(next_retry_at, NOW()),
  id=LAST_INSERT_ID(id)
`, event, msgID, convID, syncID, topic, tag, payloadJSON)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil || id == 0 {
		var qid int64
		if e := tx.QueryRowContext(ctx, `SELECT id FROM im_outbox WHERE event=? AND msg_id=? LIMIT 1`, event, msgID).Scan(&qid); e != nil {
			if err != nil {
				return 0, err
			}
			return 0, e
		}
		return qid, nil
	}
	return id, nil
}

func (r *Repo) FetchDue(ctx context.Context, limit int) ([]Record, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, event, msg_id, conv_id, sync_id, topic, tag, payload_json, status, retry_count, next_retry_at, last_error
FROM im_outbox
WHERE status = 0 AND next_retry_at <= NOW()
ORDER BY id ASC
LIMIT ?
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ID, &rec.Event, &rec.MsgID, &rec.ConvID, &rec.SyncID, &rec.Topic, &rec.Tag, &rec.PayloadJSON,
			&rec.Status, &rec.RetryCount, &rec.NextRetryAt, &rec.LastError); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *Repo) MarkSent(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE im_outbox SET status=1, last_error='' WHERE id=?`, id)
	return err
}

func (r *Repo) MarkFailed(ctx context.Context, id int64, retryCount int, lastErr string, backoff time.Duration) error {
	if backoff <= 0 {
		backoff = 1 * time.Second
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE im_outbox
SET retry_count=?, last_error=?, next_retry_at=DATE_ADD(NOW(), INTERVAL ? SECOND)
WHERE id=?
`, retryCount, truncate(lastErr, 255), int64(backoff.Seconds()), id)
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
