package stat

import (
	"context"
	"database/sql"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
)

func totalRUNEPoolUnitChanges(ctx context.Context, tableName string, until *db.Nano) (
	int64, error) {
	timeFilter := ""
	qargs := []interface{}{}
	if until != nil {
		timeFilter = "block_timestamp < $1"
		qargs = append(qargs, *until)
	}

	q := `
		SELECT
			COALESCE(SUM(units), 0) as units
		FROM ` + tableName + `
		` + db.Where(timeFilter)

	var totalUnits int64
	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var units int64
		err := rows.Scan(&units)
		if err != nil {
			return 0, err
		}
		return units, nil
	}

	return totalUnits, nil
}

func bucketedRUNEPoolUnitChanges(ctx context.Context, buckets db.Buckets, tableName string) (
	beforeUnit int64, ret []UnitsBucket, err error) {
	startTime := buckets.Window().From.ToNano()
	lastValue, err := totalRUNEPoolUnitChanges(ctx, tableName, &startTime)
	if err != nil {
		return 0, nil, err
	}
	beforeUnit = lastValue
	q := `
		SELECT
			COALESCE(SUM(units), 0) as units,
			` + db.SelectTruncatedTimestamp("block_timestamp", buckets) + ` AS truncated
		FROM ` + tableName + `
		WHERE $1 <= block_timestamp AND block_timestamp < $2
		GROUP BY truncated
		ORDER BY truncated ASC
	`

	qargs := []interface{}{buckets.Start().ToNano(), buckets.End().ToNano()}

	ret = make([]UnitsBucket, buckets.Count())
	var nextValue int64

	readNext := func(rows *sql.Rows) (nextTimestamp db.Second, err error) {
		err = rows.Scan(&nextValue, &nextTimestamp)
		if err != nil {
			return 0, err
		}
		return
	}
	nextIsCurrent := func() { lastValue += nextValue }
	saveBucket := func(idx int, bucketWindow db.Window) {
		ret[idx].Window = bucketWindow
		ret[idx].Units = lastValue
	}

	err = queryBucketedGeneral(ctx, buckets, readNext, nextIsCurrent, saveBucket, q, qargs...)
	if err != nil {
		return 0, nil, err
	}

	return beforeUnit, ret, nil
}

// Not including the until timestamp
func RUNEPoolLiquidityUnitsBefore(ctx context.Context, until *db.Nano) (int64, error) {
	stakes, err := totalRUNEPoolUnitChanges(ctx, "rune_pool_deposit_events", until)
	if err != nil {
		return 0, err
	}
	withdraws, err := totalRUNEPoolUnitChanges(ctx, "rune_pool_withdraw_events", until)
	if err != nil {
		return 0, err
	}
	return stakes - withdraws, nil
}

// PoolUnits gets net liquidity units in pools
func CurrentRUNEPoolsLiquidityUnits(ctx context.Context) (int64, error) {
	return RUNEPoolLiquidityUnitsBefore(ctx, nil)
}

// PoolUnits gets net liquidity units in pools
func RUNEPoolUnitsHistory(ctx context.Context, buckets db.Buckets) (int64, []UnitsBucket, error) {
	beforeUnitStake, ret, err := bucketedRUNEPoolUnitChanges(ctx, buckets, "rune_pool_deposit_events")
	if err != nil {
		return 0, nil, err
	}
	beforeUnitWithdraw, withdraws, err := bucketedRUNEPoolUnitChanges(ctx, buckets, "rune_pool_withdraw_events")
	if err != nil {
		return 0, nil, err
	}
	if len(ret) != len(withdraws) {
		return 0, nil, miderr.InternalErr("bucket count is different for deposits and withdraws")
	}
	for i := range ret {
		ret[i].Units -= withdraws[i].Units
	}
	beforeUnit := beforeUnitStake - beforeUnitWithdraw
	return beforeUnit, ret, nil
}
