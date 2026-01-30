package stat

import (
	"context"
	"database/sql"

	"gitlab.com/thorchain/midgard/internal/db"
)

func SwitchedRune(ctx context.Context) (int64, error) {
	q := `SELECT COALESCE(SUM(mint_e8), 0) FROM switch_events`

	var ret int64
	err := db.TheDB.QueryRow(q).Scan(&ret)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return ret, err
}
