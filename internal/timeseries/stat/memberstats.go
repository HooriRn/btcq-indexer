package stat

import (
	"context"
	"database/sql"

	"gitlab.com/thorchain/midgard/internal/db"
)

func membersCount(ctx context.Context, pools []string, until *db.Nano) (map[string]int64, error) {
	timeFilter := ""
	qargs := []interface{}{pools}
	if until != nil {
		timeFilter = "block_timestamp < $2"
		qargs = append(qargs, *until)
	}

	q := `
		SELECT
			DISTINCT on (pool) pool, count 
		FROM midgard_agg.members_count
		` + db.Where(timeFilter, "pool = ANY($1)") + `
		ORDER BY pool, block_timestamp DESC
	`

	poolsCount := make(map[string]int64)
	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var pool string
		var count int64
		err := rows.Scan(&pool, &count)
		if err != nil {
			return nil, err
		}
		poolsCount[pool] = count
	}

	return poolsCount, nil
}

type CountBucket struct {
	Window db.Window
	Count  int64
}

func GetMembersCountBucket(ctx context.Context, buckets db.Buckets, pool string) (
	beforeCount int64, ret []CountBucket, err error) {
	startTime := buckets.Window().From.ToNano()
	lastValueMap, err := membersCount(ctx, []string{pool}, &startTime)
	if err != nil {
		return 0, nil, err
	}

	lastCountValue := lastValueMap[pool]
	beforeCount = lastCountValue

	q := `
		SELECT DISTINCT ON (truncated)
			` + db.SelectTruncatedTimestamp("block_timestamp", buckets) + ` AS truncated,
			count
		FROM midgard_agg.members_count
		WHERE pool = $1 AND $2 <= block_timestamp AND block_timestamp < $3
		ORDER BY truncated, block_timestamp DESC

	`

	qargs := []interface{}{pool, buckets.Start().ToNano(), buckets.End().ToNano()}

	ret = make([]CountBucket, buckets.Count())
	var nextValue int64

	readNext := func(rows *sql.Rows) (nextTimestamp db.Second, err error) {
		err = rows.Scan(&nextTimestamp, &nextValue)
		if err != nil {
			return 0, err
		}
		return
	}
	nextIsCurrent := func() { lastCountValue = nextValue }
	saveBucket := func(idx int, bucketWindow db.Window) {
		ret[idx].Window = bucketWindow
		ret[idx].Count = lastCountValue
	}

	err = queryBucketedGeneral(ctx, buckets, readNext, nextIsCurrent, saveBucket, q, qargs...)
	if err != nil {
		return 0, nil, err
	}

	return beforeCount, ret, nil

}

func aggMembersCounts(ctx context.Context, tableName string, until *db.Nano) (int64, error) {
	timeFilter := ""
	qargs := []interface{}{}
	if until != nil {
		timeFilter = "block_timestamp < $1"
		qargs = append(qargs, *until)
	}

	q := `
		SELECT
			count 
		FROM ` + tableName + ` ` + db.Where(timeFilter) + `
		ORDER BY block_timestamp DESC
	`

	var totalCounts int64
	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var count int64
		err := rows.Scan(&count)
		if err != nil {
			return 0, err
		}
		totalCounts = count
	}

	return totalCounts, nil
}

func GetAggMembersCountBucket(ctx context.Context, buckets db.Buckets, tableName string) (
	beforeCount int64, ret []CountBucket, err error) {
	startTime := buckets.Window().From.ToNano()
	lastCountValue, err := aggMembersCounts(ctx, tableName, &startTime)
	if err != nil {
		return 0, nil, err
	}

	beforeCount = lastCountValue

	q := `
		SELECT DISTINCT ON (truncated)
			` + db.SelectTruncatedTimestamp("block_timestamp", buckets) + ` AS truncated,
			count
		FROM ` + tableName + `
		WHERE $1 <= block_timestamp AND block_timestamp < $2
		ORDER BY truncated, block_timestamp DESC

	`

	qargs := []interface{}{buckets.Start().ToNano(), buckets.End().ToNano()}

	ret = make([]CountBucket, buckets.Count())
	var nextValue int64

	readNext := func(rows *sql.Rows) (nextTimestamp db.Second, err error) {
		err = rows.Scan(&nextTimestamp, &nextValue)
		if err != nil {
			return 0, err
		}
		return
	}
	nextIsCurrent := func() { lastCountValue = nextValue }
	saveBucket := func(idx int, bucketWindow db.Window) {
		ret[idx].Window = bucketWindow
		ret[idx].Count = lastCountValue
	}

	err = queryBucketedGeneral(ctx, buckets, readNext, nextIsCurrent, saveBucket, q, qargs...)
	if err != nil {
		return 0, nil, err
	}

	return beforeCount, ret, nil
}
