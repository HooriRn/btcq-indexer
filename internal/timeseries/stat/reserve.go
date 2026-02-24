package stat

import (
	"context"
	"errors"

	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/fetch/notinchain"
)

type ValueBucket struct {
	Window db.Window
	Value  int64
}

func bucketedFeeStat(ctx context.Context, buckets db.Buckets) (ret []ValueBucket, err error) {
	// send to reserve module from 3x outbound fee
	q := `
		SELECT
			COALESCE(SUM(pool_deduct), 0) AS inflow,
				` + db.SelectTruncatedTimestamp("block_timestamp", buckets) + ` AS truncated
		FROM fee_events
		WHERE $1 <= block_timestamp AND block_timestamp < $2
		GROUP BY truncated
		ORDER BY truncated ASC
	`

	qargs := []interface{}{buckets.Start().ToNano(), buckets.End().ToNano()}

	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for i := 0; i < buckets.Count(); i++ {
		if rows.Next() {
			var bucket ValueBucket
			var from int64
			err := rows.Scan(&bucket.Value, &from)
			if err != nil {
				return ret, err
			}
			window := buckets.BucketWindow(i)
			if from != window.From.ToI() {
				return ret, errors.New("bucket isn't the same with the query")
			}
			bucket.Window = window
			ret = append(ret, bucket)
		}
	}

	return ret, nil
}

func bucketedNetworkFeeStat(ctx context.Context, buckets db.Buckets) (ret []ValueBucket, err error) {
	// send to reserve from native fee transactions
	reserveModule, err := notinchain.CachedReserveLookup()
	if err != nil {
		return nil, err
	}

	q := `
		SELECT
			COALESCE(SUM(amount_e8), 0) AS inflow,
				` + db.SelectTruncatedTimestamp("block_timestamp", buckets) + ` AS truncated
		FROM transfer_events
		WHERE $1 <= block_timestamp AND block_timestamp < $2 
		AND (to_addr = $3)
        AND (from_addr <> $3)
        AND (amount_e8 IN (2000000, 1))
		GROUP BY truncated
		ORDER BY truncated ASC
	`

	qargs := []interface{}{buckets.Start().ToNano(), buckets.End().ToNano(), reserveModule}

	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for i := 0; i < buckets.Count(); i++ {
		if rows.Next() {
			var bucket ValueBucket
			var from int64
			err := rows.Scan(&bucket.Value, &from)
			if err != nil {
				return ret, err
			}
			window := buckets.BucketWindow(i)
			if from != window.From.ToI() {
				return ret, errors.New("bucket isn't the same with the query")
			}
			bucket.Window = window
			ret = append(ret, bucket)
		}
	}

	return ret, nil
}

func bucketedGasStat(ctx context.Context, buckets db.Buckets) (ret []ValueBucket, err error) {
	// send from reserve to the pool module - counts as outflow/expense

	q := `
		SELECT
			COALESCE(SUM(qbtc_e8), 0) AS outflow,
				` + db.SelectTruncatedTimestamp("block_timestamp", buckets) + ` AS truncated
		FROM gas_events
		WHERE $1 <= block_timestamp AND block_timestamp < $2
		GROUP BY truncated
		ORDER BY truncated ASC
	`

	qargs := []interface{}{buckets.Start().ToNano(), buckets.End().ToNano()}

	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for i := 0; i < buckets.Count(); i++ {
		if rows.Next() {
			var bucket ValueBucket
			var from int64
			err := rows.Scan(&bucket.Value, &from)
			if err != nil {
				return ret, err
			}
			window := buckets.BucketWindow(i)
			if from != window.From.ToI() {
				return ret, errors.New("bucket isn't the same with the query")
			}
			bucket.Window = window
			ret = append(ret, bucket)
		}
	}

	return ret, nil
}
