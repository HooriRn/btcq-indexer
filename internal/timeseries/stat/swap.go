package stat

import (
	"context"
	"errors"

	"github.com/btcq/btcq-indexer/internal/db"
)

// Swaps are generic swap statistics.
type Swaps struct {
	TxCount     int64
	QbtcE8Total int64
}

type SwapBucket struct {
	StartTime              db.Second
	EndTime                db.Second
	QbtcToAssetCount       int64
	AssetToQbtcCount       int64
	QbtcToSynthCount       int64
	SynthToQbtcCount       int64
	QbtcToTradeCount       int64
	TradeToQbtcCount       int64
	QbtcToSecuredCount     int64
	SecuredToQbtcCount     int64
	TotalCount             int64
	QbtcToAssetVolume      int64
	AssetToQbtcVolume      int64
	QbtcToSynthVolume      int64
	SynthToQbtcVolume      int64
	QbtcToTradeVolume      int64
	TradeToQbtcVolume      int64
	SecuredToQbtcVolume    int64
	QbtcToSecuredVolume    int64
	TotalVolume            int64
	QbtcToAssetVolumeUSD   int64
	AssetToQbtcVolumeUSD   int64
	QbtcToSynthVolumeUSD   int64
	SynthToQbtcVolumeUSD   int64
	QbtcToTradeVolumeUSD   int64
	TradeToQbtcVolumeUSD   int64
	SecuredToQbtcVolumeUSD int64
	QbtcToSecuredVolumeUSD int64
	TotalVolumeUSD         int64
	QbtcToAssetFees        int64
	AssetToQbtcFees        int64
	QbtcToSynthFees        int64
	SynthToQbtcFees        int64
	QbtcToTradeFees        int64
	TradeToQbtcFees        int64
	SecuredToQbtcFees      int64
	QbtcToSecuredFees      int64
	TotalFees              int64
	QbtcToAssetSlip        int64
	AssetToQbtcSlip        int64
	QbtcToSynthSlip        int64
	SynthToQbtcSlip        int64
	QbtcToTradeSlip        int64
	TradeToQbtcSlip        int64
	SecuredToQbtcSlip      int64
	QbtcToSecuredSlip      int64
	TotalSlip              int64
	QbtcPriceUSD           float64
}

func (sb *SwapBucket) writeOneDirection(oneDirection *OneDirectionSwapBucket) {
	switch oneDirection.Direction {
	case db.RuneToAsset:
		sb.QbtcToAssetCount += oneDirection.Count
		sb.QbtcToAssetVolume += oneDirection.VolumeInQbtc
		sb.QbtcToAssetVolumeUSD += oneDirection.VolumeInUSD
		sb.QbtcToAssetFees += oneDirection.TotalFees
		sb.QbtcToAssetSlip += oneDirection.TotalSlip
	case db.AssetToRune:
		sb.AssetToQbtcCount += oneDirection.Count
		sb.AssetToQbtcVolume += oneDirection.VolumeInQbtc
		sb.AssetToQbtcVolumeUSD += oneDirection.VolumeInUSD
		sb.AssetToQbtcFees += oneDirection.TotalFees
		sb.AssetToQbtcSlip += oneDirection.TotalSlip
	case db.RuneToSynth:
		sb.QbtcToSynthCount += oneDirection.Count
		sb.QbtcToSynthVolume += oneDirection.VolumeInQbtc
		sb.QbtcToSynthVolumeUSD += oneDirection.VolumeInUSD
		sb.QbtcToSynthFees += oneDirection.TotalFees
		sb.QbtcToSynthSlip += oneDirection.TotalSlip
	case db.SynthToRune:
		sb.SynthToQbtcCount += oneDirection.Count
		sb.SynthToQbtcVolume += oneDirection.VolumeInQbtc
		sb.SynthToQbtcVolumeUSD += oneDirection.VolumeInUSD
		sb.SynthToQbtcFees += oneDirection.TotalFees
		sb.SynthToQbtcSlip += oneDirection.TotalSlip
	case db.RuneToTrade:
		sb.QbtcToTradeCount += oneDirection.Count
		sb.QbtcToTradeVolume += oneDirection.VolumeInQbtc
		sb.QbtcToTradeVolumeUSD += oneDirection.VolumeInUSD
		sb.QbtcToTradeFees += oneDirection.TotalFees
		sb.QbtcToTradeSlip += oneDirection.TotalSlip
	case db.TradeToRune:
		sb.TradeToQbtcCount += oneDirection.Count
		sb.TradeToQbtcVolume += oneDirection.VolumeInQbtc
		sb.TradeToQbtcVolumeUSD += oneDirection.VolumeInUSD
		sb.TradeToQbtcFees += oneDirection.TotalFees
		sb.TradeToQbtcSlip += oneDirection.TotalSlip
	case db.RuneToSecure:
		sb.QbtcToSecuredCount += oneDirection.Count
		sb.QbtcToSecuredVolume += oneDirection.VolumeInQbtc
		sb.QbtcToSecuredVolumeUSD += oneDirection.VolumeInUSD
		sb.QbtcToSecuredFees += oneDirection.TotalFees
		sb.QbtcToSecuredSlip += oneDirection.TotalSlip
	case db.SecureToRune:
		sb.SecuredToQbtcCount += oneDirection.Count
		sb.SecuredToQbtcVolume += oneDirection.VolumeInQbtc
		sb.SecuredToQbtcVolumeUSD += oneDirection.VolumeInUSD
		sb.SecuredToQbtcFees += oneDirection.TotalFees
		sb.SecuredToQbtcSlip += oneDirection.TotalSlip
	}
}

func (sb *SwapBucket) calculateTotals() {
	sb.TotalCount = (sb.QbtcToAssetCount + sb.AssetToQbtcCount +
		sb.QbtcToSynthCount + sb.SynthToQbtcCount + sb.TradeToQbtcCount +
		sb.QbtcToTradeCount + sb.SecuredToQbtcCount + sb.QbtcToSecuredCount)
	sb.TotalVolume = (sb.QbtcToAssetVolume + sb.AssetToQbtcVolume +
		sb.QbtcToSynthVolume + sb.SynthToQbtcVolume + sb.QbtcToTradeVolume +
		sb.TradeToQbtcVolume + sb.SecuredToQbtcVolume + sb.QbtcToSecuredVolume)
	sb.TotalVolumeUSD = (sb.QbtcToAssetVolumeUSD + sb.AssetToQbtcVolumeUSD +
		sb.QbtcToSynthVolumeUSD + sb.SynthToQbtcVolumeUSD + sb.QbtcToTradeVolumeUSD +
		sb.TradeToQbtcVolumeUSD + sb.SecuredToQbtcVolumeUSD + sb.QbtcToSecuredVolumeUSD)
	sb.TotalFees = (sb.QbtcToAssetFees + sb.AssetToQbtcFees +
		sb.QbtcToSynthFees + sb.SynthToQbtcFees + sb.QbtcToTradeFees + sb.TradeToQbtcFees +
		sb.SecuredToQbtcFees + sb.QbtcToSecuredFees)
	sb.TotalSlip = (sb.QbtcToAssetSlip + sb.AssetToQbtcSlip +
		sb.QbtcToSynthSlip + sb.SynthToQbtcSlip + sb.QbtcToTradeSlip + sb.TradeToQbtcSlip +
		sb.SecuredToQbtcSlip + sb.QbtcToSecuredSlip)
}

// Used to sum up the buckets in the meta
func (meta *SwapBucket) AddBucket(bucket SwapBucket) {
	meta.QbtcToAssetCount += bucket.QbtcToAssetCount
	meta.AssetToQbtcCount += bucket.AssetToQbtcCount
	meta.QbtcToSynthCount += bucket.QbtcToSynthCount
	meta.SynthToQbtcCount += bucket.SynthToQbtcCount
	meta.QbtcToTradeCount += bucket.QbtcToTradeCount
	meta.TradeToQbtcCount += bucket.TradeToQbtcCount
	meta.SecuredToQbtcCount += bucket.SecuredToQbtcCount
	meta.QbtcToSecuredCount += bucket.QbtcToSecuredCount
	meta.TotalCount += bucket.TotalCount
	meta.QbtcToAssetVolume += bucket.QbtcToAssetVolume
	meta.AssetToQbtcVolume += bucket.AssetToQbtcVolume
	meta.QbtcToSynthVolume += bucket.QbtcToSynthVolume
	meta.SynthToQbtcVolume += bucket.SynthToQbtcVolume
	meta.QbtcToTradeVolume += bucket.QbtcToTradeVolume
	meta.TradeToQbtcVolume += bucket.TradeToQbtcVolume
	meta.SecuredToQbtcVolume += bucket.SecuredToQbtcVolume
	meta.QbtcToSecuredVolume += bucket.QbtcToSecuredVolume
	meta.TotalVolume += bucket.TotalVolume
	meta.QbtcToAssetVolumeUSD += bucket.QbtcToAssetVolumeUSD
	meta.AssetToQbtcVolumeUSD += bucket.AssetToQbtcVolumeUSD
	meta.QbtcToSynthVolumeUSD += bucket.QbtcToSynthVolumeUSD
	meta.SynthToQbtcVolumeUSD += bucket.SynthToQbtcVolumeUSD
	meta.QbtcToTradeVolumeUSD += bucket.QbtcToTradeVolumeUSD
	meta.TradeToQbtcVolumeUSD += bucket.TradeToQbtcVolumeUSD
	meta.SecuredToQbtcVolumeUSD += bucket.SecuredToQbtcVolumeUSD
	meta.QbtcToSecuredVolumeUSD += bucket.QbtcToSecuredVolumeUSD
	meta.TotalVolumeUSD += bucket.TotalVolumeUSD
	meta.QbtcToAssetFees += bucket.QbtcToAssetFees
	meta.AssetToQbtcFees += bucket.AssetToQbtcFees
	meta.QbtcToSynthFees += bucket.QbtcToSynthFees
	meta.SynthToQbtcFees += bucket.SynthToQbtcFees
	meta.QbtcToTradeFees += bucket.QbtcToTradeFees
	meta.TradeToQbtcFees += bucket.TradeToQbtcFees
	meta.SecuredToQbtcFees += bucket.SecuredToQbtcFees
	meta.QbtcToSecuredFees += bucket.QbtcToSecuredFees
	meta.TotalFees += bucket.TotalFees
	meta.QbtcToAssetSlip += bucket.QbtcToAssetSlip
	meta.AssetToQbtcSlip += bucket.AssetToQbtcSlip
	meta.QbtcToSynthSlip += bucket.QbtcToSynthSlip
	meta.SynthToQbtcSlip += bucket.SynthToQbtcSlip
	meta.QbtcToTradeSlip += bucket.QbtcToTradeSlip
	meta.TradeToQbtcSlip += bucket.TradeToQbtcSlip
	meta.SecuredToQbtcSlip += bucket.SecuredToQbtcSlip
	meta.QbtcToSecuredSlip += bucket.QbtcToSecuredSlip
	meta.TotalSlip += bucket.TotalSlip
}

type OneDirectionSwapBucket struct {
	Time         db.Second
	Count        int64
	VolumeInQbtc int64
	VolumeInUSD  int64
	TotalFees    int64
	TotalSlip    int64
	Direction    db.SwapDirection
}

var SwapsAggregate = db.RegisterAggregate(db.NewAggregate("swaps", "swap_events").
	AddJoinQuery("qbtc_price", "r").
	AddGroupColumn("pool").
	AddGroupColumn("_direction").
	AddSumlikeExpression("volume_e8",
		`SUM(CASE
			WHEN _direction%2 = 0 THEN from_e8
			WHEN _direction%2 = 1 THEN to_e8 + liq_fee_in_qbtc_e8
			ELSE 0 END)::BIGINT`).
	AddSumlikeExpression("volume_usd_e8",
		`SUM(CASE
				WHEN _direction%2 = 0 THEN (from_e8 * r.qbtc_price_e8) / 1e6
				WHEN _direction%2 = 1 THEN ((to_e8 + liq_fee_in_qbtc_e8) * r.qbtc_price_e8) / 1e6
				ELSE 0 END)::BIGINT`).
	AddSumlikeExpression("swap_count", "COUNT(1)").
	// On swapping from asset to qbtc fees are collected in qbtc.
	AddSumlikeExpression("qbtc_fees_e8",
		"SUM(CASE WHEN _direction%2 = 1 THEN liq_fee_e8 ELSE 0 END)::BIGINT").
	// On swapping from qbtc to asset fees are collected in asset.
	AddSumlikeExpression("asset_fees_e8",
		"SUM(CASE WHEN _direction%2 = 0 THEN liq_fee_e8 ELSE 0 END)::BIGINT").
	AddBigintSumColumn("liq_fee_in_qbtc_e8").
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
			SUM(liq_fee_in_qbtc_e8) AS fee,
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
		err := rows.Scan(&bucket.Time, &bucket.Direction, &bucket.Count, &bucket.VolumeInQbtc, &bucket.VolumeInUSD, &bucket.TotalFees, &bucket.TotalSlip)
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
		current.QbtcPriceUSD = usdPrice.QbtcPriceUSD
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
		FROM btcq_indexer_agg.swaps_` + aggregate + `
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
