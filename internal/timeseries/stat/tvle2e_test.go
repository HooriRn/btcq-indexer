package stat_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/api"
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/db/testdb"
	"github.com/btcq/btcq-indexer/openapi/generated/oapigen"
)

func stringp(s string) *string {
	return &s
}

func TestTVLHistoryE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)
	testdb.DeclarePools("ABC.ABC", "ABC.XYZ", "ABC.USD1", "ABC.USD2")
	originalShowBonds := api.ShowBonds
	api.ShowBonds = true
	defer func() { api.ShowBonds = originalShowBonds }()

	originalUsdPools := config.Global.UsdPools
	config.Global.UsdPools = []string{"ABC.USD1", "ABC.USD2"}
	defer func() { config.Global.UsdPools = originalUsdPools }()

	// Not all pools existed before the start time
	blocks.NewBlock(t, "2020-01-05 12:00:00",
		testdb.AddLiquidity{
			Pool:                   "ABC.XYZ",
			RuneAddress:            "thorxyz",
			AssetAddress:           "abcxyz",
			AssetAmount:            10,
			RuneAmount:             100,
			LiquidityProviderUnits: 100,
		},
		testdb.PoolActivate("ABC.XYZ"),
		testdb.AddLiquidity{
			Pool:                   "ABC.USD1",
			RuneAddress:            "thorusd1",
			AssetAddress:           "abcusd1",
			AssetAmount:            100,
			RuneAmount:             10,
			LiquidityProviderUnits: 100,
		},
		testdb.PoolActivate("ABC.USD1"),
	)

	blocks.NewBlock(t, "2020-01-10 12:00:05",
		testdb.AddLiquidity{
			Pool:                   "ABC.ABC",
			RuneAddress:            "thorabc",
			AssetAddress:           "abcaddr",
			AssetAmount:            10,
			RuneAmount:             20,
			LiquidityProviderUnits: 20,
		},
		testdb.PoolActivate("ABC.ABC"),
	)

	blocks.NewBlock(t, "2020-01-10 14:00:00",
		testdb.AddLiquidity{
			Pool:                   "ABC.ABC",
			AssetAmount:            10,
			RuneAmount:             10,
			LiquidityProviderUnits: 10,
		},
	)

	blocks.NewBlock(t, "2020-01-11 14:00:00",
		testdb.AddLiquidity{
			Pool:                   "ABC.XYZ",
			AssetAmount:            10,
			RuneAmount:             50,
			LiquidityProviderUnits: 50,
		},
	)

	blocks.NewBlock(t, "2020-01-13 07:00:00",
		testdb.Withdraw{
			Pool:                   "ABC.USD1",
			Coin:                   "100 ABC.USD1",
			EmitAsset:              100,
			EmitRune:               10,
			LiquidityProviderUnits: 100,
			FromAddress:            "thorusd1",
			ToAddress:              "thorusd1",
			Asymmetry:              "0.000000000000000000",
			BasisPoints:            1000,
		},
	)

	blocks.NewBlock(t, "2020-01-13 08:00:00",
		testdb.AddLiquidity{
			Pool:                   "ABC.USD2",
			RuneAddress:            "thorusd2",
			AssetAddress:           "abcusd2",
			AssetAmount:            20,
			RuneAmount:             10,
			LiquidityProviderUnits: 100,
		},
		testdb.PoolActivate("ABC.USD2"),
	)

	blocks.NewBlock(t, "2020-01-13 09:00:00",
		testdb.Withdraw{
			Pool:                   "ABC.ABC",
			Coin:                   "18 ABC.ABC",
			EmitAsset:              18,
			EmitRune:               25,
			LiquidityProviderUnits: 25,
			FromAddress:            "thorabc",
			ToAddress:              "thorabc",
			Asymmetry:              "0.000000000000000000",
			BasisPoints:            1000,
		},
	)

	blocks.NewBlock(t, "2020-01-13 10:00:00",
		testdb.AddLiquidity{
			Pool:                   "ABC.ABC",
			AssetAmount:            4,
			RuneAmount:             13,
			LiquidityProviderUnits: 13,
		},
	)

	db.RefreshAggregatesForTests()

	from := db.StrToSec("2020-01-09 00:00:00")
	to := db.StrToSec("2020-01-14 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/tvl?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.TVLHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, epochStr("2020-01-09 00:00:00"), jsonResult.Meta.StartTime)
	require.Equal(t, epochStr("2020-01-14 00:00:00"), jsonResult.Meta.EndTime)
	require.Equal(t, "356", jsonResult.Meta.TotalValuePooled)
	require.Equal(t, stringp("0"), jsonResult.Meta.TotalValueBonded)
	require.Equal(t, stringp("356"), jsonResult.Meta.TotalValueLocked)
	require.Equal(t, "2", jsonResult.Meta.RunePriceUSD)
	require.Equal(t, 4, len(jsonResult.Meta.PoolsDepth))
	for _, item := range jsonResult.Meta.PoolsDepth {
		switch item.Pool {
		case "ABC.USD1":
			require.Equal(t, "0", item.TotalDepth)
		case "ABC.XYZ":
			require.Equal(t, "300", item.TotalDepth)
		case "ABC.ABC":
			require.Equal(t, "36", item.TotalDepth)
		case "ABC.USD2":
			require.Equal(t, "20", item.TotalDepth)
		}
	}

	require.Equal(t, 5, len(jsonResult.Intervals))
	require.Equal(t, epochStr("2020-01-09 00:00:00"), jsonResult.Intervals[0].StartTime)
	require.Equal(t, epochStr("2020-01-10 00:00:00"), jsonResult.Intervals[0].EndTime)
	require.Equal(t, epochStr("2020-01-14 00:00:00"), jsonResult.Intervals[4].EndTime)

	require.Equal(t, "220", jsonResult.Intervals[0].TotalValuePooled) // from initial values
	require.Equal(t, "280", jsonResult.Intervals[1].TotalValuePooled)
	require.Equal(t, "380", jsonResult.Intervals[2].TotalValuePooled)
	require.Equal(t, "380", jsonResult.Intervals[3].TotalValuePooled) // gapfill
	require.Equal(t, "10", jsonResult.Intervals[3].RunePriceUSD)      // initial USD price
	require.Equal(t, "356", jsonResult.Intervals[4].TotalValuePooled)
}

func TestTVLHistoryBondsE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)
	originalShowBonds := api.ShowBonds
	api.ShowBonds = true
	defer func() { api.ShowBonds = originalShowBonds }()

	addBondEvent := func(eventType string, e8 int64, timestamp string) {
		blocks.NewBlock(t, timestamp,
			testdb.Bond{
				BondType: eventType,
				E8:       e8,
				Coin:     fmt.Sprintf("%d THOR.RUNE", e8),
				From:     "thor1bond",
				To:       "bond_module",
				Chain:    "THOR",
			},
		)
	}

	// This will be the initial value
	addBondEvent("bond_paid", 100, "2020-01-05 12:00:00")

	addBondEvent("bond_cost", 20, "2020-01-10 12:00:00")
	addBondEvent("bond_reward", 10, "2020-01-10 14:00:00")

	addBondEvent("bond_returned", 50, "2020-01-12 09:00:00")
	addBondEvent("bond_paid", 100, "2020-01-12 16:00:00")

	// This will be skipped because we query 01-09 to 01-14
	addBondEvent("bond_paid", 1000, "2020-01-14 12:00:00")

	db.RefreshAggregatesForTests()

	from := db.StrToSec("2020-01-09 00:00:00")
	to := db.StrToSec("2020-01-13 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/tvl?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.TVLHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, oapigen.TVLHistoryItem{
		StartTime:        epochStr("2020-01-09 00:00:00"),
		EndTime:          epochStr("2020-01-13 00:00:00"),
		TotalValuePooled: "0",
		TotalValueBonded: stringp("140"),
		TotalValueLocked: stringp("140"),
		RunePriceUSD:     "NaN",
		PoolsDepth:       []oapigen.DepthHistoryItemPool{},
	}, jsonResult.Meta)
	require.Equal(t, 4, len(jsonResult.Intervals))
	require.Equal(t, epochStr("2020-01-09 00:00:00"), jsonResult.Intervals[0].StartTime)
	require.Equal(t, epochStr("2020-01-10 00:00:00"), jsonResult.Intervals[0].EndTime)
	require.Equal(t, epochStr("2020-01-13 00:00:00"), jsonResult.Intervals[3].EndTime)

	require.Equal(t, stringp("100"), jsonResult.Intervals[0].TotalValueBonded) // from initial values
	require.Equal(t, stringp("90"), jsonResult.Intervals[1].TotalValueBonded)
	require.Equal(t, stringp("90"), jsonResult.Intervals[2].TotalValueBonded) // gapfill
	require.Equal(t, stringp("140"), jsonResult.Intervals[3].TotalValueBonded)
}
