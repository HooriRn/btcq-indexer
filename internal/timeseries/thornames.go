package timeseries

import (
	"context"
	"strings"

	"gitlab.com/thorchain/midgard/internal/db"
)

type THORNameEntry struct {
	Chain   string
	Address string
}

type THORName struct {
	Owner   string
	Expire  int64
	Entries []THORNameEntry
}

func GetTHORName(ctx context.Context, name string) (tName THORName, err error) {
	q := `
		SELECT chain, address, expire, owner
		FROM midgard_agg.thorname_current_state
		WHERE name = $1 AND last_height() < expire
	`

	rows, err := db.Query(ctx, q, strings.ToLower(name))
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var entry THORNameEntry
		if err := rows.Scan(&entry.Chain, &entry.Address, &tName.Expire, &tName.Owner); err != nil {
			return tName, err
		}
		tName.Entries = append(tName.Entries, entry)
	}

	return
}

// NOTE: there is probably a pure-postrgres means of doing this, which would be
// more performant. If we find that the performance of this query to be too
// slow, can try that. I don't imagine it being much of a problem since people
// aren't going to associate their address with 100's of thornames
func GetTHORNamesByAddress(ctx context.Context, addr string) (names []string, err error) {
	q := `
		SELECT name
		FROM midgard_agg.thorname_current_state
		WHERE address = $1 AND last_height() < expire
	`

	rows, err := db.Query(ctx, q, addr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}

		names = append(names, name)
	}

	return
}

func GetTHORNamesByOwnerAddress(ctx context.Context, addr string) (names []string, err error) {
	q := `
		SELECT name
		FROM midgard_agg.thorname_owner_expiration
		WHERE owner = $1 AND last_height() < expire
	`

	rows, err := db.Query(ctx, q, addr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}

		names = append(names, name)
	}

	return
}
