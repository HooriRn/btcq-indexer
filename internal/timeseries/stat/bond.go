package stat

import (
	"context"
	"database/sql"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

func GetTotalBond(ctx context.Context, time db.Nano) (int64, error) {
	timeFilter := ""
	qargs := []interface{}{}
	if 0 < time {
		timeFilter = "block_timestamp < $1"
		qargs = []interface{}{time}
	}

	q := `
		SELECT
			SUM(E8),
			bond_type
		FROM bond_events
		` + db.Where(timeFilter) + `
		GROUP BY bond_type
	`
	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var totalBond int64
	var event_type string

	for rows.Next() {
		var x int64
		err := rows.Scan(&x, &event_type)
		if err != nil {
			return 0, err
		}
		totalBond += bondValueForType(event_type, x)
	}

	return totalBond, nil
}

type BondBucket struct {
	Window db.Window
	Bonds  int64
}

func bondValueForType(event_type string, e8 int64) int64 {
	switch event_type {
	case "bond_paid":
		return e8
	case "bond_reward":
		return e8
	case "bond_returned":
		return -e8
	case "bond_cost":
		return -e8
	default:
		midlog.ErrorF("Unrecognized bond event type: %s", event_type)
	}
	return 0
}

func BondsHistory(ctx context.Context, buckets db.Buckets) (
	ret []BondBucket, err error) {
	totalBonds, err := GetTotalBond(ctx, buckets.Start().ToNano())
	if err != nil {
		return nil, err
	}
	ret = make([]BondBucket, buckets.Count())

	q := `
	SELECT
		SUM(E8),
		bond_type,
		` + db.SelectTruncatedTimestamp("block_timestamp", buckets) + ` as truncated
	FROM bond_events
	WHERE $1 <= block_timestamp AND block_timestamp < $2
	GROUP BY truncated, bond_type
	ORDER BY truncated ASC
	`

	var event_amount int64
	var event_type string

	scanNext := func(rows *sql.Rows) (timestamp db.Second, err error) {
		err = rows.Scan(&event_amount, &event_type, &timestamp)
		if err != nil {
			return 0, err
		}
		return
	}
	applyNext := func() {
		totalBonds += bondValueForType(event_type, event_amount)
	}
	saveBucket := func(idx int, bucketWindow db.Window) {
		ret[idx].Window = bucketWindow
		ret[idx].Bonds = totalBonds
	}

	err = queryBucketedGeneral(ctx, buckets, scanNext, applyNext, saveBucket, q, buckets.Start().ToNano(), buckets.End().ToNano())

	return ret, err
}
