package stat

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/timeseries"
	"github.com/btcq/btcq-indexer/internal/util"
	"github.com/btcq/btcq-indexer/openapi/generated/oapigen"
)

func qbtcPriceUSDForDepths(depths timeseries.DepthMap) float64 {
	ret := math.NaN()

	qbtcPricesInUsd := []float64{}
	for _, pool := range config.Global.UsdPools {
		poolInfo, ok := depths[pool]
		if ok && poolInfo.AssetDepth > 0 && poolInfo.QbtcDepth > 0 {
			qbtcPricesInUsd = append(qbtcPricesInUsd, 1/poolInfo.AssetPrice())
		}
	}

	if len(qbtcPricesInUsd) > 0 {
		ret = util.GetMedian(qbtcPricesInUsd)
	}

	return ret
}

// Returns median of the whitelisted usd pools.
func QbtcPriceUSD() float64 {
	return qbtcPriceUSDForDepths(timeseries.Latest.GetState().Pools)
}

func ServeUSDDebug(resp http.ResponseWriter, req *http.Request) {
	state := timeseries.Latest.GetState()
	for _, pool := range config.Global.UsdPools {
		poolInfo := state.PoolInfo(pool)
		if poolInfo == nil {
			fmt.Fprintf(resp, "%s - pool not found\n", pool)
		} else {
			depth := float64(poolInfo.QbtcDepth) / 1e8
			qbtcPrice := 1 / poolInfo.AssetPrice()
			fmt.Fprintf(resp, "%s - qbtcDepth: %.0f qbtcPriceUsd: %.2f\n", pool, depth, qbtcPrice)
		}
	}

	fmt.Fprintf(resp, "\n\nqbtcPriceUSD: %v", QbtcPriceUSD())
}

func GetQbtcPriceHistory(ctx context.Context, buckets db.Buckets) (oapigen.QbtcPriceHistory, error) {
	usdPrices, err := USDPriceHistory(ctx, buckets)
	if err != nil {
		return oapigen.QbtcPriceHistory{}, err
	}

	intervals := oapigen.QbtcPriceIntervals{}
	for _, usd := range usdPrices {
		interval := oapigen.QbtcPriceItem{
			StartTime:    util.IntStr(usd.Window.From.ToI()),
			EndTime:      util.IntStr(usd.Window.Until.ToI()),
			QbtcPriceUSD: util.FloatStr(usd.QbtcPriceUSD),
		}
		intervals = append(intervals, interval)
	}

	ret := oapigen.QbtcPriceHistory{
		Meta: oapigen.QbtcPriceMeta{
			StartTime:         util.IntStr(buckets.Start().ToI()),
			EndTime:           util.IntStr(buckets.End().ToI()),
			StartQbtcPriceUSD: util.FloatStr(usdPrices[0].QbtcPriceUSD),
			EndQbtcPriceUSD:   util.FloatStr(usdPrices[len(usdPrices)-1].QbtcPriceUSD),
		},
		Intervals: intervals,
	}

	return ret, nil
}
