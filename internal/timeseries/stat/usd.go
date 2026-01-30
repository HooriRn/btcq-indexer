package stat

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func runePriceUSDForDepths(depths timeseries.DepthMap) float64 {
	ret := math.NaN()

	runePricesInUsd := []float64{}
	for _, pool := range config.Global.UsdPools {
		poolInfo, ok := depths[pool]
		if ok && poolInfo.AssetDepth > 0 && poolInfo.RuneDepth > 0 {
			runePricesInUsd = append(runePricesInUsd, 1/poolInfo.AssetPrice())
		}
	}

	if len(runePricesInUsd) > 0 {
		ret = util.GetMedian(runePricesInUsd)
	}

	return ret
}

// Returns median of the whitelisted usd pools.
func RunePriceUSD() float64 {
	return runePriceUSDForDepths(timeseries.Latest.GetState().Pools)
}

func ServeUSDDebug(resp http.ResponseWriter, req *http.Request) {
	state := timeseries.Latest.GetState()
	for _, pool := range config.Global.UsdPools {
		poolInfo := state.PoolInfo(pool)
		if poolInfo == nil {
			fmt.Fprintf(resp, "%s - pool not found\n", pool)
		} else {
			depth := float64(poolInfo.RuneDepth) / 1e8
			runePrice := 1 / poolInfo.AssetPrice()
			fmt.Fprintf(resp, "%s - runeDepth: %.0f runePriceUsd: %.2f\n", pool, depth, runePrice)
		}
	}

	fmt.Fprintf(resp, "\n\nrunePriceUSD: %v", RunePriceUSD())
}

func GetRunePriceHistory(ctx context.Context, buckets db.Buckets) (oapigen.RunePriceHistory, error) {
	usdPrices, err := USDPriceHistory(ctx, buckets)
	if err != nil {
		return oapigen.RunePriceHistory{}, err
	}

	intervals := oapigen.RunePriceIntervals{}
	for _, usd := range usdPrices {
		interval := oapigen.RunePriceItem{
			StartTime:    util.IntStr(usd.Window.From.ToI()),
			EndTime:      util.IntStr(usd.Window.Until.ToI()),
			RunePriceUSD: util.FloatStr(usd.RunePriceUSD),
		}
		intervals = append(intervals, interval)
	}

	ret := oapigen.RunePriceHistory{
		Meta: oapigen.RunePriceMeta{
			StartTime:         util.IntStr(buckets.Start().ToI()),
			EndTime:           util.IntStr(buckets.End().ToI()),
			StartRunePriceUSD: util.FloatStr(usdPrices[0].RunePriceUSD),
			EndRunePriceUSD:   util.FloatStr(usdPrices[len(usdPrices)-1].RunePriceUSD),
		},
		Intervals: intervals,
	}

	return ret, nil
}
