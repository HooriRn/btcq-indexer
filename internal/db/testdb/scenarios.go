package testdb

import (
	"testing"
)

// A test scenario with 10 swaps
// 4x rune -> asset: volume 40, fee 4, slip 5
// 3x asset -> rune: volume 3000 + 300, fee 300, slip 6
// 2x rune -> synth: volume 200000, fee 20000, slip 7
// 1x synth -> rune: volume 10000000 + 1000000, fee 1000000, slip 8
func ScenarioTenSwaps(t *testing.T, blocks *blockCreator) {
	blocks.NewBlock(t, "2010-01-01 00:00:00",
		AddLiquidity{
			Pool:        "BTC.BTC",
			RuneAddress: "thoraddr1",
			AssetAmount: 1000000,
			RuneAmount:  10000000,
		},
		PoolActivate("BTC.BTC"),
	)

	// 4x rune -> asset
	blocks.NewBlock(t, "2020-01-01 00:01:00",
		Swap{
			Pool:               "BTC.BTC",
			Coin:               "10 THOR.RUNE",
			EmitAsset:          "1 BTC.BTC",
			LiquidityFeeInRune: 1,
			Slip:               5,
		},
		Swap{
			Pool:               "BTC.BTC",
			Coin:               "10 THOR.RUNE",
			EmitAsset:          "1 BTC.BTC",
			LiquidityFeeInRune: 1,
			Slip:               5,
		},
		Swap{
			Pool:               "BTC.BTC",
			Coin:               "10 THOR.RUNE",
			EmitAsset:          "1 BTC.BTC",
			LiquidityFeeInRune: 1,
			Slip:               5,
		},
		Swap{
			Pool:               "BTC.BTC",
			Coin:               "10 THOR.RUNE",
			EmitAsset:          "1 BTC.BTC",
			LiquidityFeeInRune: 1,
			Slip:               5,
		})

	// 3x asset -> rune
	blocks.NewBlock(t, "2020-01-01 00:02:00",
		Swap{
			Pool:               "BTC.BTC",
			Coin:               "100 BTC.BTC",
			EmitAsset:          "1000 THOR.RUNE",
			LiquidityFeeInRune: 100,
			LiquidityFee:       10,
			Slip:               6,
		},
		Swap{
			Pool:               "BTC.BTC",
			Coin:               "100 BTC.BTC",
			EmitAsset:          "1000 THOR.RUNE",
			LiquidityFeeInRune: 100,
			LiquidityFee:       10,
			Slip:               6,
		},
		Swap{
			Pool:               "BTC.BTC",
			Coin:               "100 BTC.BTC",
			EmitAsset:          "1000 THOR.RUNE",
			LiquidityFeeInRune: 100,
			LiquidityFee:       10,
			Slip:               6,
		})

	// 2x rune -> synth
	blocks.NewBlock(t, "2020-01-01 00:03:00",
		Swap{
			Pool:               "BTC.BTC",
			Coin:               "100000 THOR.RUNE",
			EmitAsset:          "10000 BTC/BTC",
			LiquidityFeeInRune: 100,
			Slip:               7,
		},
		Swap{
			Pool:               "BTC.BTC",
			Coin:               "100000 THOR.RUNE",
			EmitAsset:          "10000 BTC/BTC",
			LiquidityFeeInRune: 100,
			Slip:               7,
		})

	// 1x synth -> rune
	blocks.NewBlock(t, "2020-01-01 00:04:00",
		Swap{
			Pool:               "BTC.BTC",
			Coin:               "1000000 BTC/BTC",
			EmitAsset:          "10000000 THOR.RUNE",
			LiquidityFeeInRune: 100,
			Slip:               8,
		},
	)

}
