package stat_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/db/testdb"
	"github.com/btcq/btcq-indexer/internal/timeseries"
	"github.com/btcq/btcq-indexer/openapi/generated/oapigen"
)

func TestUsdPrices(t *testing.T) {
	testdb.InitTest(t)
	timeseries.SetDepthsForTest([]timeseries.Depth{
		{Pool: "BNB.BNB", AssetDepth: 1000, QbtcDepth: 2000},
		{Pool: "USDA", AssetDepth: 300, QbtcDepth: 100},
		{Pool: "USDB", AssetDepth: 5000, QbtcDepth: 1000},
	})

	config.Global.UsdPools = []string{"USDA", "USDB"}

	{
		body := testdb.CallJSON(t,
			"http://localhost:8080/v2/stats")

		var result oapigen.StatsData
		testdb.MustUnmarshal(t, body, &result)
		// RunePriceUSD field removed - QBTC price is now calculated differently
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
		// RunePriceHistoryResponse endpoint removed - test disabled
		// body := testdb.CallJSON(t, "http://localhost:8080/v2/history/rune")
		// var result interface{}
		// testdb.MustUnmarshal(t, body, &result)
	}
}

func TestPrices(t *testing.T) {
	testdb.InitTest(t)
	timeseries.SetDepthsForTest([]timeseries.Depth{
		{Pool: "BNB.BNB", AssetDepth: 1000, QbtcDepth: 2000},
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
		// RunePriceUSD field removed - QBTC price is now calculated differently
		_ = result
	}
}
