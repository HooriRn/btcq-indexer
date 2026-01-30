package stat

import (
	"context"
	"errors"

	"gitlab.com/thorchain/midgard/internal/db"
)

// Swaps are generic swap statistics.
type Swaps struct {
	TxCount     int64
	RuneE8Total int64
}

type SwapBucket struct {
	StartTime              db.Second
	EndTime                db.Second
	RuneToAssetCount       int64
	AssetToRuneCount       int64
	RuneToSynthCount       int64
	SynthToRuneCount       int64
	RuneToTradeCount       int64
	TradeToRuneCount       int64
	RuneToSecuredCount     int64
	SecuredToRuneCount     int64
	TotalCount             int64
	RuneToAssetVolume      int64
	AssetToRuneVolume      int64
	RuneToSynthVolume      int64
	SynthToRuneVolume      int64
	RuneToTradeVolume      int64
	TradeToRuneVolume      int64
	SecuredToRuneVolume    int64
	RuneToSecuredVolume    int64
	TotalVolume            int64
	RuneToAssetVolumeUSD   int64
	AssetToRuneVolumeUSD   int64
	RuneToSynthVolumeUSD   int64
	SynthToRuneVolumeUSD   int64
	RuneToTradeVolumeUSD   int64
	TradeToRuneVolumeUSD   int64
	SecuredToRuneVolumeUSD int64
	RuneToSecuredVolumeUSD int64
	TotalVolumeUSD         int64
	RuneToAssetFees        int64
	AssetToRuneFees        int64
	RuneToSynthFees        int64
	SynthToRuneFees        int64
	RuneToTradeFees        int64
	TradeToRuneFees        int64
	SecuredToRuneFees      int64
	RuneToSecuredFees      int64
	TotalFees              int64
	RuneToAssetSlip        int64
	AssetToRuneSlip        int64
	RuneToSynthSlip        int64
	SynthToRuneSlip        int64
	RuneToTradeSlip        int64
	TradeToRuneSlip        int64
	SecuredToRuneSlip      int64
	RuneToSecuredSlip      int64
	TotalSlip              int64
	RunePriceUSD           float64
}

func (sb *SwapBucket) writeOneDirection(oneDirection *OneDirectionSwapBucket) {
	switch oneDirection.Direction {
	case db.RuneToAsset:
		sb.RuneToAssetCount += oneDirection.Count
		sb.RuneToAssetVolume += oneDirection.VolumeInRune
		sb.RuneToAssetVolumeUSD += oneDirection.VolumeInUSD
		sb.RuneToAssetFees += oneDirection.TotalFees
		sb.RuneToAssetSlip += oneDirection.TotalSlip
	case db.AssetToRune:
		sb.AssetToRuneCount += oneDirection.Count
		sb.AssetToRuneVolume += oneDirection.VolumeInRune
		sb.AssetToRuneVolumeUSD += oneDirection.VolumeInUSD
		sb.AssetToRuneFees += oneDirection.TotalFees
		sb.AssetToRuneSlip += oneDirection.TotalSlip
	case db.RuneToSynth:
		sb.RuneToSynthCount += oneDirection.Count
		sb.RuneToSynthVolume += oneDirection.VolumeInRune
		sb.RuneToSynthVolumeUSD += oneDirection.VolumeInUSD
		sb.RuneToSynthFees += oneDirection.TotalFees
		sb.RuneToSynthSlip += oneDirection.TotalSlip
	case db.SynthToRune:
		sb.SynthToRuneCount += oneDirection.Count
		sb.SynthToRuneVolume += oneDirection.VolumeInRune
		sb.SynthToRuneVolumeUSD += oneDirection.VolumeInUSD
		sb.SynthToRuneFees += oneDirection.TotalFees
		sb.SynthToRuneSlip += oneDirection.TotalSlip
	case db.RuneToTrade:
		sb.RuneToTradeCount += oneDirection.Count
		sb.RuneToTradeVolume += oneDirection.VolumeInRune
		sb.RuneToTradeVolumeUSD += oneDirection.VolumeInUSD
		sb.RuneToTradeFees += oneDirection.TotalFees
		sb.RuneToTradeSlip += oneDirection.TotalSlip
	case db.TradeToRune:
		sb.TradeToRuneCount += oneDirection.Count
		sb.TradeToRuneVolume += oneDirection.VolumeInRune
		sb.TradeToRuneVolumeUSD += oneDirection.VolumeInUSD
		sb.TradeToRuneFees += oneDirection.TotalFees
		sb.TradeToRuneSlip += oneDirection.TotalSlip
	case db.RuneToSecure:
		sb.RuneToSecuredCount += oneDirection.Count
		sb.RuneToSecuredVolume += oneDirection.VolumeInRune
		sb.RuneToSecuredVolumeUSD += oneDirection.VolumeInUSD
		sb.RuneToSecuredFees += oneDirection.TotalFees
		sb.RuneToSecuredSlip += oneDirection.TotalSlip
	case db.SecureToRune:
		sb.SecuredToRuneCount += oneDirection.Count
		sb.SecuredToRuneVolume += oneDirection.VolumeInRune
		sb.SecuredToRuneVolumeUSD += oneDirection.VolumeInUSD
		sb.SecuredToRuneFees += oneDirection.TotalFees
		sb.SecuredToRuneSlip += oneDirection.TotalSlip
	}
}

func (sb *SwapBucket) calculateTotals() {
	sb.TotalCount = (sb.RuneToAssetCount + sb.AssetToRuneCount +
		sb.RuneToSynthCount + sb.SynthToRuneCount + sb.TradeToRuneCount +
		sb.RuneToTradeCount + sb.SecuredToRuneCount + sb.RuneToSecuredCount)
	sb.TotalVolume = (sb.RuneToAssetVolume + sb.AssetToRuneVolume +
		sb.RuneToSynthVolume + sb.SynthToRuneVolume + sb.RuneToTradeVolume +
		sb.TradeToRuneVolume + sb.SecuredToRuneVolume + sb.RuneToSecuredVolume)
	sb.TotalVolumeUSD = (sb.RuneToAssetVolumeUSD + sb.AssetToRuneVolumeUSD +
		sb.RuneToSynthVolumeUSD + sb.SynthToRuneVolumeUSD + sb.RuneToTradeVolumeUSD +
		sb.TradeToRuneVolumeUSD + sb.SecuredToRuneVolumeUSD + sb.RuneToSecuredVolumeUSD)
	sb.TotalFees = (sb.RuneToAssetFees + sb.AssetToRuneFees +
		sb.RuneToSynthFees + sb.SynthToRuneFees + sb.RuneToTradeFees + sb.TradeToRuneFees +
		sb.SecuredToRuneFees + sb.RuneToSecuredFees)
	sb.TotalSlip = (sb.RuneToAssetSlip + sb.AssetToRuneSlip +
		sb.RuneToSynthSlip + sb.SynthToRuneSlip + sb.RuneToTradeSlip + sb.TradeToRuneSlip +
		sb.SecuredToRuneSlip + sb.RuneToSecuredSlip)
}

// Used to sum up the buckets in the meta
func (meta *SwapBucket) AddBucket(bucket SwapBucket) {
	meta.RuneToAssetCount += bucket.RuneToAssetCount
	meta.AssetToRuneCount += bucket.AssetToRuneCount
	meta.RuneToSynthCount += bucket.RuneToSynthCount
	meta.SynthToRuneCount += bucket.SynthToRuneCount
	meta.RuneToTradeCount += bucket.RuneToTradeCount
	meta.TradeToRuneCount += bucket.TradeToRuneCount
	meta.SecuredToRuneCount += bucket.SecuredToRuneCount
	meta.RuneToSecuredCount += bucket.RuneToSecuredCount
	meta.TotalCount += bucket.TotalCount
	meta.RuneToAssetVolume += bucket.RuneToAssetVolume
	meta.AssetToRuneVolume += bucket.AssetToRuneVolume
	meta.RuneToSynthVolume += bucket.RuneToSynthVolume
	meta.SynthToRuneVolume += bucket.SynthToRuneVolume
	meta.RuneToTradeVolume += bucket.RuneToTradeVolume
	meta.TradeToRuneVolume += bucket.TradeToRuneVolume
	meta.SecuredToRuneVolume += bucket.SecuredToRuneVolume
	meta.RuneToSecuredVolume += bucket.RuneToSecuredVolume
	meta.TotalVolume += bucket.TotalVolume
	meta.RuneToAssetVolumeUSD += bucket.RuneToAssetVolumeUSD
	meta.AssetToRuneVolumeUSD += bucket.AssetToRuneVolumeUSD
	meta.RuneToSynthVolumeUSD += bucket.RuneToSynthVolumeUSD
	meta.SynthToRuneVolumeUSD += bucket.SynthToRuneVolumeUSD
	meta.RuneToTradeVolumeUSD += bucket.RuneToTradeVolumeUSD
	meta.TradeToRuneVolumeUSD += bucket.TradeToRuneVolumeUSD
	meta.SecuredToRuneVolumeUSD += bucket.SecuredToRuneVolumeUSD
	meta.RuneToSecuredVolumeUSD += bucket.RuneToSecuredVolumeUSD
	meta.TotalVolumeUSD += bucket.TotalVolumeUSD
	meta.RuneToAssetFees += bucket.RuneToAssetFees
	meta.AssetToRuneFees += bucket.AssetToRuneFees
	meta.RuneToSynthFees += bucket.RuneToSynthFees
	meta.SynthToRuneFees += bucket.SynthToRuneFees
	meta.RuneToTradeFees += bucket.RuneToTradeFees
	meta.TradeToRuneFees += bucket.TradeToRuneFees
	meta.SecuredToRuneFees += bucket.SecuredToRuneFees
	meta.RuneToSecuredFees += bucket.RuneToSecuredFees
	meta.TotalFees += bucket.TotalFees
	meta.RuneToAssetSlip += bucket.RuneToAssetSlip
	meta.AssetToRuneSlip += bucket.AssetToRuneSlip
	meta.RuneToSynthSlip += bucket.RuneToSynthSlip
	meta.SynthToRuneSlip += bucket.SynthToRuneSlip
	meta.RuneToTradeSlip += bucket.RuneToTradeSlip
	meta.TradeToRuneSlip += bucket.TradeToRuneSlip
	meta.SecuredToRuneSlip += bucket.SecuredToRuneSlip
	meta.RuneToSecuredSlip += bucket.RuneToSecuredSlip
	meta.TotalSlip += bucket.TotalSlip
}

type OneDirectionSwapBucket struct {
	Time         db.Second
	Count        int64
	VolumeInRune int64
	VolumeInUSD  int64
	TotalFees    int64
	TotalSlip    int64
	Direction    db.SwapDirection
}

var SwapsAggregate = db.RegisterAggregate(db.NewAggregate("swaps", "swap_events").
	AddJoinQuery("rune_price", "r").
	AddGroupColumn("pool").
	AddGroupColumn("_direction").
	AddSumlikeExpression("volume_e8",
		`SUM(CASE
			WHEN _direction%2 = 0 THEN from_e8
			WHEN _direction%2 = 1 THEN to_e8 + liq_fee_in_rune_e8
			ELSE 0 END)::BIGINT`).
	AddSumlikeExpression("volume_usd_e8",
		`SUM(CASE
				WHEN _direction%2 = 0 THEN (from_e8 * r.rune_price_e8) / 1e6
				WHEN _direction%2 = 1 THEN ((to_e8 + liq_fee_in_rune_e8) * r.rune_price_e8) / 1e6
				ELSE 0 END)::BIGINT`).
	AddSumlikeExpression("swap_count", "COUNT(1)").
	// On swapping from asset to rune fees are collected in rune.
	AddSumlikeExpression("rune_fees_e8",
		"SUM(CASE WHEN _direction%2 = 1 THEN liq_fee_e8 ELSE 0 END)::BIGINT").
	// On swapping from rune to asset fees are collected in asset.
	AddSumlikeExpression("asset_fees_e8",
		"SUM(CASE WHEN _direction%2 = 0 THEN liq_fee_e8 ELSE 0 END)::BIGINT").
	AddBigintSumColumn("liq_fee_in_rune_e8").
	AddBigintSumColumn("swap_slip_bp"))

// Returns sparse buckets, when there are no swaps in the bucket, the bucket is missing.
// Returns several results for a given for all directions where a swap is present.
func GetSwapBuckets(ctx context.Context, pool *string, buckets db.Buckets) (
	[]OneDirectionSwapBucket, error) {

	filters := []string{}
	params := []interface{}{}
	if pool != nil {
		filters = append(filters, "pool = $1")
		params = append(params, *pool)
	}
	q, params := SwapsAggregate.BucketedQuery(`
		SELECT
			aggregate_timestamp/1000000000 as time,
			_direction,
			SUM(swap_count) AS count,
			SUM(volume_e8) AS volume,
			SUM(volume_usd_e8) AS volume_usd,
			SUM(liq_fee_in_rune_e8) AS fee,
			SUM(swap_slip_bp) AS slip
		FROM %s
		GROUP BY _direction, time
		ORDER BY time ASC
	`, buckets, filters, params)

	rows, err := db.Query(ctx, q, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ret := []OneDirectionSwapBucket{}
	for rows.Next() {
		var bucket OneDirectionSwapBucket
		err := rows.Scan(&bucket.Time, &bucket.Direction, &bucket.Count, &bucket.VolumeInRune, &bucket.VolumeInUSD, &bucket.TotalFees, &bucket.TotalSlip)
		if err != nil {
			return []OneDirectionSwapBucket{}, err
		}
		ret = append(ret, bucket)
	}
	return ret, rows.Err()
}

// Does not fill USD field of the SwapBucket
// If pool is nil, returns global
func GetOneIntervalSwapsNoUSD(
	ctx context.Context, pool *string, buckets db.Buckets) (
	*SwapBucket, error) {

	if !buckets.OneInterval() {
		return nil, errors.New("Single interval buckets expected for swapsNoUSD")
	}
	swaps, err := GetSwapBuckets(ctx, pool, buckets)
	if err != nil {
		return nil, err
	}

	ret := SwapBucket{
		StartTime: buckets.Start(),
		EndTime:   buckets.End(),
	}
	for _, swap := range swaps {
		if swap.Time != buckets.Start() {
			return nil, errors.New("Bad returned timestamp while reading swap stats")
		}
		ret.writeOneDirection(&swap)
	}
	ret.calculateTotals()
	return &ret, nil
}

// Returns gapfilled PoolSwaps for given pool, window and interval
func GetPoolSwaps(ctx context.Context, pool *string, buckets db.Buckets) ([]SwapBucket, error) {
	swaps, err := GetSwapBuckets(ctx, pool, buckets)
	if err != nil {
		return nil, err
	}
	usdPrice, err := USDPriceHistory(ctx, buckets)
	if err != nil {
		return nil, err
	}

	return mergeSwapsGapfill(swaps, usdPrice), nil
}

func mergeSwapsGapfill(swaps []OneDirectionSwapBucket,
	denseUSDPrices []USDPriceBucket) []SwapBucket {
	ret := make([]SwapBucket, len(denseUSDPrices))

	timeAfterLast := denseUSDPrices[len(denseUSDPrices)-1].Window.Until + 1
	swaps = append(swaps, OneDirectionSwapBucket{Time: timeAfterLast})

	idx := 0
	for i, usdPrice := range denseUSDPrices {
		current := &ret[i]
		current.StartTime = usdPrice.Window.From
		current.EndTime = usdPrice.Window.Until
		for swaps[idx].Time == current.StartTime {
			swap := &swaps[idx]
			current.writeOneDirection(swap)
			idx++
		}

		current.calculateTotals()
		current.RunePriceUSD = usdPrice.RunePriceUSD
	}

	return ret
}

func addVolumes(
	ctx context.Context,
	pools []string,
	w db.Window,
	poolVolumes *map[string]int64) error {

	bucket := db.OneIntervalBuckets(w.From, w.Until)
	wheres := []string{
		"pool = ANY($1)",
	}
	params := []interface{}{pools}

	q, params := SwapsAggregate.BucketedQuery(`
	SELECT
		pool,
		volume_e8 AS volume
	FROM %s
	`, bucket, wheres, params)

	swapRows, err := db.Query(ctx, q, params...)
	if err != nil {
		return err
	}
	defer swapRows.Close()

	for swapRows.Next() {
		var pool string
		var volume int64
		err := swapRows.Scan(&pool, &volume)
		if err != nil {
			return err
		}
		(*poolVolumes)[pool] += volume
	}
	return nil
}

// PoolsTotalVolume computes total volume amount for given timestamps (from/to) and pools
func PoolsTotalVolume(ctx context.Context, pools []string, w db.Window) (map[string]int64, error) {
	poolVolumes := make(map[string]int64)
	err := addVolumes(ctx, pools, w, &poolVolumes)
	if err != nil {
		return nil, err
	}

	return poolVolumes, nil
}

type CountVolume = struct {
	Count  int64
	Volume int64
}
type SwapStats map[db.SwapDirection]CountVolume

func (s SwapStats) Totals() (total CountVolume) {
	for _, cv := range s {
		total.Count += cv.Count
		total.Volume += cv.Volume
	}
	return
}

func GlobalSwapStats(ctx context.Context, aggregate string, start db.Second) (SwapStats, error) {
	q := `
		SELECT
			_direction,
			SUM(volume_e8),
			SUM(swap_count)
		FROM midgard_agg.swaps_` + aggregate + `
		WHERE aggregate_timestamp >= $1
		GROUP BY _direction`

	rows, err := db.Query(ctx, q, start.ToNano().ToI())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make(SwapStats)
	for rows.Next() {
		var dir db.SwapDirection
		var volume, count int64
		err = rows.Scan(&dir, &volume, &count)
		if err != nil {
			return nil, err
		}
		res[dir] = CountVolume{count, volume}
	}

	return res, nil
}
