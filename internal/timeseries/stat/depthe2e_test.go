package stat_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/db/testdb"
	"github.com/btcq/btcq-indexer/openapi/generated/oapigen"
)

func TestDepthHistoryE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-01-05 12:00:00",
		testdb.AddLiquidity{
			RuneAddress:  "thoraddr1",
			AssetAddress: "bnbaddr1",
			Pool:         "BNB.BNB",
			AssetAmount:  10,
			RuneAmount:   10,
		},
		testdb.PoolActivate("BNB.BNB"),
	)
	blocks.NewBlock(t, "2020-01-05 12:00:10",
		testdb.Withdraw{
			Pool:        "BNB.BNB",
			Coin:        "10 BNB",
			EmitAsset:   10,
			EmitRune:    10,
			FromAddress: "thoraddr1",
			ToAddress:   "thoraddr2",
			ID:          "withdraw1",
			Asymmetry:   "0.000000000000000000",
			BasisPoints: 1000,
		},
	)
	blocks.NewBlock(t, "2020-01-06 12:00:00",
		testdb.AddLiquidity{
			RuneAddress:  "thoraddr1",
			AssetAddress: "bnbaddr1",
			Pool:         "BNB.BNB",
			AssetAmount:  20,
			RuneAmount:   20,
		},
	)

	blocks.NewBlock(t, "2020-01-10 12:00:05",
		testdb.AddLiquidity{
			RuneAddress:  "thoraddr1",
			AssetAddress: "bnbaddr1",
			Pool:         "BNB.BNB",
			AssetAmount:  2,
			RuneAmount:   2,
		},
	)
	blocks.NewBlock(t, "2020-01-11 12:00:00",
		testdb.Swap{
			Pool:               "BNB.BNB",
			Coin:               "10 BNB.BNB",
			EmitAsset:          "10 THOR.RUNE",
			FromAddress:        "bnb1",
			ToAddress:          "thoraddr3",
			TxID:               "swap1",
			LiquidityFeeInRune: 1,
			LiquidityFee:       1,
		},
	)
	blocks.NewBlock(t, "2020-01-12 10:00:00",
		testdb.AddLiquidity{
			RuneAddress:  "thoraddr1",
			AssetAddress: "bnbaddr1",
			Pool:         "BNB.BNB",
			AssetAmount:  1,
			RuneAmount:   1,
		},
	)
	db.RefreshAggregatesForTests()

	from := db.StrToSec("2020-01-09 00:00:00")
	to := db.StrToSec("2020-01-13 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BNB.BNB?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, oapigen.DepthHistoryMeta{
		StartTime:        epochStr("2020-01-09 00:00:00"),
		EndTime:          epochStr("2020-01-13 00:00:00"),
		PriceShiftLoss:   "0.8844332774281065",
		LuviIncrease:     "0.33166247903553997",
		StartAssetDepth:  "20",
		StartLPUnits:     "1",
		StartSynthUnits:  "0",
		StartMemberCount: "1",
		StartQbtcDepth:   "20",
		EndAssetDepth:    "33",
		EndLPUnits:       "3",
		EndSynthUnits:    "0",
		EndQbtcDepth:     "12",
		EndMemberCount:   "1",
	}, jsonResult.Meta)
	require.Equal(t, 4, len(jsonResult.Intervals))
	require.Equal(t, epochStr("2020-01-09 00:00:00"), jsonResult.Intervals[0].StartTime)
	require.Equal(t, epochStr("2020-01-10 00:00:00"), jsonResult.Intervals[0].EndTime)
	require.Equal(t, epochStr("2020-01-13 00:00:00"), jsonResult.Intervals[3].EndTime)

	// initial value correct
	jan9 := jsonResult.Intervals[0]
	require.Equal(t, "20", jan9.QbtcDepth)

	jan10 := jsonResult.Intervals[1]
	require.Equal(t, "22", jan10.QbtcDepth)
	require.Equal(t, "22", jan10.AssetDepth)
	require.Equal(t, "1", jan10.AssetPrice)

	// gapfill works.
	jan11 := jsonResult.Intervals[2]
	require.Equal(t, "0.34375", jan11.AssetPrice)
}

func TestUSDHistoryE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)
	testdb.DeclarePools("BNB.BNB", "BNB.USDA", "BNB.USDB")

	originalUsdPools := config.Global.UsdPools
	defer func() { config.Global.UsdPools = originalUsdPools }()
	config.Global.UsdPools = []string{"BNB.USDA", "BNB.USDB", "ABC.USD1", "ABC.USD2"}

	blocks.NewBlock(t, "2020-01-05 12:00:00",
		testdb.AddLiquidity{
			Pool:                   "BNB.BNB",
			RuneAddress:            "thoraddr1",
			AssetAddress:           "bnbaddr1",
			AssetAmount:            10,
			RuneAmount:             20,
			LiquidityProviderUnits: 100,
		},
		testdb.PoolActivate("BNB.BNB"),
		testdb.AddLiquidity{
			Pool:                   "BNB.USDA",
			AssetAmount:            200,
			RuneAmount:             100,
			LiquidityProviderUnits: 100,
		},
		testdb.PoolActivate("BNB.USDA"),
		testdb.AddLiquidity{
			Pool:                   "BNB.USDB",
			AssetAmount:            30,
			RuneAmount:             10,
			LiquidityProviderUnits: 100,
		},
		testdb.PoolActivate("USDB"),
	)

	// Deepen USDB so the USD price median tilts towards it.
	blocks.NewBlock(t, "2020-01-10 12:00:05",
		testdb.AddLiquidity{
			Pool:                   "BNB.USDB",
			AssetAmount:            2970,
			RuneAmount:             990,
			LiquidityProviderUnits: 100,
		},
	)

	// Remove USDA liquidity entirely, forcing rune price USD to come from USDB only.
	blocks.NewBlock(t, "2020-01-11 12:00:05",
		testdb.Withdraw{
			Pool:                   "BNB.USDA",
			EmitAsset:              200,
			EmitRune:               100,
			LiquidityProviderUnits: 100,
			FromAddress:            "thoraddr1",
			ToAddress:              "thoraddr1",
		},
	)

	// Change the BNB pool ratio to validate USD prices after rune price shift.
	blocks.NewBlock(t, "2020-01-13 12:00:00",
		testdb.AddLiquidity{
			Pool:                   "BNB.BNB",
			RuneAddress:            "thoraddr1",
			AssetAddress:           "bnbaddr1",
			AssetAmount:            5,
			RuneAmount:             30,
			LiquidityProviderUnits: 50,
		},
	)

	from := db.StrToSec("2020-01-09 00:00:00")
	to := db.StrToSec("2020-01-14 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BNB.BNB?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, 5, len(jsonResult.Intervals))
	require.Equal(t, epochStr("2020-01-09 00:00:00"), jsonResult.Intervals[0].StartTime)

	require.Equal(t, "2", jsonResult.Intervals[0].AssetPrice)

	require.Equal(t, "5", jsonResult.Intervals[0].AssetPriceUSD)
	require.Equal(t, "5", jsonResult.Intervals[1].AssetPriceUSD)
	require.Equal(t, "6", jsonResult.Intervals[2].AssetPriceUSD)
	require.Equal(t, "6", jsonResult.Intervals[3].AssetPriceUSD)
	require.Equal(t, "10", jsonResult.Intervals[4].AssetPriceUSD)
}

func TestLiquidityUnitsHistoryE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-01-10 12:00:00",
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			RuneAddress:            "thoraddr1",
			AssetAddress:           "btcaddr1",
			AssetAmount:            10,
			RuneAmount:             10,
			LiquidityProviderUnits: 10,
		},
		testdb.PoolActivate("BTC.BTC"),
	)

	blocks.NewBlock(t, "2020-01-20 12:00:00",
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			RuneAddress:            "thoraddr2",
			AssetAddress:           "btcaddr2",
			AssetAmount:            10,
			RuneAmount:             10,
			LiquidityProviderUnits: 10, // total 20
		},
	)

	// This block belongs to a different pool and should be ignored.
	blocks.NewBlock(t, "2020-01-20 13:00:00",
		testdb.AddLiquidity{
			Pool:                   "BNB.BNB",
			RuneAddress:            "thoraddr3",
			AssetAddress:           "bnbaddr1",
			AssetAmount:            1000,
			RuneAmount:             1000,
			LiquidityProviderUnits: 1000,
		},
		testdb.PoolActivate("BNB.BNB"),
	)

	blocks.NewBlock(t, "2020-01-21 12:00:00",
		testdb.Withdraw{
			Pool:                   "BTC.BTC",
			EmitAsset:              5,
			EmitRune:               5,
			LiquidityProviderUnits: 5, // total 15
			FromAddress:            "btcaddr2",
			ToAddress:              "thoraddr2",
		},
	)

	from := db.StrToSec("2020-01-19 00:00:00")
	to := db.StrToSec("2020-01-22 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BTC.BTC?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, 3, len(jsonResult.Intervals))
	require.Equal(t, epochStr("2020-01-20 00:00:00"), jsonResult.Intervals[0].EndTime)
	require.Equal(t, "10", jsonResult.Intervals[0].LiquidityUnits)

	require.Equal(t, epochStr("2020-01-21 00:00:00"), jsonResult.Intervals[1].EndTime)
	require.Equal(t, "20", jsonResult.Intervals[1].LiquidityUnits)

	require.Equal(t, epochStr("2020-01-22 00:00:00"), jsonResult.Intervals[2].EndTime)
	require.Equal(t, "15", jsonResult.Intervals[2].LiquidityUnits)
}

func TestMembersHistoryE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-01-10 12:00:00",
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			AssetAddress:           "btc1",
			AssetAmount:            10,
			RuneAmount:             10,
			LiquidityProviderUnits: 10,
		},
		testdb.PoolActivate("BTC.BTC"),
	)

	blocks.NewBlock(t, "2020-01-20 12:00:00",
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			RuneAddress:            "thor1",
			AssetAddress:           "btc2",
			AssetAmount:            10,
			RuneAmount:             10,
			LiquidityProviderUnits: 10,
		},
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			AssetAddress:           "btc1",
			AssetAmount:            10,
			RuneAmount:             10,
			LiquidityProviderUnits: 10, // Total 20 for btc1
		},
	)

	blocks.NewBlock(t, "2020-01-20 13:00:00",
		testdb.AddLiquidity{
			Pool:                   "BNB.BNB",
			RuneAddress:            "thor1",
			AssetAddress:           "bnb1",
			AssetAmount:            1000,
			RuneAmount:             1000,
			LiquidityProviderUnits: 1000,
		},
		testdb.PoolActivate("BNB.BNB"),
	)

	blocks.NewBlock(t, "2020-01-21 12:00:00",
		testdb.Withdraw{
			Pool:                   "BTC.BTC",
			EmitAsset:              20,
			EmitRune:               20,
			LiquidityProviderUnits: 20,
			FromAddress:            "btc1",
			ToAddress:              "thoraddr1",
		},
		testdb.Withdraw{
			Pool:                   "BTC.BTC",
			EmitAsset:              5,
			EmitRune:               5,
			LiquidityProviderUnits: 5,
			FromAddress:            "thor1",
			ToAddress:              "btc2",
		},
	)

	from := db.StrToSec("2020-01-19 00:00:00")
	to := db.StrToSec("2020-01-22 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BTC.BTC?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, 3, len(jsonResult.Intervals))
	require.Equal(t, epochStr("2020-01-20 00:00:00"), jsonResult.Intervals[0].EndTime)
	require.Equal(t, "1", jsonResult.Intervals[0].MembersCount)

	require.Equal(t, epochStr("2020-01-21 00:00:00"), jsonResult.Intervals[1].EndTime)
	require.Equal(t, "2", jsonResult.Intervals[1].MembersCount)

	require.Equal(t, epochStr("2020-01-22 00:00:00"), jsonResult.Intervals[2].EndTime)
	require.Equal(t, "1", jsonResult.Intervals[2].MembersCount)
}

func TestDepthAggregateE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	// The first block feeds the continuous aggregate; later ones exercise the live table.
	blocks.NewBlock(t, "2020-01-01 23:57:00",
		testdb.AddLiquidity{
			Pool:                   "A.A",
			AssetAmount:            1,
			RuneAmount:             30,
			LiquidityProviderUnits: 30,
		},
		testdb.PoolActivate("A.A"),
		testdb.AddLiquidity{
			Pool:                   "B.B",
			AssetAmount:            1,
			RuneAmount:             10,
			LiquidityProviderUnits: 10,
		},
		testdb.PoolActivate("B.B"),
	)

	blocks.NewBlock(t, "2020-01-02 00:02:00",
		testdb.AddLiquidity{
			Pool:                   "A.A",
			AssetAmount:            1,
			RuneAmount:             10,
			LiquidityProviderUnits: 10,
		},
	)

	blocks.NewBlock(t, "2020-01-02 00:03:00",
		testdb.AddLiquidity{
			Pool:                   "A.A",
			AssetAmount:            1,
			RuneAmount:             20,
			LiquidityProviderUnits: 20,
		},
		testdb.AddLiquidity{
			Pool:                   "B.B",
			AssetAmount:            1,
			RuneAmount:             10,
			LiquidityProviderUnits: 10,
		},
	)

	to := db.StrToSec("2020-01-02 00:02:30")
	var jsonResult oapigen.DepthHistoryResponse

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/A.A?&to=%d", to))
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, "40", jsonResult.Intervals[0].QbtcDepth)

	body = testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/B.B?&to=%d", to))
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, "10", jsonResult.Intervals[0].QbtcDepth)
}

func TestLiqUnitValueIndexWithInterval(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2010-01-01 23:57:00",
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			RuneAddress:            "thoraddr1",
			AssetAmount:            100,
			RuneAmount:             1000,
			LiquidityProviderUnits: 10,
		},
		testdb.PoolActivate("BTC.BTC"),
	)

	blocks.NewBlock(t, "2010-02-01 23:57:00",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "550 THOR.RUNE",
			EmitAsset:          "50 BTC.BTC",
			LiquidityFeeInRune: 0,
			LiquidityFee:       0,
			Slip:               42,
		},
	)
	// Pool balance after: 50 btc, 1550 rune

	blocks.NewBlock(t, "2010-02-01 23:57:01",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "170 BTC.BTC",
			EmitAsset:          "1000 THOR.RUNE",
			LiquidityFeeInRune: 0,
			LiquidityFee:       0,
			Slip:               42,
		},
	)
	// Pool balance after: 220 btc, 550 rune

	blocks.NewBlock(t, "2010-02-22 00:00:01")

	from := db.StrToSec("2010-01-01 00:00:00")
	to := db.StrToSec("2010-02-22 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BTC.BTC?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, "220", jsonResult.Intervals[51].AssetDepth)
	require.Equal(t, "550", jsonResult.Intervals[51].QbtcDepth)

	//sqrt(100 * 1000) / 10, for both intervals, as we did not increase / decrease liquidity
	testdb.RoughlyEqual(t, 31.622776, jsonResult.Intervals[0].Luvi)
	testdb.RoughlyEqual(t, 31.622776, jsonResult.Intervals[1].Luvi)
	//sqrt(220 * 550) / 10
	testdb.RoughlyEqual(t, 34.78505, jsonResult.Intervals[51].Luvi)

	//edge case, since we added the block pool depth, the depth will be 0, so the priceshift loss is not present at all
	require.Equal(t, "NaN", jsonResult.Meta.PriceShiftLoss)

	from = db.StrToSec("2010-01-02 00:00:00")
	body = testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BTC.BTC?interval=day&from=%d&to=%d", from, to))

	testdb.MustUnmarshal(t, body, &jsonResult)
	//this should be 2*sqrt(0.5)/1.5
	testdb.RoughlyEqual(t, 0.8, jsonResult.Meta.PriceShiftLoss)
	testdb.RoughlyEqual(t, 1.1, jsonResult.Meta.LuviIncrease) //minimal luvi decrease
}

func TestLiqUnitValueIndexWithoutInterval(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2010-01-01 23:57:00",
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			RuneAddress:            "thoraddr1",
			AssetAmount:            100,
			RuneAmount:             1000,
			LiquidityProviderUnits: 10,
		},
		testdb.PoolActivate("BTC.BTC"),
	)

	blocks.NewBlock(t, "2010-02-01 23:57:00",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "550 THOR.RUNE",
			EmitAsset:          "50 BTC.BTC",
			LiquidityFeeInRune: 1,
			LiquidityFee:       1,
			Slip:               42,
		},
	)
	// Pool balance after: 50 btc, 1550 rune

	blocks.NewBlock(t, "2010-02-01 23:57:01",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "170 BTC.BTC",
			EmitAsset:          "1000 THOR.RUNE",
			LiquidityFeeInRune: 1,
			LiquidityFee:       1,
			Slip:               42,
		},
	)
	// Pool balance after: 220 btc, 550 rune

	blocks.NewBlock(t, "2010-02-22 00:00:01")

	from := db.StrToSec("2010-01-01 00:00:00")
	to := db.StrToSec("2010-02-22 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BTC.BTC?from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	//sqrt(100 * 998), for both intervals, as we did not increase / decrease liquidity
	require.Equal(t, 1, len(jsonResult.Intervals))
	testdb.RoughlyEqual(t, 34.721751, jsonResult.Intervals[0].Luvi)

	//edge case, since we added the block pool depth, the depth will be 0, so the priceshift loss is not present at all
	require.Equal(t, "NaN", jsonResult.Meta.PriceShiftLoss)

	from = db.StrToSec("2010-01-02 00:00:00")
	body = testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BTC.BTC?from=%d&to=%d", from, to))

	testdb.MustUnmarshal(t, body, &jsonResult)
	//this should be 2*sqrt(0.5)/1.5
	testdb.RoughlyEqual(t, 0.799125, jsonResult.Meta.PriceShiftLoss)
	testdb.RoughlyEqual(t, 1.097998, jsonResult.Meta.LuviIncrease) //minimal luvi decrease
}

func TestLiqUnitValueIndexSynths(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-01-01 23:57:00",
		testdb.AddLiquidity{Pool: "ETH.ETH", AssetAmount: 100 * 100000000, RuneAmount: 1000 * 100000000, LiquidityProviderUnits: 1},
		testdb.PoolActivate("ETH.ETH"),
	)

	blocks.NewBlock(t, "2020-02-21 23:57:00", testdb.Swap{
		Pool:      "ETH.ETH",
		Coin:      "100000000 THOR.RUNE",
		EmitAsset: "42 ETH/ETH",
	})

	db.RefreshAggregatesForTests()

	from := db.StrToSec("2020-01-01 00:00:00")
	to := db.StrToSec("2020-02-22 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/ETH.ETH?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	//sqrt(100*100000000 * 1000*100000000), for both intervals, as we did not increase / decrease liquidity
	require.Equal(t, "31622776601.683792", jsonResult.Intervals[0].Luvi)
	require.Equal(t, "31622776601.683792", jsonResult.Intervals[1].Luvi)
	//sqrt(100*100000000 * 1001*100000000), we have 1 rune in synth, needs to be included
	require.Equal(t, "31638584039.11275", jsonResult.Intervals[51].Luvi)

	from = db.StrToSec("2020-01-02 00:00:00")
	body = testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/ETH.ETH?interval=day&from=%d&to=%d", from, to))

	testdb.MustUnmarshal(t, body, &jsonResult)
	//sqrt(100*100000000 * 1001*100000000) / sqrt(100*100000000 * 1000*100000000)
	require.Equal(t, "1.000499875062461", jsonResult.Meta.LuviIncrease) //minimal luvi decrease

	body = testdb.CallJSON(t,
		fmt.Sprintf("http://localhost:8080/v2/pool/ETH.ETH?period=30d"))

	var poolResult oapigen.PoolDetail
	testdb.MustUnmarshal(t, body, &poolResult)

	// TODO (HooriRn): Delete this, Deprecated
	// AnnualPercentageRate and PoolAPY fields removed
	_ = poolResult
}

func TestOHLCPricesE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)
	testdb.DeclarePools("BTC.BTC", "BNB.USDA")
	originalUsdPools := config.Global.UsdPools
	defer func() { config.Global.UsdPools = originalUsdPools }()
	config.Global.UsdPools = []string{"BNB.USDA"}

	// Set up USD pool for rune price calculation
	blocks.NewBlock(t, "2020-01-05 12:00:00",
		testdb.AddLiquidity{
			Pool:                   "BNB.USDA",
			RuneAddress:            "thorusd",
			AssetAddress:           "usdaddr",
			AssetAmount:            200,
			RuneAmount:             100,
			LiquidityProviderUnits: 100,
		},
		testdb.PoolActivate("BNB.USDA"),
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			RuneAddress:            "thoraddr",
			AssetAddress:           "btcaddr",
			AssetAmount:            10,
			RuneAmount:             10,
			LiquidityProviderUnits: 100,
		},
		testdb.PoolActivate("BTC.BTC"),
	)

	// Day 1 (2020-01-10): multiple price changes
	blocks.NewBlock(t, "2020-01-10 00:00:05",
		testdb.Withdraw{
			Pool:                   "BTC.BTC",
			EmitAsset:              10,
			EmitRune:               10,
			LiquidityProviderUnits: 100,
			FromAddress:            "btcaddr",
			ToAddress:              "thoraddr",
		},
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			RuneAddress:            "thoraddr",
			AssetAddress:           "btcaddr",
			AssetAmount:            10,
			RuneAmount:             20,
			LiquidityProviderUnits: 100,
		},
	)

	blocks.NewBlock(t, "2020-01-10 06:00:00",
		testdb.Withdraw{
			Pool:                   "BTC.BTC",
			EmitAsset:              10,
			EmitRune:               20,
			LiquidityProviderUnits: 100,
			FromAddress:            "btcaddr",
			ToAddress:              "thoraddr",
		},
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			RuneAddress:            "thoraddr",
			AssetAddress:           "btcaddr",
			AssetAmount:            5,
			RuneAmount:             30,
			LiquidityProviderUnits: 100,
		},
	)

	blocks.NewBlock(t, "2020-01-10 12:00:00",
		testdb.Withdraw{
			Pool:                   "BTC.BTC",
			EmitAsset:              5,
			EmitRune:               30,
			LiquidityProviderUnits: 100,
			FromAddress:            "btcaddr",
			ToAddress:              "thoraddr",
		},
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			RuneAddress:            "thoraddr",
			AssetAddress:           "btcaddr",
			AssetAmount:            20,
			RuneAmount:             10,
			LiquidityProviderUnits: 100,
		},
	)

	blocks.NewBlock(t, "2020-01-10 23:00:00",
		testdb.Withdraw{
			Pool:                   "BTC.BTC",
			EmitAsset:              20,
			EmitRune:               10,
			LiquidityProviderUnits: 100,
			FromAddress:            "btcaddr",
			ToAddress:              "thoraddr",
		},
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			RuneAddress:            "thoraddr",
			AssetAddress:           "btcaddr",
			AssetAmount:            8,
			RuneAmount:             16,
			LiquidityProviderUnits: 100,
		},
	)

	// Day 2 (2020-01-11): single price point
	blocks.NewBlock(t, "2020-01-11 12:00:00",
		testdb.Withdraw{
			Pool:                   "BTC.BTC",
			EmitAsset:              8,
			EmitRune:               16,
			LiquidityProviderUnits: 100,
			FromAddress:            "btcaddr",
			ToAddress:              "thoraddr",
		},
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			RuneAddress:            "thoraddr",
			AssetAddress:           "btcaddr",
			AssetAmount:            4,
			RuneAmount:             12,
			LiquidityProviderUnits: 100,
		},
	)

	from := db.StrToSec("2020-01-10 00:00:00")
	to := db.StrToSec("2020-01-12 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BTC.BTC?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, 2, len(jsonResult.Intervals))

	// Day 1: Check OHLC prices
	day1 := jsonResult.Intervals[0]
	require.NotNil(t, day1.OpenPriceUSD, "OpenPriceUSD should not be nil")
	require.NotNil(t, day1.HighPriceUSD, "HighPriceUSD should not be nil")
	require.NotNil(t, day1.LowPriceUSD, "LowPriceUSD should not be nil")
	require.NotNil(t, day1.ClosePriceUSD, "ClosePriceUSD should not be nil")

	// Open: first price = 2.0 * 2.0 = 4.0
	testdb.RoughlyEqual(t, 4.0, *day1.OpenPriceUSD)

	// High: max price = 6.0 * 2.0 = 12.0
	testdb.RoughlyEqual(t, 12.0, *day1.HighPriceUSD)

	// Low: min price = 0.5 * 2.0 = 1.0
	testdb.RoughlyEqual(t, 1.0, *day1.LowPriceUSD)

	// Close: last price = 2.0 * 2.0 = 4.0
	testdb.RoughlyEqual(t, 4.0, *day1.ClosePriceUSD)

	// Verify High >= Open, Close and Low <= Open, Close
	highPrice, _ := strconv.ParseFloat(*day1.HighPriceUSD, 64)
	openPrice, _ := strconv.ParseFloat(*day1.OpenPriceUSD, 64)
	closePrice, _ := strconv.ParseFloat(*day1.ClosePriceUSD, 64)
	lowPrice, _ := strconv.ParseFloat(*day1.LowPriceUSD, 64)
	require.GreaterOrEqual(t, highPrice, openPrice)
	require.GreaterOrEqual(t, highPrice, closePrice)
	require.LessOrEqual(t, lowPrice, openPrice)
	require.LessOrEqual(t, lowPrice, closePrice)

	// Day 2: Single price point, all OHLC should be the same
	day2 := jsonResult.Intervals[1]
	require.NotNil(t, day2.OpenPriceUSD)
	require.NotNil(t, day2.HighPriceUSD)
	require.NotNil(t, day2.LowPriceUSD)
	require.NotNil(t, day2.ClosePriceUSD)

	// All should be 3.0 * 2.0 = 6.0
	expectedPrice := 6.0
	testdb.RoughlyEqual(t, expectedPrice, *day2.OpenPriceUSD)
	testdb.RoughlyEqual(t, expectedPrice, *day2.HighPriceUSD)
	testdb.RoughlyEqual(t, expectedPrice, *day2.LowPriceUSD)
	testdb.RoughlyEqual(t, expectedPrice, *day2.ClosePriceUSD)
}
