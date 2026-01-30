package stat_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func TestAffiliateFee(t *testing.T) {
	config.Global.UsdPools = []string{"ETH.USDA", "ETH.USDB"}

	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2019-12-25 12:00:00", testdb.AddLiquidity{
		Pool: "ETH.USDB", AssetAmount: 300 * 1e8, RuneAmount: 100 * 1e8,
	}, testdb.PoolActivate("ETH.USDB"))

	blocks.NewBlock(t, "2020-01-02 12:00:00", testdb.AddLiquidity{
		Pool: "ETH.USDA", AssetAmount: 200 * 1e8, RuneAmount: 100 * 1e8,
	}, testdb.PoolActivate("ETH.USDA"))

	blocks.NewBlock(t, "2020-01-03 13:00:00", testdb.Swap{
		TxID:               "44444",
		Pool:               "ETH.USDA",
		EmitAsset:          "1000000000 THOR.RUNE",
		Coin:               "2000000000 ETH.USDA",
		FromAddress:        "thor2",
		LiquidityFeeInRune: 1_000,
		Slip:               100,
		ToAddress:          "VAULT",
	}, testdb.AffiliateFee{
		Asset:     "THOR.RUNE",
		FeeAmount: 4 * 1e8,
		Thorname:  "thor1",
	})

	blocks.NewBlock(t, "2020-01-03 13:10:00", testdb.Swap{
		TxID:               "55555",
		Pool:               "ETH.USDB",
		EmitAsset:          "6000000000 ETH.USDB",
		Coin:               "2000000000 THOR.RUNE",
		FromAddress:        "thor2",
		LiquidityFeeInRune: 0,
		Slip:               100,
		ToAddress:          "VAULT",
	}, testdb.AffiliateFee{
		Asset:     "ETH.USDB",
		FeeAmount: 5 * 1e8,
		Thorname:  "thor3",
	})

	from := db.StrToSec("2020-01-03 13:00:00")
	to := db.StrToSec("2020-01-03 15:10:00")
	body := testdb.CallJSON(t,
		fmt.Sprintf("http://localhost:8080/v2/history/affiliate?interval=hour&from=%d&to=%d", from, to))

	var affiliateHistory oapigen.AffiliateHistoryResponse
	testdb.MustUnmarshal(t, body, &affiliateHistory)

	require.Equal(t, "2", affiliateHistory.Meta.Count)
}
