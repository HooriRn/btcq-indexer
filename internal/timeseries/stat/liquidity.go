package stat

import (
	"context"
	"strconv"

	"github.com/btcq/btcq-indexer/internal/db"
)

// TODO (HooriRn): Delete imp protection from the code and table
type liquidityBucket struct {
	assetVolume int64
	qbtcVolume  int64
	volume      int64
	count       int64
}

type liquidityOneTableResult struct {
	total   liquidityBucket
	buckets map[db.Second]liquidityBucket
}

func liquidityChangesFromTable(
	ctx context.Context, buckets db.Buckets, pool string,
	table, assetColumn, qbtcColumn, impLossProtColumn string) (
	ret liquidityOneTableResult, err error) {

	window := buckets.Window()

	// NOTE: pool filter and arguments are the same in all queries
	var poolFilter string
	queryArguments := []interface{}{window.From.ToNano(), window.Until.ToNano()}
	if pool != "*" {
		poolFilter = "pool = $3 AND "
		queryArguments = append(queryArguments, pool)
	}

	// GET DATA
	// TODO(acsaba): To get the depths for a given timestamp, we join by block_timestamp, assuming
	// there will always be a row in block_pool_depths as depth is being changed by the event
	// itself on that block. This won't be the case if for some reason there are other events
	// and the depth ends up being the same than previous block as new row won't be stored
	// Even though unlikely, we need to guard against this.
	query := `
	SELECT
		COUNT(*) AS count,
		SUM(` + assetColumn + `) AS asset_in_qbtc_sum,
		SUM(` + qbtcColumn + `) as qbtc_sum,
		` + db.SelectTruncatedTimestamp("block_timestamp", buckets) + ` AS start_time
	FROM ` + table + `
	WHERE ` + poolFilter + `$1 <= block_timestamp AND block_timestamp < $2
	GROUP BY start_time
	`

	rows, err := db.Query(ctx, query, queryArguments...)
	if err != nil {
		return
	}
	defer rows.Close()

	ret.buckets = map[db.Second]liquidityBucket{}

	// Store query results into aggregate variables
	for rows.Next() {
		var bucket liquidityBucket
		var startTime db.Second
		err = rows.Scan(
			&bucket.count, &bucket.assetVolume, &bucket.qbtcVolume, &startTime)
		if err != nil {
			return
		}
		bucket.volume = bucket.assetVolume + bucket.qbtcVolume

		ret.buckets[startTime] = bucket
		ret.total.assetVolume += bucket.assetVolume
		ret.total.qbtcVolume += bucket.qbtcVolume
		ret.total.volume += bucket.volume
		ret.total.count += bucket.count
	}

	return
}

func floatStr(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
