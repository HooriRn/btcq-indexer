// Sometimes ThorNode state is updated but the events doesn't reflect that perfectly.
//
// In these cases we open a bug report so future events are correct, but the old events will
// stay the same, and we apply these corrections to the existing events.
package record

import (
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/fetch/sync/chain"
)

const MidgardBalanceCorrectionAddress = "MidgardBalanceCorrectionAddress"

func LoadCorrections(chainID string) {
	if chainID == "" {
		return
	}

	loadMainnet202104Corrections(chainID)
	loadTestnet202111Corrections(chainID)
	loadStagenetCorrections(chainID)
}

/////////////// Corrections for Missing Events

func AddMissingEvents(meta *Metadata) {
	f, ok := AdditionalEvents[meta.BlockHeight]
	if ok {
		f(meta)
	}
}

type (
	AddEventsFunc    func(meta *Metadata)
	AddEventsFuncMap map[int64]AddEventsFunc
)

var AdditionalEvents = AddEventsFuncMap{}

/////////////// Corrections for Withdraws
// Removed: Withdraw events are no longer tracked

/////////////// Blacklist of fee events
// Removed: Fee events are no longer tracked

/////////////// Artificial deposits to fix member pool units.
// Removed: Stake and Withdraw events are no longer tracked

/////////////// Artificial pool balance changes to fix ThorNode/Midgard depth divergences.
// Removed: PoolBalanceChange events are no longer tracked

/////////////// Old style withdraws

// Logic for withdraw changed since start of chaosnet 2021-04. This variable describes the height
// where the logic change happened.
var withdrawCoinKeptHeight int64 = 0

// In Mainnet later, the logic was changed to add the withdraw coin in the pool,
// rather than in the vault.  This variable describes the height where the logic change happened.
var withdrawCoinPooledHeight int64 = 0

func (m AddEventsFuncMap) Add(height int64, f AddEventsFunc) {
	fOrig, alreadyExists := m[height]
	if alreadyExists {
		m[height] = func(meta *Metadata) {
			fOrig(meta)
			f(meta)
		}
		return
	}
	m[height] = f
}

// Removed: WithdrawCorrectionMap.Add - Withdraw events are no longer tracked

/////////////// Block Corrections

func applyBlockCorrections(block *chain.Block) {
	applyTimestampCorrections(block)
}

/////////////// Timestamp corrections

var TimestampCorrections = map[int64]db.Second{}

func applyTimestampCorrections(block *chain.Block) {
	if sec, ok := TimestampCorrections[block.Height]; ok {
		block.Time = sec.ToTime()
	}
}

/////////////// Undelayed Liquidity Fees on Swap

var undelayedLiquidityFeesHeight int64 = 0
