package record

import (
	_ "embed"

	"github.com/rs/zerolog/log"
)

// This file contains many small independent corrections

const ChainIDMainnet202104 = "thorchain"

func loadMainnet202104Corrections(chainID string) {
	if chainID == ChainIDMainnet202104 {
		log.Info().Msgf(
			"Loading corrections for chaosnet started on 2021-04 id: %s",
			chainID)

		// Removed: All corrections for deleted event types (Withdraw, Stake, Transfer, THORNameChange, Burn, Coinbase, PoolBalanceChange)
		// Only rewards and cosm wasm events are now tracked
	}
}

//////////////////////// Activate genesis node.

// Genesis node bonded rune and became listed as Active without any events.
func loadMainnetcorrectGenesisNode() {
	// Correction data emptied; functionality preserved for future use.
}

//////////////////////// Missing Withdraws

type AdditionalWithdraw struct {
	Pool     string
	FromAddr string
	Reason   string
	RuneE8   int64
	AssetE8  int64
	Units    int64
}

func (w *AdditionalWithdraw) Record(meta *Metadata) {
	// Removed: Withdraw events are no longer tracked
	_ = w // suppress unused variable warning
	_ = meta
}

func addWithdraw(height int64, w AdditionalWithdraw) {
	AdditionalEvents.Add(height, w.Record)
}

func loadMainnetMissingWithdraws() {
	// Correction data emptied; functionality preserved for future use.
}

//////////////////////// Missing Swap Refund

// https://gitlab.com/thorchain/thornode/-/merge_requests/2716
// tried to refund ETH asset that never went out by sending RUNE back to the user from Asgard Module.
// Swap event handling removed; correction skipped.
func loadMainnetMissingRefund() {}

//////////////////////// Fix HEGIC pool missbalance.

// When pool gets removed by thornode `removeLiquidityProviders` the event contains null value
// And gets ignored by Midgard. Here is the fix from thornode:
// https://gitlab.com/thorchain/thornode/-/merge_requests/2819

func loadMainnetRemoveLiquidityCorrections() {
	// Removed: Withdraw events are no longer tracked
}

//////////////////////// Fix withdraw assets not forwarded.

// In the early blocks of the chain the asset sent in with the withdraw initiation
// was not forwarded back to the user. This was fixed for later blocks:
//  https://gitlab.com/thorchain/thornode/-/merge_requests/1635

func correctWithdawsForwardedAsset(meta *Metadata) {
	// Removed: Withdraw events are no longer tracked
}

func loadMainnetWithdrawForwardedAssetCorrections() {
	// Removed: Withdraw events are no longer tracked
}

func correctWithdawsMainnetFilter(meta *Metadata) {
	// Removed: Withdraw events are no longer tracked
}

//////////////////////// Follow ThorNode bug on withdraw (units and rune was added to the pool)

// https://gitlab.com/thorchain/thornode/-/issues/954
// ThorNode added units to a member after a withdraw instead of removing.
// The bug was corrected, but an arbitrage account hit this bug 13 times.
//
// The values were generated with cmd/statechecks
// The member address was identified with cmd/membercheck
func loadMainnetWithdrawIncreasesUnits() {
	// Removed: Stake events are no longer tracked
}

// Removed: artificialPoolBallanceChanges - PoolBalanceChange events are no longer tracked

//////////////////////// Balance corrections

// Due to missing transfer events, account balances diverged compared to thornode.
// These synthethic correction transfers generate compensating transfers from or to
// the midgard-balance-correction-address.
//
// The corrections are not precise, for simplicity were set to the first fork height 4786560,
// except in the case of genesis BaseAccount set to height 1.
//
// Generated with cmd/checks/balance/balancecheck.go
func loadMainnetBalanceCorrections() {
	// Removed: Transfer events are no longer tracked
}

//////////////////////// Preregister Thornames

// The pre-registered thornames were loaded directly into the thornode KV in a store
// migration at V88 (height 5531995) and did not properly emit events. These are loaded
// from the configuration found in <thornode>/x/thorchain/preregister_thornames.json.

//go:embed preregister_thornames.json
var preregisterThornamesData []byte

func loadMainnetPreregisterThornames() {
	// Removed: THORNameChange events are no longer tracked
}

//////////////////////// THORNode invariants

// see here: https://gitlab.com/thorchain/thornode/-/merge_requests/2814#e751f3a359cd1a5d6a635a4f468271d2b6fe57bf
// fix for synth supply missmatch that happened in v116

func loadMainnetTHORNodeInvariats() {
	// Removed: Burn and Coinbase events are no longer tracked
}
