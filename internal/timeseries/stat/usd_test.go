package stat_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func TestUsdPrices(t *testing.T) {
	testdb.InitTest(t)
	timeseries.SetDepthsForTest([]timeseries.Depth{
		{Pool: "BNB.BNB", AssetDepth: 1000, RuneDepth: 2000},
		{Pool: "USDA", AssetDepth: 300, RuneDepth: 100},
		{Pool: "USDB", AssetDepth: 5000, RuneDepth: 1000},
	})

	config.Global.UsdPools = []string{"USDA", "USDB"}

	{
		body := testdb.CallJSON(t,
			"http://localhost:8080/v2/stats")

		var result oapigen.StatsData
		testdb.MustUnmarshal(t, body, &result)
		require.Equal(t, "4", result.RunePriceUSD)
	}

	{
		body := testdb.CallJSON(t,
			"http://localhost:8080/v2/pool/BNB.BNB/stats")

		var result oapigen.PoolStatsDetail
		testdb.MustUnmarshal(t, body, &result)
		require.Equal(t, "8", result.AssetPriceUSD)
	}

	{
		body := testdb.CallJSON(t,
			"http://localhost:8080/v2/pool/BNB.BNB")

		var result oapigen.PoolDetail
		testdb.MustUnmarshal(t, body, &result)
		require.Equal(t, "8", result.AssetPriceUSD)
	}
}

func TestRuneUsdPrice(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)
	config.Global.UsdPools = []string{"ETH.USDA", "BNB.USDB"}

	blocks.NewBlock(t, "2020-09-01 00:10:00",
		testdb.PoolActivate("BNB.USDB"),
		testdb.PoolActivate("ETH.USDA"),
		testdb.AddLiquidity{
			Pool:                   "BNB.USDB",
			AssetAddress:           "bnbaddr1",
			RuneAddress:            "thoraddr1",
			AssetAmount:            100_00000000,
			RuneAmount:             50_00000000,
			LiquidityProviderUnits: 2,
		},
		testdb.AddLiquidity{
			Pool:                   "ETH.USDA",
			AssetAddress:           "ethaddr1",
			RuneAddress:            "thoraddr1",
			AssetAmount:            100_00000000,
			RuneAmount:             20_00000000,
			LiquidityProviderUnits: 2,
		},
	)

	blocks.NewBlock(t, "2020-09-01 00:10:05")

	{
		body := testdb.CallJSON(t,
			"http://localhost:8080/v2/history/rune")

		var result oapigen.RunePriceHistoryResponse
		testdb.MustUnmarshal(t, body, &result)
		require.Equal(t, "3.5", result.Meta.StartRunePriceUSD)
		require.Equal(t, "3.5", result.Meta.EndRunePriceUSD)
	}
}

func TestPrices(t *testing.T) {
	testdb.InitTest(t)
	timeseries.SetDepthsForTest([]timeseries.Depth{
		{Pool: "BNB.BNB", AssetDepth: 1000, RuneDepth: 2000},
	})

	{
		body := testdb.CallJSON(t,
			"http://localhost:8080/v2/pool/BNB.BNB/stats")

		var result oapigen.PoolStatsDetail
		testdb.MustUnmarshal(t, body, &result)
		require.Equal(t, "2", result.AssetPrice)
	}

	{
		body := testdb.CallJSON(t,
			"http://localhost:8080/v2/pool/BNB.BNB")

		var result oapigen.PoolDetail
		testdb.MustUnmarshal(t, body, &result)
		require.Equal(t, "2", result.AssetPrice)
	}
}

func TestUSDPriceRecord(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	config.Global.UsdPools = []string{"ETH.USDA", "BNB.USDB"}

	blocks.NewBlock(t, "2020-09-01 00:10:00",
		testdb.PoolActivate("BNB.USDB"),
		testdb.PoolActivate("ETH.USDA"),
		testdb.AddLiquidity{
			Pool:                   "BNB.USDB",
			AssetAddress:           "bnbaddr1",
			RuneAddress:            "thoraddr1",
			AssetAmount:            100_00000000,
			RuneAmount:             50_00000000,
			LiquidityProviderUnits: 2,
		},
		testdb.AddLiquidity{
			Pool:                   "ETH.USDA",
			AssetAddress:           "ethaddr1",
			RuneAddress:            "thoraddr1",
			AssetAmount:            100_00000000,
			RuneAmount:             20_00000000,
			LiquidityProviderUnits: 2,
		},
	)

	blocks.NewBlock(t, "2020-09-01 00:10:05",
		testdb.SetMimir{
			Key:   "HALTETHCHAIN",
			Value: 1,
		},
	)

	{
		body := testdb.CallJSON(t,
			"http://localhost:8080/v2/stats")

		var result oapigen.StatsData
		testdb.MustUnmarshal(t, body, &result)
		require.Equal(t, "3.5", result.RunePriceUSD)
	}
}
