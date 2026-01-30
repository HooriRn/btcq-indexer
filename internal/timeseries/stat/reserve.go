package stat

import (
	"context"
	"errors"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/notinchain"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

type ValueBucket struct {
	Window db.Window
	Value  int64
}

func GetReserveHistory(ctx context.Context, buckets db.Buckets) (oapigen.ReserveHistoryResponse, error) {
	fees, err := bucketedFeeStat(ctx, buckets)
	if err != nil {
		return oapigen.ReserveHistoryResponse{}, err
	}

	gases, err := bucketedGasStat(ctx, buckets)
	if err != nil {
		return oapigen.ReserveHistoryResponse{}, err
	}

	networkFees, err := bucketedNetworkFeeStat(ctx, buckets)
	if err != nil {
		return oapigen.ReserveHistoryResponse{}, err
	}

	intervals := oapigen.ReserveIntervals{}
	var inflow int64
	var outflow int64
	var netFee int64
	for i, fee := range fees {
		interval := oapigen.ReserveItem{
			StartTime:        util.IntStr(fee.Window.From.ToI()),
			EndTime:          util.IntStr(fee.Window.Until.ToI()),
			GasFeeOutbound:   util.IntStr(fee.Value),
			GasReimbursement: util.IntStr(gases[i].Value),
			NetworkFee:       util.IntStr(networkFees[i].Value),
		}
		intervals = append(intervals, interval)

		// accumulate meta
		inflow += fee.Value
		outflow += gases[i].Value
		netFee += networkFees[i].Value
	}

	ret := oapigen.ReserveHistoryResponse{
		Meta: oapigen.ReserveMeta{
			StartTime:        util.IntStr(buckets.Start().ToI()),
			EndTime:          util.IntStr(buckets.End().ToI()),
			NetworkFee:       util.IntStr(netFee),
			GasFeeOutbound:   util.IntStr(inflow),
			GasReimbursement: util.IntStr(outflow),
		},
		Intervals: intervals,
	}

	return ret, nil
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
			COALESCE(SUM(rune_e8), 0) AS outflow,
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
