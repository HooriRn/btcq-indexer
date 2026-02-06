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
	"time"

	"github.com/btcq/btcq-indexer/config"
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

func jsonSwapHistory(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	buckets, merr := db.BucketsFromQuery(r.Context(), &urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	var pool *string
	poolParam := util.ConsumeUrlParam(&urlParams, "pool")
	if poolParam != "" {
		pool = &poolParam
	}

	merr = util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	mergedPoolSwaps, err := stat.GetPoolSwaps(r.Context(), pool, buckets)
	if err != nil {
		btcqerr.InternalErr(err.Error()).ReportHTTP(w)
		return
	}
	var result oapigen.SwapHistoryResponse = createVolumeIntervals(mergedPoolSwaps)
	if buckets.OneInterval() {
		result.Intervals = oapigen.SwapHistoryIntervals{}
	}
	respJSON(w, result)
}

func toSwapHistoryItem(bucket stat.SwapBucket) oapigen.SwapHistoryItem {
	return oapigen.SwapHistoryItem{
		StartTime:              util.IntStr(bucket.StartTime.ToI()),
		EndTime:                util.IntStr(bucket.EndTime.ToI()),
		ToAssetVolume:          util.IntStr(bucket.RuneToAssetVolume),
		ToRuneVolume:           util.IntStr(bucket.AssetToRuneVolume),
		ToTradeVolume:          util.IntStr(bucket.RuneToTradeVolume),
		FromTradeVolume:        util.IntStr(bucket.TradeToRuneVolume),
		ToSecuredVolume:        util.IntStr(bucket.SecuredToRuneVolume),
		FromSecuredVolume:      util.IntStr(bucket.RuneToSecuredVolume),
		SynthMintVolume:        util.IntStr(bucket.RuneToSynthVolume),
		SynthRedeemVolume:      util.IntStr(bucket.SynthToRuneVolume),
		TotalVolume:            util.IntStr(bucket.TotalVolume),
		ToAssetVolumeUSD:       util.IntStr(bucket.RuneToAssetVolumeUSD),
		ToRuneVolumeUSD:        util.IntStr(bucket.AssetToRuneVolumeUSD),
		ToTradeVolumeUSD:       util.IntStr(bucket.RuneToTradeVolumeUSD),
		FromTradeVolumeUSD:     util.IntStr(bucket.TradeToRuneVolumeUSD),
		ToSecuredVolumeUSD:     util.IntStr(bucket.RuneToSecuredVolumeUSD),
		FromSecuredVolumeUSD:   util.IntStr(bucket.SecuredToRuneVolumeUSD),
		SynthMintVolumeUSD:     util.IntStr(bucket.RuneToSynthVolumeUSD),
		SynthRedeemVolumeUSD:   util.IntStr(bucket.SynthToRuneVolumeUSD),
		TotalVolumeUSD:         util.IntStr(bucket.TotalVolumeUSD),
		ToAssetCount:           util.IntStr(bucket.RuneToAssetCount),
		ToRuneCount:            util.IntStr(bucket.AssetToRuneCount),
		ToTradeCount:           util.IntStr(bucket.RuneToTradeCount),
		FromTradeCount:         util.IntStr(bucket.TradeToRuneCount),
		ToSecuredCount:         util.IntStr(bucket.SecuredToRuneCount),
		FromSecuredCount:       util.IntStr(bucket.RuneToSecuredCount),
		SynthMintCount:         util.IntStr(bucket.RuneToSynthCount),
		SynthRedeemCount:       util.IntStr(bucket.SynthToRuneCount),
		TotalCount:             util.IntStr(bucket.TotalCount),
		ToAssetFees:            util.IntStr(bucket.RuneToAssetFees),
		ToRuneFees:             util.IntStr(bucket.AssetToRuneFees),
		ToTradeFees:            util.IntStr(bucket.RuneToTradeFees),
		FromTradeFees:          util.IntStr(bucket.TradeToRuneFees),
		ToSecuredFees:          util.IntStr(bucket.RuneToSecuredFees),
		FromSecuredFees:        util.IntStr(bucket.SecuredToRuneFees),
		SynthMintFees:          util.IntStr(bucket.RuneToSynthFees),
		SynthRedeemFees:        util.IntStr(bucket.SynthToRuneFees),
		TotalFees:              util.IntStr(bucket.TotalFees),
		ToAssetAverageSlip:     ratioStr(bucket.RuneToAssetSlip, bucket.RuneToAssetCount),
		ToRuneAverageSlip:      ratioStr(bucket.AssetToRuneSlip, bucket.AssetToRuneCount),
		ToTradeAverageSlip:     ratioStr(bucket.RuneToTradeSlip, bucket.RuneToTradeCount),
		FromTradeAverageSlip:   ratioStr(bucket.TradeToRuneSlip, bucket.TradeToRuneCount),
		ToSecuredAverageSlip:   ratioStr(bucket.SecuredToRuneSlip, bucket.RuneToSecuredCount),
		FromSecuredAverageSlip: ratioStr(bucket.SecuredToRuneSlip, bucket.SecuredToRuneCount),
		SynthMintAverageSlip:   ratioStr(bucket.RuneToSynthSlip, bucket.RuneToSynthCount),
		SynthRedeemAverageSlip: ratioStr(bucket.SynthToRuneSlip, bucket.SynthToRuneCount),
		AverageSlip:            ratioStr(bucket.TotalSlip, bucket.TotalCount),
		QbtcPriceUSD:           floatStr(bucket.QbtcPriceUSD),
	}
}

func createVolumeIntervals(buckets []stat.SwapBucket) (result oapigen.SwapHistoryResponse) {
	metaBucket := stat.SwapBucket{}

	for _, bucket := range buckets {
		metaBucket.AddBucket(bucket)

		result.Intervals = append(result.Intervals, toSwapHistoryItem(bucket))
	}

	result.Meta = toSwapHistoryItem(metaBucket)
	result.Meta.StartTime = result.Intervals[0].StartTime
	result.Meta.EndTime = result.Intervals[len(result.Intervals)-1].EndTime
	result.Meta.RunePriceUSD = result.Intervals[len(result.Intervals)-1].RunePriceUSD
	return
}

// TODO(huginn): remove when bonds are fixed
var ShowBonds bool = false

func jsonTVLHistory(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
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

	// TODO(huginn): optimize, just this call is 1.8 sec
	// defer timer.Console("tvlDepthSingle")()
	depths, err := stat.TVLDepthHistory(r.Context(), buckets)
	if err != nil {
		btcqerr.InternalErrE(err).ReportHTTP(w)
		return
	}

	bonds, err := stat.BondsHistory(r.Context(), buckets)
	if err != nil {
		btcqerr.InternalErrE(err).ReportHTTP(w)
		return
	}
	if len(depths) != len(bonds) || depths[0].Window != bonds[0].Window {
		btcqerr.InternalErr("Buckets misalligned").ReportHTTP(w)
		return
	}

	var result oapigen.TVLHistoryResponse = toTVLHistoryResponse(depths, bonds)
	respJSON(w, result)
}

func toTVLHistoryResponse(depths []stat.TVLDepthBucket, bonds []stat.BondBucket) (
	result oapigen.TVLHistoryResponse) {

	showBonds := func(value string) *string {
		if !ShowBonds {
			return nil
		}
		return &value
	}

	result.Intervals = make(oapigen.TVLHistoryIntervals, 0, len(depths))
	for i, bucket := range depths {
		pools := 2 * bucket.TotalPoolDepth
		bonds := bonds[i].Bonds
		poolsDepth := toOapiPoolsDepth(bucket.PoolsMapRuneDepth)
		result.Intervals = append(result.Intervals, oapigen.TVLHistoryItem{
			StartTime:        util.IntStr(bucket.Window.From.ToI()),
			EndTime:          util.IntStr(bucket.Window.Until.ToI()),
			TotalValuePooled: util.IntStr(pools),
			TotalValueBonded: showBonds(util.IntStr(bonds)),
			TotalValueLocked: showBonds(util.IntStr(pools + bonds)),
			RunePriceUSD:     floatStr(bucket.RunePriceUSD),
			PoolsDepth:       poolsDepth,
		})
	}
	result.Meta = result.Intervals[len(depths)-1]
	result.Meta.StartTime = result.Intervals[0].StartTime
	return
}

func toOapiPoolsDepth(poolsMapDepth map[string]int64) []oapigen.DepthHistoryItemPool {
	ret := make([]oapigen.DepthHistoryItemPool, 0)
	for poolName, poolDepth := range poolsMapDepth {
		ret = append(ret, oapigen.DepthHistoryItemPool{
			Pool:       poolName,
			TotalDepth: util.IntStr(2 * poolDepth),
		})
	}
	return ret
}

func jsonNetwork(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	merr := util.CheckUrlEmpty(r.URL.Query())
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	network, err := timeseries.GetNetworkData(r.Context())
	if err != nil {
		respError(w, err)
		return
	}

	respJSON(w, network)
}

// TODO(HooriRn): this struct is not needed since the graphql depracation, replace with the corresponding oapi version. (delete-graphql)
type Node struct {
	Secp256K1 string `json:"secp256k1"`
	Ed25519   string `json:"ed25519"`
}

func jsonNodes(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	secpAddrs, edAddrs, err := timeseries.NodesSecpAndEd(r.Context(), time.Now())
	if err != nil {
		respError(w, err)
		return
	}

	m := make(map[string]struct {
		Secp string
		Ed   string
	}, len(secpAddrs))
	for key, addr := range secpAddrs {
		e := m[addr]
		e.Secp = key
		m[addr] = e
	}
	for key, addr := range edAddrs {
		e := m[addr]
		e.Ed = key
		m[addr] = e
	}

	array := make([]oapigen.Node, 0, len(m))
	for key, e := range m {
		array = append(array, oapigen.Node{
			Secp256k1:   e.Secp,
			Ed25519:     e.Ed,
			NodeAddress: key,
		})
	}
	respJSON(w, array)
}

func jsonKnownPools(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()
	merr := util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	lastTime := timeseries.Latest.GetState().Timestamp
	pools, err := timeseries.GetPoolsStatuses(r.Context(), lastTime)
	if err != nil {
		respError(w, err)
		return
	}

	respJSON(w, oapigen.KnownPools{AdditionalProperties: pools})
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

type SaverData struct {
	SaversUnits int64
	SaversDepth int64
}

type EarningsInfo struct {
	Earnings                   int64
	AnnualEarningsAsPercentage float64
}

type poolAggregates struct {
	depths               timeseries.DepthMap
	dailyVolumes         map[string]int64
	liquidityUnits       map[string]int64
	annualPercentageRate map[string]float64
	saverDataMap         map[string]SaverData
	earningsDataMap      map[string]EarningsInfo
	lpLuvi               map[string]float64
}

func getPoolAggregates(ctx context.Context, pools []string, apyBucket db.Buckets) (
	*poolAggregates, error) {

	latestState := timeseries.Latest.GetState()
	now := latestState.NextSecond()
	window24h := db.Window{From: now - 24*60*60, Until: now}

	dailyVolumes, err := stat.PoolsTotalVolume(ctx, pools, window24h)
	if err != nil {
		return nil, err
	}

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

	saverData := getSaversData(latestState.Pools, liquidityUnitsNow)

	mapLpLuvi, err := GetPoolAPRs(ctx, latestState.Pools, liquidityUnitsNow, pools,
		apyBucket.Start().ToNano(), apyBucket.End().ToNano())
	if err != nil {
		return nil, err
	}

	// Get earnings data for the pools
	poolEarningsMapStat, err := stat.GetPoolsEarnings(ctx, apyBucket)
	if err != nil {
		return nil, err
	}

	mapEarningsInfo := make(map[string]EarningsInfo)
	earningsApy := make(map[string]float64)
	periodsPerYear := db.GetPPYFromBuckets(apyBucket)
	for pool, earnings := range poolEarningsMapStat {
		er := earnings.TotalLiquidityFeesRune + earnings.Rewards
		pr := float64(er) / float64(latestState.Pools[pool].QbtcDepth)
		mapEarningsInfo[pool] = EarningsInfo{
			Earnings:                   er,
			AnnualEarningsAsPercentage: (pr * periodsPerYear),
		}
		earningsApy[pool] = timeseries.CalculateAPYInterest(pr, periodsPerYear)
	}

	// TODO (HooriRn): Delete APR
	aggregates := poolAggregates{
		depths:               latestState.Pools,
		dailyVolumes:         dailyVolumes,
		liquidityUnits:       liquidityUnitsNow,
		annualPercentageRate: earningsApy,
		saverDataMap:         saverData,
		earningsDataMap:      mapEarningsInfo,
		lpLuvi:               mapLpLuvi,
	}

	return &aggregates, nil
}

func getSaversData(depths timeseries.DepthMap, liquidityUnits map[string]int64) map[string]SaverData {
	ret := map[string]SaverData{}
	for p := range depths {
		actualPoolName := strings.Replace(p, "/", ".", 1)
		if record.GetCoinType([]byte(p)) == record.AssetSynth {
			ret[actualPoolName] = SaverData{
				SaversDepth: depths[p].AssetDepth,
				SaversUnits: liquidityUnits[p],
			}
		}
	}
	return ret
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
	synthSupply := aggregates.depths[pool].SynthDepth
	dailyVolume := aggregates.dailyVolumes[pool]
	liquidityUnits := aggregates.liquidityUnits[pool]
	synthUnits := timeseries.CalculateSynthUnits(assetDepth, synthSupply, liquidityUnits)
	poolUnits := liquidityUnits + synthUnits
	price := timeseries.AssetPrice(assetDepth, qbtcDepth)
	priceUSD := price * qbtcPriceUsd
	saversUnit := aggregates.saverDataMap[pool].SaversUnits
	saversDepth := aggregates.saverDataMap[pool].SaversDepth
	earnings := aggregates.earningsDataMap[pool].Earnings
	annualEarningsPerDepth := aggregates.earningsDataMap[pool].AnnualEarningsAsPercentage
	poolApy := aggregates.annualPercentageRate[pool]
	poolLuvi := aggregates.lpLuvi[pool]

	var SaversYieldShare float64 = 0
	if status == "available" && assetDepth > 0 && saversUnit > 0 {
		maxSynthForSaversYield := float64(record.Recorder.CurrentMimirStatus("MAXSYNTHSFORSAVERSYIELD"))
		synthYieldBps := float64(record.Recorder.CurrentMimirStatus("SYNTHYIELDBASISPOINTS"))
		synthPerPoolDepth := (float64(synthUnits) / float64(poolUnits)) * 1e4
		SaversYieldShare = synthYieldBps - (synthYieldBps * synthPerPoolDepth / maxSynthForSaversYield)
	}

	saversAPR := 0.00
	if _, ok := aggregates.depths[util.ConvertNativePoolToSynth(pool)]; ok {
		saversAPR = aggregates.lpLuvi[util.ConvertNativePoolToSynth(pool)]
	}

	depthPlus2Percent, depthMinus2Percent := calculateDepthsPercentage(assetDepth, runeDepth)
	depthPlus2PercentStr := util.IntStr(depthPlus2Percent)
	depthMinus2PercentStr := util.IntStr(depthMinus2Percent)

	return oapigen.PoolDetail{
		Asset:                          pool,
		AssetDepth:                     util.IntStr(assetDepth),
		RuneDepth:                      util.IntStr(runeDepth),
		AssetPrice:                     floatStr(price),
		AssetPriceUSD:                  floatStr(priceUSD),
		LiquidityInUSD:                 floatStr(runePriceUsd * 2 * float64(runeDepth/1e8)),
		Status:                         status,
		Units:                          util.IntStr(poolUnits),
		LiquidityUnits:                 util.IntStr(liquidityUnits),
		SynthUnits:                     util.IntStr(synthUnits),
		SynthSupply:                    util.IntStr(synthSupply),
		Volume24h:                      util.IntStr(dailyVolume),
		NativeDecimal:                  util.IntStr(decimal),
		SaversUnits:                    util.IntStr(saversUnit),
		SaversDepth:                    util.IntStr(saversDepth),
		SaversAPR:                      floatStr(saversAPR),
		Earnings:                       util.IntStr(earnings),
		EarningsAnnualAsPercentOfDepth: floatStr(annualEarningsPerDepth),
		AnnualPercentageRate:           floatStr(poolApy),
		PoolAPY:                        floatStr(poolApy),
		LpLuvi:                         floatStr(poolLuvi),
		SaversYieldShare:               floatStrPtr(SaversYieldShare / 1e4),
		DepthPlus2Percent:              &depthPlus2PercentStr,
		DepthMinus2Percent:             &depthMinus2PercentStr,
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
		runeDepth := aggregates.depths[pool].RuneDepth
		assetDepth := aggregates.depths[pool].AssetDepth
		poolDecimal, ok := poolsDecimal[pool]
		if !ok {
			poolDecimal.NativeDecimals = -1
		}
		if 0 < runeDepth && 0 < assetDepth {
			status := poolStatusFromMap(pool, statusMap)
			poolsResponse = append(poolsResponse, buildPoolDetail(r.Context(), pool, status,
				*aggregates, runePriceUsd, poolDecimal.NativeDecimals))
		}
	}

	respJSON(w, poolsResponse)
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

// returns string array
func jsonMembers(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	var pool *string
	poolParam := util.ConsumeUrlParam(&urlParams, "pool")
	if poolParam != "" {
		pool = &poolParam
		if !timeseries.PoolExists(*pool) {
			btcqerr.BadRequestF("Unknown pool: %s", *pool).ReportHTTP(w)
			return
		}
	}
	merr := util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	addrs, err := timeseries.GetMemberIds(r.Context(), pool)
	if err != nil {
		respError(w, err)
		return
	}
	result := oapigen.MembersResponse(addrs)
	respJSON(w, result)
}

func jsonMemberDetails(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	urlParams := r.URL.Query()

	showPoolsType := timeseries.RegularPools
	if util.ConsumeUrlParam(&urlParams, "showSavers") == "true" {
		showPoolsType = timeseries.RegularAndSaverPools
	}

	if merr := util.CheckUrlEmpty(urlParams); merr != nil {
		merr.ReportHTTP(w)
		return
	}

	addr := strings.Join(withLowered(ps[0].Value), ",")

	addrs := strings.Split(addr, ",")
	pools, err := timeseries.GetMemberPools(r.Context(), addrs, showPoolsType)
	if err != nil {
		respError(w, err)
		return
	}

	if len(pools) == 0 {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	respJSON(w, oapigen.MemberDetailsResponse{
		Pools: pools.ToOapigen(),
	})
}

func getSaversRedeemValue(pools timeseries.MemberPools, poolsDepthMap timeseries.DepthMap,
	poolsUnitsMap map[string]int64) map[string]int64 {
	poolRedeemValueMap := map[string]int64{}
	for _, pool := range pools {
		poolUnits := float64(poolsUnitsMap[pool.Pool])
		if poolUnits == 0.0 {
			continue
		}
		saversDepth := float64(poolsDepthMap[pool.Pool].AssetDepth)
		poolRedeemValueMap[pool.Pool] = int64((saversDepth * float64(pool.LiquidityUnits)) / poolUnits)
	}

	return poolRedeemValueMap
}

func jsonSaverDetails(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	urlParams := r.URL.Query()

	if merr := util.CheckUrlEmpty(urlParams); merr != nil {
		merr.ReportHTTP(w)
		return
	}

	addr := strings.Join(withLowered(ps[0].Value), ",")

	addrs := strings.Split(addr, ",")
	pools, err := timeseries.GetMemberPools(r.Context(), addrs, timeseries.SaverPools)
	if err != nil {
		respError(w, err)
		return
	}

	if len(pools) == 0 {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	poolDepthMap := timeseries.Latest.GetState().Pools
	memberPools := make([]string, 0)
	for _, memberPool := range pools {
		if timeseries.PoolExists(memberPool.Pool) {
			memberPools = append(memberPools, memberPool.Pool)
		}
	}
	poolUnitsMap, err := stat.CurrentPoolsLiquidityUnits(r.Context(), memberPools)
	if err != nil {
		respError(w, err)
		return
	}

	poolRedeemValue := getSaversRedeemValue(pools, poolDepthMap, poolUnitsMap)

	respJSON(w, oapigen.SaverDetailsResponse{
		Pools: pools.ToSavers(poolRedeemValue),
	})
}

func jsonTHORName(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	merr := util.CheckUrlEmpty(r.URL.Query())
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	name := ps[0].Value

	n, err := timeseries.GetTHORName(r.Context(), name)
	if err != nil {
		respError(w, err)
		return
	}
	if n.Owner == "" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	entries := make([]oapigen.THORNameEntry, len(n.Entries))
	for i, e := range n.Entries {
		entries[i] = oapigen.THORNameEntry{
			Chain:   e.Chain,
			Address: e.Address,
		}
	}

	respJSON(w, oapigen.THORNameDetailsResponse{
		Owner:   n.Owner,
		Expire:  util.IntStr(n.Expire),
		Entries: entries,
	})
}

type ThornameReverseLookupFunc func(ctx context.Context, addr string) (names []string, err error)

func jsonTHORNameReverse(
	w http.ResponseWriter, r *http.Request, ps httprouter.Params,
	lookupFunc ThornameReverseLookupFunc) {

	merr := util.CheckUrlEmpty(r.URL.Query())
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	addr := ps[0].Value

	var names []string
	for _, addr := range withLowered(addr) {
		var err error
		names, err = lookupFunc(r.Context(), addr)
		if err != nil {
			respError(w, err)
			return
		}
		if 0 < len(names) {
			break
		}
	}

	if len(names) == 0 {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	respJSON(w, oapigen.ReverseTHORNameResponse(
		names,
	))
}

func jsonTHORNameAddress(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	jsonTHORNameReverse(w, r, ps, timeseries.GetTHORNamesByAddress)
}

func jsonTHORNameOwner(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	jsonTHORNameReverse(w, r, ps, timeseries.GetTHORNamesByOwnerAddress)
}

func jsonChurns(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	churns, err := timeseries.GetChurnsData(r.Context())
	if err != nil {
		return
	}
	respJSON(w, churns)
}

func jsonVotes(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	period, err := parsePeriodParam(&urlParams, "90d")
	if err != nil {
		btcqerr.BadRequest(err.Error()).ReportHTTP(w)
		return
	}

	votes, err := timeseries.GetVotesStats(r.Context(), period)
	if err != nil {
		return
	}

	votesResponse := oapigen.VotesResponse{}
	for value, vi := range votes {
		votesResponse = append(votesResponse, oapigen.VoteValue{
			Value: value,
			Votes: vi,
		})
	}
	respJSON(w, votesResponse)
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

	swapsAll, err := stat.GlobalSwapStats(ctx, "day", 0)
	if err != nil {
		return err
	}

	swaps24h, err := stat.GlobalSwapStats(ctx, "5min", now-24*60*60)
	if err != nil {
		return err
	}

	swaps30d, err := stat.GlobalSwapStats(ctx, "hour", now-30*24*60*60)
	if err != nil {
		return err
	}

	var qbtcDepth int64
	for poolName, poolInfo := range state.Pools {
		if record.GetCoinType([]byte(poolName)) != record.AssetDerived {
			qbtcDepth += poolInfo.QbtcDepth
		}
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
		SwapVolume:         util.IntStr(swapsAll.Totals().Volume),
		SwapCount24h:       util.IntStr(swaps24h.Totals().Count),
		SwapCount30d:       util.IntStr(swaps30d.Totals().Count),
		SwapCount:          util.IntStr(swapsAll.Totals().Count),
		ToAssetCount:       util.IntStr(swapsAll[db.RuneToAsset].Count),
		ToRuneCount:        util.IntStr(swapsAll[db.AssetToRune].Count),
		SynthMintCount:     util.IntStr(swapsAll[db.RuneToSynth].Count),
		SynthBurnCount:     util.IntStr(swapsAll[db.SynthToRune].Count),
		DailyActiveUsers:   "0", // deprecated
		MonthlyActiveUsers: "0", // deprecated
		UniqueSwapperCount: "0", // deprecated
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

func jsonActions(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()
	params := timeseries.ActionsParams{
		Limit:         util.ConsumeUrlParam(&urlParams, "limit"),
		NextPageToken: util.ConsumeUrlParam(&urlParams, "nextPageToken"),
		PrevPageToken: util.ConsumeUrlParam(&urlParams, "prevPageToken"),
		Timestamp:     util.ConsumeUrlParam(&urlParams, "timestamp"),
		Height:        util.ConsumeUrlParam(&urlParams, "height"),
		FromTimestamp: util.ConsumeUrlParam(&urlParams, "fromTimestamp"),
		FromHeight:    util.ConsumeUrlParam(&urlParams, "fromHeight"),
		Offset:        util.ConsumeUrlParam(&urlParams, "offset"),
		ActionType:    util.ConsumeUrlParam(&urlParams, "type"),
		Address:       util.ConsumeUrlParam(&urlParams, "address"),
		TXId:          util.ConsumeUrlParam(&urlParams, "txid"),
		Asset:         util.ConsumeUrlParam(&urlParams, "asset"),
		TxType:        util.ConsumeUrlParam(&urlParams, "txType"),
		Affiliate:     util.ConsumeUrlParam(&urlParams, "affiliate"),
	}

	merr := util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	var actions oapigen.ActionsResponse
	var err error
	filteredAddresses := config.Global.FilteredAddresses
	for _, addr := range withLowered(params.Address) {
		params.Address = addr
		if name, ok := filteredAddresses[addr]; ok {
			errMsg := "The requested address is filtered, It might be one of module addresses."
			if len(name) > 0 {
				errMsg += "\n\nLabel: %s"
				respError(w, btcqerr.BadRequestF(errMsg, name))
			} else {
				respError(w, btcqerr.BadRequestF(errMsg, name))
			}
			return
		}
		actions, err = timeseries.GetActions(r.Context(), time.Time{}, params)
		if err != nil {
			respError(w, err)
			return
		}
		if len(actions.Actions) != 0 {
			break
		}
	}

	respJSON(w, actions)
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

func jsonHolders(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	urlParams := r.URL.Query()

	asset := util.ConsumeUrlParam(&urlParams, "asset")
	limitStr := util.ConsumeUrlParam(&urlParams, "limit")

	// Default values
	if asset == "" {
		asset = "THOR.RUNE"
	}
	var limit int64 = 100
	if limitStr != "" {
		var err error
		limit, err = strconv.ParseInt(limitStr, 10, 64)
		if err != nil {
			btcqerr.BadRequest("Invalid limit parameter").ReportHTTP(w)
			return
		}
	}

	if merr := util.CheckUrlEmpty(urlParams); merr != nil {
		merr.ReportHTTP(w)
		return
	}

	result, merr := timeseries.GetTopHolders(r.Context(), asset, limit)

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

func jsonSwaps(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	period, err := parsePeriodParam(&urlParams, "24h")
	if err != nil {
		btcqerr.BadRequest(err.Error()).ReportHTTP(w)
		return
	}

	result, err := timeseries.GetTopSwaps(r.Context(), period)

	if err != nil {
		respError(w, err)
		return
	}

	respJSON(w, result)
}

func jsonReserveHistory(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
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

	ret, err := stat.GetReserveHistory(r.Context(), buckets)
	if err != nil {
		btcqerr.InternalErrE(err).ReportHTTP(w)
		return
	}

	respJSON(w, ret)
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

func jsonAffiliateHistory(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	buckets, merr := db.BucketsFromQuery(r.Context(), &urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	var thorname *string
	poolParam := util.ConsumeUrlParam(&urlParams, "thorname")
	if poolParam != "" {
		thorname = &poolParam
	}

	merr = util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	mergedAffiliates, err := stat.GetTHORNameAffiliate(r.Context(), thorname, buckets)
	if err != nil {
		btcqerr.InternalErr(err.Error()).ReportHTTP(w)
		return
	}

	respJSON(w, mergedAffiliates)
}

func jsonRUJIMerge(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	period, err := parsePeriodParam(&urlParams, "all")
	if err != nil {
		btcqerr.BadRequest(err.Error()).ReportHTTP(w)
		return
	}

	switches, err := timeseries.GetSwitchStats(r.Context(), period)
	if err != nil {
		return
	}

	respJSON(w, switches)
}

func jsonTCYDistribution(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	urlParams := r.URL.Query()
	address := ps[0].Value

	period, err := parsePeriodParam(&urlParams, "30d")
	if err != nil {
		btcqerr.BadRequest(err.Error()).ReportHTTP(w)
		return
	}

	merr := util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	ret, err := stat.GetTCYDistribution(r.Context(), address, period)
	if err != nil {
		btcqerr.InternalErrE(err).ReportHTTP(w)
		return
	}

	respJSON(w, ret)
}

func jsonAffiliateStats(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	buckets, merr := db.BucketsFromQuery(r.Context(), &urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	thorname := util.ConsumeUrlParam(&urlParams, "thorname")

	merr = util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	result, err := timeseries.GetAffiliateStats(r.Context(), buckets, thorname)

	if err != nil {
		btcqerr.InternalErrE(err).ReportHTTP(w)
		return
	}

	respJSON(w, result)
}

func jsonAffiliateEarning(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	buckets, merr := db.BucketsFromQuery(r.Context(), &urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	thorname := util.ConsumeUrlParam(&urlParams, "thorname")

	merr = util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	result, err := timeseries.GetAffiliateEarning(r.Context(), buckets, thorname)
	if err != nil {
		btcqerr.InternalErrE(err).ReportHTTP(w)
		return
	}

	respJSON(w, result)
}
