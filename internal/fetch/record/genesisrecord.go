package record

import (
	"fmt"
	"strings"

	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/util"
	"github.com/btcq/btcq-indexer/internal/util/btcqerr"
)

// handle metadata after each insertion
func increaseMetadata(m *Metadata) {
	m.EventId.EventIndex++
}

func recordGenPools(m *Metadata) {
	// No thorchain genesis data for this chain.
}

func recordGenSupplies(m *Metadata) {
	for _, e := range db.GenesisData.AppState.Bank.Supplies {
		poolName := strings.ToUpper(e.Denom)
		if util.AssetFromString(e.Denom).Synth {
			poolName = util.ConvertSynthPoolToNative(poolName)
		}

		Recorder.AddPoolSynthE8Depth([]byte(poolName), e.Amount)
	}
}

func parseCosmosDenom(b string) (asset string, err error) {
	switch denom := string(b); denom {
	case "":
		err = fmt.Errorf("no units given in amount %q", b)
		return
	case "rune":
		asset = nativeRune
	default:
		asset = strings.ToUpper(denom)
	}

	return asset, nil
}

func recordGenTransfers(m *Metadata) {
	if config.Global.EventRecorder.OnTransferEnabled { // check with the config
		for index, b := range db.GenesisData.AppState.Bank.Balances {
			if b.Address == "" {
				btcqerr.LogEventParseErrorF("failed to get the account address, index: %d", index)
			}

			for _, c := range b.Coins {
				cols := []string{"from_addr", "to_addr", "asset", "amount_e8"}

				coin, err := parseCosmosDenom(c.Denom)
				if err != nil {
					btcqerr.LogEventParseErrorF("failed to parse denom from genesis, err: %s", err)
				}

				err = InsertWithMeta("transfer_events", m, cols,
					"genesis", b.Address, coin, c.Amount)
				if err != nil {
					btcqerr.LogEventParseErrorF(
						"failed to insert transfer event from genesis, err: %s", err)
				}

				increaseMetadata(m)
			}
		}
	}
}

func recordGenLPs(m *Metadata) {
	// No thorchain genesis data for this chain.
}

func recordGenTHORNames(m *Metadata) {
	// No thorchain genesis data for this chain.
}

func recordGenNodes(m *Metadata) {
	// No thorchain genesis data for this chain.
}

func recordGenLoans(m *Metadata) {
	// No thorchain genesis data for this chain.
}

func recordGenMimirs(m *Metadata) {
	// No thorchain genesis data for this chain.
}
