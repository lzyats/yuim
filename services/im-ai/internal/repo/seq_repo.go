package repo

import (
	"context"
	"database/sql"
)

// SeqRepo generates per-conversation sequence atomically using MySQL LAST_INSERT_ID trick.
type SeqRepo struct {
	db *sql.DB
}

func NewSeqRepo(db *sql.DB) *SeqRepo { return &SeqRepo{db: db} }

// NextConvSeq returns next sequence for conv_id within the provided transaction.
func (r *SeqRepo) NextConvSeq(ctx context.Context, tx *sql.Tx, convID string) (int64, error) {
	// Insert seq=0 then increment; LAST_INSERT_ID returns the new seq.
	// Requires conv_id as PK.
	_, err := tx.ExecContext(ctx, `
INSERT INTO im_conv_seq (conv_id, seq)
VALUES (?, 0)
ON DUPLICATE KEY UPDATE seq = LAST_INSERT_ID(seq + 1)
`, convID)
	if err != nil {
		return 0, err
	}
	var seq int64
	if err := tx.QueryRowContext(ctx, `SELECT LAST_INSERT_ID()`).Scan(&seq); err != nil {
		return 0, err
	}
	// For new row, LAST_INSERT_ID() will be 0 (from VALUES), so bump to 1 by updating once more.
	if seq == 0 {
		_, err := tx.ExecContext(ctx, `UPDATE im_conv_seq SET seq = LAST_INSERT_ID(seq + 1) WHERE conv_id = ?`, convID)
		if err != nil {
			return 0, err
		}
		if err := tx.QueryRowContext(ctx, `SELECT LAST_INSERT_ID()`).Scan(&seq); err != nil {
			return 0, err
		}
	}
	return seq, nil
}
