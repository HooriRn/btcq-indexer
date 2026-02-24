package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/decimal"
	"github.com/btcq/btcq-indexer/internal/fetch/record"
	"github.com/btcq/btcq-indexer/internal/util"
	"github.com/btcq/btcq-indexer/internal/util/btcqerr"
	"github.com/julienschmidt/httprouter"

	"github.com/btcq/btcq-indexer/internal/timeseries"
	"github.com/btcq/btcq-indexer/internal/timeseries/stat"
	"github.com/btcq/btcq-indexer/openapi/generated/oapigen"
)

func jsonHealth(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	merr := util.CheckUrlEmpty(r.URL.Query())
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	height, _, _ := timeseries.LastBlock()
	synced := db.FullyCaughtUp()
	var genesisInfo *oapigen.GenesisInf = nil
	if genInfo := db.GenesisInfo.Get(); genInfo.Get().Height > 0 {
		genesisInfo = &oapigen.GenesisInf{
			Height: int(genInfo.Get().Height),
			Hash:   genInfo.Get().Hash,
		}
	}

	respJSON(w, oapigen.HealthResponse{
		InSync:         synced,
		Database:       true,
		ScannerHeight:  util.IntStr(height + 1),
		LastThorNode:   db.LastThorNodeBlock.AsHeightTS(),
		LastFetched:    db.LastFetchedBlock.AsHeightTS(),
		LastCommitted:  db.LastCommittedBlock.AsHeightTS(),
		LastAggregated: db.LastAggregatedBlock.AsHeightTS(),
		GenesisInfo:    genesisInfo,
	})
}

func luvi(assetE8 int64, qbtcE8 int64, poolUnits int64) float64 {
	if poolUnits <= 0 {
		return math.NaN()
	}
	return math.Sqrt(float64(assetE8)*float64(qbtcE8)) / float64(poolUnits)
}

func luviFromLPUnits(depths timeseries.PoolDepths, lpUnits int64) float64 {
	synthUnits := timeseries.CalculateSynthUnits(depths.AssetDepth, depths.SynthDepth, lpUnits)
	return luvi(depths.AssetDepth, depths.QbtcDepth, lpUnits+synthUnits)
}

// Filters out Suspended pools.
// If there is a status url parameter then returns pools with that status only.
func poolsWithRequestedStatus(
	ctx context.Context, urlParams *url.Values, statusMap map[string]string) (
	[]string, error) {

	pools, err := timeseries.PoolsWithDeposit(ctx)
	if err != nil {
		return nil, err
	}
	requestedStatus := util.ConsumeUrlParam(urlParams, "status")
	if requestedStatus != "" {
		const errormsg = "Max one status parameter, accepted values: available, staged, suspended"
		requestedStatus = strings.ToLower(requestedStatus)
		// Allowed statuses in
		// https://gitlab.com/thorchain/thornode/-/blob/master/x/thorchain/types/type_pool.go
		if requestedStatus != "available" && requestedStatus != "staged" && requestedStatus != "suspended" {
			return nil, fmt.Errorf(errormsg)
		}
	}
	ret := []string{}
	for _, pool := range pools {
		poolStatus := poolStatusFromMap(pool, statusMap)
		if poolStatus != "suspended" && (requestedStatus == "" || poolStatus == requestedStatus) {
			ret = append(ret, pool)
		}
	}
	return ret, nil
}

func GetPoolAPRs(ctx context.Context,
	depthsNow timeseries.DepthMap, lpUnitsNow map[string]int64, pools []string,
	aprStartTime db.Nano, now db.Nano) (
	map[string]float64, error) {

	var periodsPerYear float64 = 365 * 24 * 60 * 60 * 1e9 / float64(now-aprStartTime)
	liquidityUnitsBefore, err := stat.PoolsLiquidityUnitsBefore(ctx, pools, &aprStartTime)
	if err != nil {
		return nil, err
	}
	depthsBefore, err := stat.DepthsBefore(ctx, pools, aprStartTime)
	if err != nil {
		return nil, err
	}

	ret := map[string]float64{}
	for _, pool := range pools {
		if record.GetCoinType([]byte(pool)) == record.AssetNative {
			luviNow := luviFromLPUnits(depthsNow[pool], lpUnitsNow[pool])
			luviBefore := luviFromLPUnits(depthsBefore[pool], liquidityUnitsBefore[pool])
			luviIncrease := luviNow / luviBefore
			ret[pool] = math.Pow(luviIncrease, periodsPerYear) - 1
		} else {
			_, unitNowOk := lpUnitsNow[pool]
			_, unitBeforeOk := liquidityUnitsBefore[pool]
			if !unitBeforeOk || !unitNowOk {
				ret[pool] = 0.00
				continue
			}
			swapGrowthIndexBefore := float64(depthsBefore[pool].AssetDepth) / float64(liquidityUnitsBefore[pool])
			swapGrowthIndexNow := float64(depthsNow[pool].AssetDepth) / float64(lpUnitsNow[pool])
			ret[pool] = ((swapGrowthIndexNow - swapGrowthIndexBefore) / swapGrowthIndexBefore) * periodsPerYear
		}
	}
	return ret, nil
}

func GetSinglePoolAPR(ctx context.Context,
	depths timeseries.PoolDepths, lpUnits int64, pool string, start db.Nano, now db.Nano) (
	float64, error) {
	aprs, err := GetPoolAPRs(
		ctx,
		timeseries.DepthMap{pool: depths},
		map[string]int64{pool: lpUnits},
		[]string{pool},
		start, now)
	if err != nil {
		return 0, err
	}
	return aprs[pool], nil
}

type EarningsInfo struct {
	Earnings                   int64
	AnnualEarningsAsPercentage float64
}

type poolAggregates struct {
	depths          timeseries.DepthMap
	dailyVolumes    map[string]int64
	liquidityUnits  map[string]int64
	earningsDataMap map[string]EarningsInfo
}

func getPoolAggregates(ctx context.Context, pools []string, apyBucket db.Buckets) (
	*poolAggregates, error) {

	latestState := timeseries.Latest.GetState()
	// now := latestState.NextSecond()
	// window24h := db.Window{From: now - 24*60*60, Until: now}
	//
	// dailyVolumes, err := stat.PoolsTotalVolume(ctx, pools, window24h)
	// if err != nil {
	// 	return nil, err
	// }
	dailyVolumes := make(map[string]int64) // Return empty map

	// this adds pool synth for pool endpoint
	if len(pools) == 1 {
		synthPool := strings.Replace(pools[0], ".", "/", 1)
		_, ok := latestState.Pools[synthPool]
		if ok {
			pools = append(pools, synthPool)
		}
	}

	liquidityUnitsNow, err := stat.PoolsLiquidityUnitsBefore(ctx, pools, nil)
	if err != nil {
		return nil, err
	}

	// Get earnings data for the pools
	poolEarningsMapStat, err := stat.GetPoolsEarnings(ctx, apyBucket)
	if err != nil {
		return nil, err
	}

	mapEarningsInfo := make(map[string]EarningsInfo)
	periodsPerYear := db.GetPPYFromBuckets(apyBucket)
	for pool, earnings := range poolEarningsMapStat {
		er := earnings.TotalLiquidityFeesQbtc + earnings.Rewards
		pr := float64(er) / float64(latestState.Pools[pool].QbtcDepth)
		mapEarningsInfo[pool] = EarningsInfo{
			Earnings:                   er,
			AnnualEarningsAsPercentage: (pr * periodsPerYear),
		}
	}

	aggregates := poolAggregates{
		depths:          latestState.Pools,
		dailyVolumes:    dailyVolumes,
		liquidityUnits:  liquidityUnitsNow,
		earningsDataMap: mapEarningsInfo,
	}

	return &aggregates, nil
}

func poolStatusFromMap(pool string, statusMap map[string]string) string {
	status, ok := statusMap[pool]
	if !ok {
		status = timeseries.DefaultPoolStatus
	}
	return status
}

// calculateDepthsPercentage calculates the liquidity depths at ± 2% price movements
// using Uniswap V2 constant product formula with fee from L1SLIPMINBPS mimir value.
// Returns depth in QBTC units for both directions.
// Depth represents the maximum QBTC value that can be traded before price moves ±2%.
func calculateDepthsPercentage(assetDepth, qbtcDepth int64) (depthPlus2Percent, depthMinus2Percent int64) {
	if assetDepth <= 0 || qbtcDepth <= 0 {
		return 0, 0
	}

	l1SlipMinBps := record.Recorder.CurrentMimirStatus("L1SLIPMINBPS")
	feeRate := float64(l1SlipMinBps) / 10000.0
	if feeRate <= 0 || feeRate >= 1.0 {
		feeRate = 0.001
	}

	feeMultiplier := 1.0 - feeRate
	const priceChange = 0.02 // 2%

	x := float64(assetDepth)
	y := float64(qbtcDepth)
	k := x * y

	xPrimeUp := x / (1 + priceChange)
	yPrimeUp := k / xPrimeUp
	dyPostFee := yPrimeUp - y

	dyIn := dyPostFee / feeMultiplier
	if dyIn < 0 {
		dyIn = 0
	}

	xPrimeDown := x / (1 - priceChange)
	dxPostFee := xPrimeDown - x

	dxIn := dxPostFee / feeMultiplier
	if dxIn < 0 {
		dxIn = 0
	}

	yPrimeDown := k / xPrimeDown
	dyOut := y - yPrimeDown
	if dyOut < 0 {
		dyOut = 0
	}

	return int64(dyIn), int64(dyOut)
}

func buildPoolDetail(
	ctx context.Context, pool, status string, aggregates poolAggregates, qbtcPriceUsd float64,
	decimal int64) oapigen.PoolDetail {
	assetDepth := aggregates.depths[pool].AssetDepth
	qbtcDepth := aggregates.depths[pool].QbtcDepth
	dailyVolume := aggregates.dailyVolumes[pool]
	liquidityUnits := aggregates.liquidityUnits[pool]
	poolUnits := liquidityUnits
	price := timeseries.AssetPrice(assetDepth, qbtcDepth)
	priceUSD := price * qbtcPriceUsd
	earnings := aggregates.earningsDataMap[pool].Earnings

	depthPlus2Percent, depthMinus2Percent := calculateDepthsPercentage(assetDepth, qbtcDepth)
	depthPlus2PercentStr := util.IntStr(depthPlus2Percent)
	depthMinus2PercentStr := util.IntStr(depthMinus2Percent)

	return oapigen.PoolDetail{
		Asset:              pool,
		AssetDepth:         util.IntStr(assetDepth),
		QbtcDepth:          util.IntStr(qbtcDepth),
		AssetPrice:         floatStr(price),
		AssetPriceUSD:      floatStr(priceUSD),
		LiquidityInUSD:     floatStr(qbtcPriceUsd * 2 * float64(qbtcDepth/1e8)),
		Status:             status,
		Units:              util.IntStr(poolUnits),
		LiquidityUnits:     util.IntStr(liquidityUnits),
		Volume24h:          util.IntStr(dailyVolume),
		NativeDecimal:      util.IntStr(decimal),
		Earnings:           util.IntStr(earnings),
		DepthPlus2Percent:  &depthPlus2PercentStr,
		DepthMinus2Percent: &depthMinus2PercentStr,
	}
}

func jsonPools(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	_, lastTime, _ := timeseries.LastBlock()
	statusMap, err := timeseries.GetPoolsStatuses(r.Context(), db.Nano(lastTime.UnixNano()))
	if err != nil {
		respError(w, err)
		return
	}
	pools, err := poolsWithRequestedStatus(r.Context(), &urlParams, statusMap)
	if err != nil {
		respError(w, err)
		return
	}

	apyBucket, err := parsePeriodParam(&urlParams, "14d")
	if err != nil {
		btcqerr.BadRequest(err.Error()).ReportHTTP(w)
		return
	}

	merr := util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	aggregates, err := getPoolAggregates(r.Context(), pools, apyBucket)
	if err != nil {
		respError(w, err)
		return
	}

	qbtcPriceUsd := stat.QbtcPriceUSD()

	poolsDecimal := decimal.PoolsDecimal()

	poolsResponse := oapigen.PoolsResponse{}
	for _, pool := range pools {
		qbtcDepth := aggregates.depths[pool].QbtcDepth
		assetDepth := aggregates.depths[pool].AssetDepth
		poolDecimal, ok := poolsDecimal[pool]
		if !ok {
			poolDecimal.NativeDecimals = -1
		}
		if 0 < qbtcDepth && 0 < assetDepth {
			status := poolStatusFromMap(pool, statusMap)
			poolsResponse = append(poolsResponse, buildPoolDetail(r.Context(), pool, status,
				*aggregates, qbtcPriceUsd, poolDecimal.NativeDecimals))
		}
	}

	respJSON(w, poolsResponse)
}

const defaultBlocksLimit = 20
const maxBlocksLimit = 100

func jsonBlocks(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()
	heightParam := strings.TrimSpace(util.ConsumeUrlParam(&urlParams, "height"))
	hashParam := strings.TrimSpace(util.ConsumeUrlParam(&urlParams, "hash"))
	limitParam := strings.TrimSpace(util.ConsumeUrlParam(&urlParams, "limit"))
	offsetParam := strings.TrimSpace(util.ConsumeUrlParam(&urlParams, "offset"))

	if heightParam != "" || hashParam != "" {
		if heightParam != "" && hashParam != "" {
			btcqerr.BadRequest("provide only one of height or hash").ReportHTTP(w)
			return
		}
		var block *db.BlockDetail
		var err error
		if heightParam != "" {
			height, err := strconv.ParseInt(heightParam, 10, 64)
			if err != nil || height < 0 {
				btcqerr.BadRequest("invalid height").ReportHTTP(w)
				return
			}
			block, err = db.GetBlockByHeight(r.Context(), height)
			if err != nil {
				btcqerr.InternalErrE(err).ReportHTTP(w)
				return
			}
		} else {
			if !util.IsValidHexHash(hashParam) {
				btcqerr.BadRequest("invalid hash").ReportHTTP(w)
				return
			}
			block, err = db.GetBlockByHash(r.Context(), hashParam)
			if err != nil {
				btcqerr.InternalErrE(err).ReportHTTP(w)
				return
			}
		}
		if block == nil {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		var finalizedEvents []oapigen.BlockEvent
		if len(block.FinalizedEvents) > 0 {
			if err := json.Unmarshal(block.FinalizedEvents, &finalizedEvents); err != nil {
				btcqerr.InternalErrE(err).ReportHTTP(w)
				return
			}
		}
		var txs []oapigen.BlockTx
		if len(block.Txs) > 0 {
			if err := json.Unmarshal(block.Txs, &txs); err != nil {
				btcqerr.InternalErrE(err).ReportHTTP(w)
				return
			}
		}
		resp := oapigen.BlockDetailResponse{
			Height:          block.Height,
			Timestamp:       int64(block.Timestamp),
			Hash:            db.PrintableHash(string(block.Hash)),
			FinalizedEvents: finalizedEvents,
			Txs:             txs,
		}
		respJSON(w, resp)
		return
	}

	limit := defaultBlocksLimit
	if limitParam != "" {
		l, err := strconv.Atoi(limitParam)
		if err != nil || l < 1 {
			btcqerr.BadRequest("invalid limit").ReportHTTP(w)
			return
		}
		if l > maxBlocksLimit {
			l = maxBlocksLimit
		}
		limit = l
	}
	offset := 0
	if offsetParam != "" {
		o, err := strconv.Atoi(offsetParam)
		if err != nil || o < 0 {
			btcqerr.BadRequest("invalid offset").ReportHTTP(w)
			return
		}
		offset = o
	}

	list, err := db.GetBlocksList(r.Context(), limit, offset)
	if err != nil {
		btcqerr.InternalErrE(err).ReportHTTP(w)
		return
	}
	items := make([]oapigen.BlockSummaryItem, 0, len(list))
	for _, s := range list {
		items = append(items, oapigen.BlockSummaryItem{
			Height:               s.Height,
			Timestamp:            int64(s.Timestamp),
			Hash:                 db.PrintableHash(string(s.Hash)),
			TxCount:              s.TxCount,
			FinalizedEventsCount: s.FinalizedEventsCount,
		})
	}
	respJSON(w, oapigen.BlocksListResponse{Blocks: items, Limit: limit, Offset: offset})
}

func jsonPool(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	urlParams := r.URL.Query()

	apyBucket, err := parsePeriodParam(&urlParams, "14d")
	if err != nil {
		btcqerr.BadRequest(err.Error()).ReportHTTP(w)
		return
	}

	merr := util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	pool := ps[0].Value

	if !timeseries.PoolExistsNow(pool) {
		btcqerr.BadRequestF("Unknown pool: %s", pool).ReportHTTP(w)
		return
	}

	status, err := timeseries.PoolStatus(r.Context(), pool)
	if err != nil {
		btcqerr.InternalErrE(err).ReportHTTP(w)
		return
	}

	aggregates, err := getPoolAggregates(r.Context(), []string{pool}, apyBucket)
	if err != nil {
		btcqerr.InternalErrE(err).ReportHTTP(w)
		return
	}

	qbtcPriceUsd := stat.QbtcPriceUSD()

	poolsDecimal := decimal.PoolsDecimal()

	poolDecimal, ok := poolsDecimal[pool]
	if !ok {
		poolDecimal.NativeDecimals = -1
	}

	poolResponse := oapigen.PoolResponse(
		buildPoolDetail(r.Context(), pool, status, *aggregates, qbtcPriceUsd,
			poolDecimal.NativeDecimals))
	respJSON(w, poolResponse)
}

// TODO(muninn): remove cache once it's <0.5s
func calculateJsonStats(ctx context.Context, w io.Writer) error {
	state := timeseries.Latest.GetState()
	now := db.NowSecond()
	window := db.Window{From: 0, Until: now}

	// TODO(huginn): Rewrite to member table if doable, stakes/unstakes lookup is ~0.8 s
	stakes, err := stat.StakesLookup(ctx, window)
	if err != nil {
		return err
	}
	withdraws, err := stat.WithdrawsLookup(ctx, window)
	if err != nil {
		return err
	}

	// swapsAll, err := stat.GlobalSwapStats(ctx, "day", 0)
	// if err != nil {
	// 	return err
	// }
	//
	// swaps24h, err := stat.GlobalSwapStats(ctx, "5min", now-24*60*60)
	// if err != nil {
	// 	return err
	// }
	//
	// swaps30d, err := stat.GlobalSwapStats(ctx, "hour", now-30*24*60*60)
	// if err != nil {
	// 	return err
	// }

	var qbtcDepth int64
	for _, poolInfo := range state.Pools {
		qbtcDepth += poolInfo.QbtcDepth
	}

	switchedQbtc, err := stat.SwitchedQbtc(ctx)
	if err != nil {
		return err
	}

	qbtcPrice := stat.QbtcPriceUSD()

	writeJSON(w, oapigen.StatsResponse{
		QbtcDepth:          util.IntStr(qbtcDepth),
		SwitchedQbtc:       util.IntStr(switchedQbtc),
		QbtcPriceUSD:       floatStr(qbtcPrice),
		SwapVolume:         "0",
		SwapCount24h:       "0",
		SwapCount30d:       "0",
		SwapCount:          "0",
		ToAssetCount:       "0",
		ToQbtcCount:        "0",
		SynthMintCount:     "0",
		SynthBurnCount:     "0",
		AddLiquidityVolume: util.IntStr(stakes.TotalVolume),
		WithdrawVolume:     util.IntStr(withdraws.TotalVolume),
		AddLiquidityCount:  util.IntStr(stakes.Count),
		WithdrawCount:      util.IntStr(withdraws.Count),
	})
	return nil
}

func cachedJsonStats() httprouter.Handle {
	cachedHandler := CreateAndRegisterCache(calculateJsonStats, "stats")
	return cachedHandler.ServeHTTP
}

func jsonBalance(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	urlParams := r.URL.Query()

	height := util.ConsumeUrlParam(&urlParams, "height")
	timestamp := util.ConsumeUrlParam(&urlParams, "timestamp")

	if merr := util.CheckUrlEmpty(urlParams); merr != nil {
		merr.ReportHTTP(w)
		return
	}

	address := ps[0].Value
	result, merr := timeseries.GetBalances(r.Context(), address, height, timestamp)

	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	respJSON(w, result)
}

func jsonSwagger(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	swagger, err := oapigen.GetSwagger()
	if err != nil {
		respError(w, err)
		return
	}
	respJSON(w, swagger)
}

func writeJSON(w io.Writer, body interface{}) {
	e := json.NewEncoder(w)
	e.SetIndent("", "\t")
	// Error discarded
	_ = e.Encode(body)
}

func respJSON(w http.ResponseWriter, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, body)
}

func respError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func ratioStr(a, b int64) string {
	if b == 0 {
		return "0"
	} else {
		return strconv.FormatFloat(float64(a)/float64(b), 'f', -1, 64)
	}
}

func floatStr(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func floatStrPtr(f float64) *string {
	if f == 0 {
		return nil
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return &s
}

// returns max 2 results
func withLowered(s string) []string {
	lower := strings.ToLower(s)
	if lower != s {
		return []string{s, lower}
	} else {
		return []string{s}
	}
}

func parsePeriodParam(urlParams *url.Values, def string) (db.Buckets, error) {
	period := util.ConsumeUrlParam(urlParams, "period")
	if period == "" {
		period = def
	}
	var buckets db.Buckets
	now := db.NowSecond()
	switch period {
	case "1h":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 60*60, now}}
	case "24h":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 24*60*60, now}}
	case "7d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 7*24*60*60, now}}
	case "14d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 14*24*60*60, now}}
	case "30d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 30*24*60*60, now}}
	case "90d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 90*24*60*60, now}}
	case "100d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 100*24*60*60, now}}
	case "180d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 180*24*60*60, now}}
	case "365d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 365*24*60*60, now}}
	case "all":
		buckets = db.AllHistoryBuckets()
	default:
		return db.Buckets{}, fmt.Errorf(
			"invalid `period` param: %s. Accepted values:  1h, 24h, 7d, 14d, 30d, 90d, 100d, 180d, 365d, all",
			period)
	}

	return buckets, nil
}

func jsonQbtcPriceHistory(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	buckets, merr := db.BucketsFromQuery(r.Context(), &urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	merr = util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	ret, err := stat.GetQbtcPriceHistory(r.Context(), buckets)
	if err != nil {
		btcqerr.InternalErrE(err).ReportHTTP(w)
		return
	}

	respJSON(w, ret)
}


