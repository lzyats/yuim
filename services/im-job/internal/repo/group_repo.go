package repo

import (
	"context"
	"database/sql"
)

type GroupRepo struct {
	db *sql.DB
}

func NewGroupRepo(db *sql.DB) *GroupRepo { return &GroupRepo{db: db} }

func (r *GroupRepo) ListActiveMemberUIDs(ctx context.Context, groupID int64, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = 2000
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT user_id
FROM im_group_member
WHERE group_id=? AND status=1
ORDER BY user_id ASC
LIMIT ?
`, groupID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]int64, 0, limit)
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out = append(out, uid)
	}
	return out, rows.Err()
}
