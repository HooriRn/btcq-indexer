package record

import (
	"github.com/rs/zerolog/log"
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
	// Correction data emptied; functionality preserved for future use.
}

func loadTestnetTimestampCorrections() {
	// Correction data emptied; functionality preserved for future use.
}
