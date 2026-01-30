package record

import (
	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/midgard/internal/db"
)

// Testnet started on 2021-11-06
const ChainIDTestnet20211106 = "thorchain-testnet-v0"

// ThorNode state and events diverged on testnet. We apply all these changes to be in sync with
// Thornode.
func loadTestnet202111Corrections(chainID string) {
	if chainID == ChainIDTestnet20211106 {
		log.Info().Msgf(
			"Loading corrections for testnet started on 2021-11-06 id: %s",
			chainID)

		loadTestnetMissingWithdraws()
		loadTestnetTimestampCorrections()
	}
}

//////////////////////// Missing withdraws

func loadTestnetMissingWithdraws() {
	// On Pool suspension the withdraws had FromAddr=null and they were skipped by Midgard.
	// Later the pool was reactivated, so having correct units is important even at suspension.
	// There is a plan to fix ThorNode events:
	// https://gitlab.com/thorchain/thornode/-/issues/1164
	addWithdraw(10000, AdditionalWithdraw{
		Pool:     "BNB.BUSD-74E",
		FromAddr: "tthor1qkd5f9xh2g87wmjc620uf5w08ygdx4etu0u9fs",
		Reason:   "midgard correction suspended pool withdraws missing",
		RuneE8:   0,
		AssetE8:  0,
		Units:    10000000000,
	})

	addWithdraw(222784, AdditionalWithdraw{
		Pool:     "BNB.BNB",
		FromAddr: "tbnb1yc20slera2g4fhnkkyttqxf70qxa4jtm42qq4t",
		Reason:   "midgard correction",
		RuneE8:   294194696841,
		AssetE8:  106918851,
		Units:    170138465261,
	})
}

func loadTestnetTimestampCorrections() {
	// Testnet torchain-v1 genesis block at height[1276572]
	// received the timestamp of block height[1] of the
	// previous testnet thorchain, causing a timestamp collision.
	TimestampCorrections[1276572] = db.StrToSec("2022-02-03 19:06:23")
}
