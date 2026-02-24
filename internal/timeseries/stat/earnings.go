package stat

import (
	"context"

	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/timeseries"
)

type PoolEarnings struct {
	Pool                   string
	QbtcLiquidityFees      int64 // fees charged in QBTC
	AssetLiquidityFees     int64 // fees charged in asset
	TotalLiquidityFeesQbtc int64 // asset + QBTC fees in QBTC
	Rewards                int64 // rewards sent to / extracted from pool each block
}

type poolEarningsMap map[string]*PoolEarnings

func (peMap poolEarningsMap) getPoolEarnings(pool string) *PoolEarnings {
	pe := peMap[pool]
	if pe == nil {
		newPoolEarnings := PoolEarnings{Pool: pool}
		// Nil map means there were no entries for the bucket at all
		if peMap != nil {
			peMap[pool] = &newPoolEarnings
		}
		return &newPoolEarnings
	} else {
		return pe
	}
}

var RewardsAggregate = db.RegisterAggregate(db.NewAggregate("rewards_events", "rewards_events").
	AddBigintSumColumn("bond_e8"))

// TODO (HooriRn): Separate pool and rewards query from EarningsHistory
// TODO (HooriRn): add pool specific query
func GetPoolsEarnings(ctx context.Context, buckets db.Buckets) (poolEarningsMap, error) {
	// Pools rewards
	poolRewardsQ, params := timeseries.RewardEntriesAggregate.BucketedQuery(`
		SELECT
			qbtc_e8,
			aggregate_timestamp/1000000000 AS start_time,
			pool
		FROM %s
	`, buckets, nil, nil)

	poolRewardsRows, err := db.Query(ctx, poolRewardsQ, params...)
	if err != nil {
		return nil, err
	}
	defer poolRewardsRows.Close()

	// Pool liquidity fees
	// liquidityFeesByPoolQ, params := SwapsAggregate.BucketedQuery(`
	// 	SELECT
	// 		rune_fees_E8,
	// 		asset_fees_E8,
	// 		liq_fee_in_rune_E8,
	// 		aggregate_timestamp/1000000000 AS start_time,
	// 		pool
	// 	FROM %s
	// `, buckets, nil, nil)
	//
	// liquidityFeesByPoolRows, err := db.Query(ctx, liquidityFeesByPoolQ, params...)
	// if err != nil {
	// 	return nil, err
	// }
	// defer liquidityFeesByPoolRows.Close()

	mapPoolEarningStat := make(poolEarningsMap)

	// TODO (HooriRn): check if we also calculate the synth pool as earning or Reward entries already
	// does it.
	// for liquidityFeesByPoolRows.Next() {
	// 	var qbtcLiquidityFees, assetLiquidityFees, totalLiquidityFeesQbtc int64
	// 	var startTime db.Second
	// 	var pool string
	// 	err := liquidityFeesByPoolRows.Scan(
	// 		&qbtcLiquidityFees,
	// 		&assetLiquidityFees,
	// 		&totalLiquidityFeesQbtc,
	// 		&startTime,
	// 		&pool)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	//
	// 	// Add fees to earnings by pool
	// 	metaPoolEarnings := mapPoolEarningStat.getPoolEarnings(pool)
	// 	metaPoolEarnings.QbtcLiquidityFees += qbtcLiquidityFees
	// 	metaPoolEarnings.AssetLiquidityFees += assetLiquidityFees
	// 	metaPoolEarnings.TotalLiquidityFeesQbtc += totalLiquidityFeesQbtc
	// }

	for poolRewardsRows.Next() {
		var runeE8 int64
		var startTime db.Second
		var pool string
		err := poolRewardsRows.Scan(&runeE8, &startTime, &pool)
		if err != nil {
			return nil, err
		}

		// Add rewards to earnings by pool
		mapPoolEarningStat.getPoolEarnings(pool).Rewards += runeE8
	}

	return mapPoolEarningStat, nil
}
