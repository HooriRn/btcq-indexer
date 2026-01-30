package timeseries_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/internal/fetch/notinchain"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func TestNetworkAPY(t *testing.T) {
	defer testdb.StartMockThornode()()
	blocks := testdb.InitTestBlocks(t)

	// Active bond amount = 1500 rune
	testdb.RegisterThornodeNodes([]notinchain.NodeAccount{
		{Status: "Active", TotalBond: 1500},
		{Status: "Standby", TotalBond: 123}})

	// reserve=5200
	// blocks per year = 520 (10 weekly)
	// emission curve = 2
	// rewards per block: 5200 / (520 * 2) = 5
	testdb.RegisterThornodeReserve(notinchain.Network{
		TotalReserve:          5200,
		EffectiveSecurityBond: 1500,
	})

	blocks.NewBlock(t, "2020-09-01 00:00:00",
		testdb.SetMimir{Key: "EmissionCurve", Value: 2},
		testdb.SetMimir{Key: "BlocksPerYear", Value: 520},
		testdb.SetMimir{Key: "IncentiveCurve", Value: 2},
		testdb.SetMimir{Key: "PendulumUseEffectiveSecurity", Value: 0},
		testdb.SetMimir{Key: "PendulumUseVaultAssets", Value: 0},
		testdb.SetMimir{Key: "PendulumAssetsBasisPoints", Value: 10_000},

		testdb.AddLiquidity{Pool: "BNB.TWT-123", AssetAmount: 550, RuneAmount: 910},
		testdb.PoolActivate("BNB.TWT-123"),
	)

	blocks.NewBlock(t, "2020-09-01 00:10:00",
		testdb.Swap{
			Pool:               "BNB.TWT-123",
			Coin:               "100 THOR.RUNE",
			EmitAsset:          "50 BNB.TWT-123",
			LiquidityFeeInRune: 10,
		},
	)

	testdb.RegisterThornodeVault([]notinchain.AsgardVaults{
		{
			Status: "ActiveVault",
			Coins: []notinchain.Coin{
				{
					Asset:  "BNB.TWT-123",
					Amount: "500",
				},
			},
		},
	})
	// Final depths: Rune = 1000 (900 + 100) ; Asset = 500 (550 - 50)
	// LP pooled amount is considered 2000 (double the rune amount)

	body := testdb.CallJSON(t, "http://localhost:8080/v2/network")

	var jsonApiResult oapigen.Network
	testdb.MustUnmarshal(t, body, &jsonApiResult)

	require.Equal(t, "1", jsonApiResult.ActiveNodeCount)
	require.Equal(t, "1", jsonApiResult.StandbyNodeCount)
	require.Equal(t, "1500", jsonApiResult.BondMetrics.TotalActiveBond)
	require.Equal(t, "123", jsonApiResult.BondMetrics.TotalStandbyBond)
	require.Equal(t, "1500", jsonApiResult.BondMetrics.BondHardCap)
	require.Equal(t, "5200", jsonApiResult.TotalReserve)
	require.Equal(t, "1000", jsonApiResult.TotalPooledRune)

	require.Equal(t, "5", jsonApiResult.BlockRewards.BlockReward)

	// baseNodeShare = 10, basePoolShare = 5, TotalReward = 15
	// adjustmentNodeShare = (totalEffective / securityBond), adjustmentPoolShare = (availablePoolsRune / vaultsLiquidity)
	// adjustmentNodeShare = 10, adjustmentPoolShare = 5
	// poolShareFactor = adjustmentPoolShare / (adjustmentPoolShare + adjustmentNodeShare) = 0.3333333333333333
	require.Equal(t, "0.3333333333333333", jsonApiResult.PoolShareFactor)

	// Weekly income = 60 (block reward * weekly blocks + liquidity fees)
	// LP earning weekly = 20 (60 * 0.3333333333333333)
	// LP weekly yield = 1% (weekly earning / 2*rune depth = 20 / 2*1000)
	// LP cumulative yearly yield ~ 67.76% ( 1.01 ** 52)
	require.Contains(t, jsonApiResult.LiquidityAPY, "0.677688921462944")

	// Bonding earning = 40 (60 * 0.6666666666666666)
	// Bonding weekly yield = 2.666% (weekly earning / active bond = 40 / 1500)
	// Bonding cumulative yearly yield ~ 292.945% ( 1.0266666666666666 ** 52)
	require.Contains(t, jsonApiResult.BondingAPY, "2.92945222220460")
}

func TestNetworkNextChurnHeight(t *testing.T) {
	defer testdb.StartMockThornode()()
	blocks := testdb.InitTestBlocks(t)

	// ChurnInterval = 20 ; ChurnRetryInterval = 10
	blocks.NewBlock(t, "2020-09-01 00:00:00",
		testdb.SetMimir{Key: "ChurnInterval", Value: 20},
		testdb.SetMimir{Key: "ChurnRetryInterval", Value: 10},
		testdb.SetMimir{Key: "PendulumUseEffectiveSecurity", Value: 0},
		testdb.SetMimir{Key: "PendulumUseVaultAssets", Value: 0},
		testdb.SetMimir{Key: "PendulumAssetsBasisPoints", Value: 10_000},
	)

	// Last churn at block 2
	blocks.NewBlock(t, "2020-09-01 00:10:00", testdb.ActiveVault{AddVault: "addr"})

	body := testdb.CallJSON(t, "http://localhost:8080/v2/network")
	var result oapigen.Network
	testdb.MustUnmarshal(t, body, &result)

	require.Equal(t, "22", result.NextChurnHeight)

	blocks.EmptyBlocksBefore(t, 23) // Churn didn't happen at block 22

	body = testdb.CallJSON(t, "http://localhost:8080/v2/network")
	testdb.MustUnmarshal(t, body, &result)

	require.Equal(t, "32", result.NextChurnHeight)
}

func TestNetworkPoolCycle(t *testing.T) {
	defer testdb.StartMockThornode()()
	blocks := testdb.InitTestBlocks(t)

	// PoolCycle = 10
	blocks.NewBlock(t, "2020-09-01 00:00:00",
		testdb.SetMimir{Key: "PoolCycle", Value: 10},
		testdb.SetMimir{Key: "PendulumUseEffectiveSecurity", Value: 0},
		testdb.SetMimir{Key: "PendulumUseVaultAssets", Value: 0},
		testdb.SetMimir{Key: "PendulumAssetsBasisPoints", Value: 10_000},
	)

	// last block = 13
	blocks.EmptyBlocksBefore(t, 14)

	body := testdb.CallJSON(t, "http://localhost:8080/v2/network")
	var result oapigen.Network
	testdb.MustUnmarshal(t, body, &result)
	require.Equal(t, "7", result.PoolActivationCountdown)
}

func TestSetNodeMimir(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	// Set initial Mimir values
	blocks.NewBlock(t, "2020-09-01 00:00:00",
		testdb.UpdateNodeAccountStatus{
			NodeAddr: "node1", Former: "Standby", Current: "Active",
		},
		testdb.UpdateNodeAccountStatus{
			NodeAddr: "node2", Former: "Standby", Current: "Active",
		},
	)

	blocks.NewBlock(t, "2020-09-02 00:00:00",
		testdb.SetNodeMimir{
			Key: "NodeMimirKey1", Value: 100, Address: "node1",
		},
	)

	blocks.NewBlock(t, "2020-09-02 00:01:00",
		testdb.SetNodeMimir{
			Key: "NodeMimirKey1", Value: 200, Address: "node2",
		},
	)

	body := testdb.CallJSON(t, "http://localhost:8080/v2/votes")
	var result oapigen.VotesResponse
	testdb.MustUnmarshal(t, body, &result)

	require.Equal(t, 2, len(result[0].Votes))
	require.Equal(t, "NodeMimirKey1", result[0].Value)
}
