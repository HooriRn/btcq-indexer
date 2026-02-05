package record

import (
	_ "embed"
	"encoding/json"

	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

// This file contains many small independent corrections

const ChainIDMainnet202104 = "thorchain"

func loadMainnet202104Corrections(chainID string) {
	if chainID == ChainIDMainnet202104 {
		log.Info().Msgf(
			"Loading corrections for chaosnet started on 2021-04 id: %s",
			chainID)

		loadMainnetCorrectionsWithdrawImpLoss()
		loadMainnetWithdrawForwardedAssetCorrections()
		loadMainnetWithdrawIncreasesUnits()
		loadMainnetcorrectGenesisNode()
		loadMainnetMissingWithdraws()
		loadMainnetRemoveLiquidityCorrections()
		loadMainnetBalanceCorrections()
		loadMainnetMissingRefund()
		loadMainnetPreregisterThornames()
		loadMainnetTHORNodeInvariats()
		registerArtificialPoolBallanceChanges(
			mainnetArtificialDepthChanges, "Midgard fix on mainnet")
		// Actually block 2104917 (2021-09-15) upon the switch from v0.64.0 to v0.67.0,
		// specifically the implementation of THORNode MR !1834 resolving THORNode Issue #1052.
		withdrawCoinKeptHeight = 1970000
		GlobalWithdrawCorrection = correctWithdawsMainnetFilter

		// This is the block (2023-03-16) upon the switch from v1.106.0 to v1.107.0,
		// specifically the implementation of THORNode MR !2777 resolving THORNode Issue #1415.
		withdrawCoinPooledHeight = 9989661

		// This is the block upon the switch from v3.3.2 to v3.4.0,
		// specifically the implementation of THORNode MR !3964 resolving THORNode Issue #2176.
		undelayedLiquidityFeesHeight = 20515000
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
	reason := []byte(w.Reason)
	chain := strings.Split(w.Pool, ".")[0]

	hashF := fnv.New32a()
	fmt.Fprint(hashF, w.Reason, w.Pool, w.FromAddr, w.RuneE8, w.AssetE8, w.Units)
	txID := strconv.Itoa(int(hashF.Sum32()))

	withdraw := Withdraw{
		FromAddr:    []byte(w.FromAddr),
		Chain:       []byte(chain),
		Pool:        []byte(w.Pool),
		Asset:       []byte("THOR.RUNE"),
		ToAddr:      reason,
		Memo:        reason,
		Tx:          []byte(txID),
		EmitRuneE8:  w.RuneE8,
		EmitAssetE8: w.AssetE8,
		StakeUnits:  w.Units,
	}
	Recorder.OnWithdraw(&withdraw, meta)
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
	corrections := []Withdraw{}
	fn := func(meta *Metadata) {
		for _, c := range corrections {
			withdraw := Withdraw{
				Asymmetry:           0.0,
				BasisPoints:         10000,
				Chain:               []byte("THOR"),
				Pool:                []byte("ETH.HEGIC-0X584BC13C7D411C00C01A62E8019472DE68768430"),
				Asset:               []byte("THOR.RUNE"),
				ToAddr:              []byte(""),
				Memo:                []byte(""),
				Tx:                  []byte("0000000000000000000000000000000000000000000000000000000000000000"),
				EmitRuneE8:          0,
				EmitAssetE8:         0,
				ImpLossProtectionE8: 0,
				FromAddr:            c.FromAddr,
				StakeUnits:          c.StakeUnits,
			}
			Recorder.OnWithdraw(&withdraw, meta)
		}
	}
	AdditionalEvents.Add(7171200, fn)
}

//////////////////////// Fix withdraw assets not forwarded.

// In the early blocks of the chain the asset sent in with the withdraw initiation
// was not forwarded back to the user. This was fixed for later blocks:
//  https://gitlab.com/thorchain/thornode/-/merge_requests/1635

func correctWithdawsForwardedAsset(withdraw *Withdraw, meta *Metadata) KeepOrDiscard {
	withdraw.AssetE8 = 0
	return Keep
}

// generate block heights where this occurred:
//
//	select FORMAT('    %s,', b.height)
//	from withdraw_events as x join block_log as b on x.block_timestamp = b.timestamp
//	where asset_e8 != 0 and asset != 'THOR.RUNE' and b.height < 220000;
func loadMainnetWithdrawForwardedAssetCorrections() {
	var heightWithOldWithdraws []int64 = []int64{}
	for _, height := range heightWithOldWithdraws {
		WithdrawCorrections.Add(height, correctWithdawsForwardedAsset)
	}
}

func correctWithdawsMainnetFilter(withdraw *Withdraw, meta *Metadata) KeepOrDiscard {
	// In the beginning of the chain withdrawing pending liquidity emitted a
	// withdraw event with units=0.
	// This was later corrected, and pending_liquidity events are emitted instead.
	if withdraw.StakeUnits == 0 && meta.BlockHeight < 1000000 {
		return Discard
	}
	return Keep
}

//////////////////////// Follow ThorNode bug on withdraw (units and rune was added to the pool)

// https://gitlab.com/thorchain/thornode/-/issues/954
// ThorNode added units to a member after a withdraw instead of removing.
// The bug was corrected, but an arbitrage account hit this bug 13 times.
//
// The values were generated with cmd/statechecks
// The member address was identified with cmd/membercheck
func loadMainnetWithdrawIncreasesUnits() {
	type MissingAdd struct {
		AdditionalRune  int64
		AdditionalUnits int64
	}
	corrections := map[int64]MissingAdd{}

	correct := func(meta *Metadata) {
		missingAdd := corrections[meta.BlockHeight]
		stake := Stake{
			AddBase: AddBase{
				Pool:     []byte("ETH.ETH"),
				RuneAddr: []byte("thor1hyarrh5hslcg3q5pgvl6mp6gmw92c4tpzdsjqg"),
				RuneE8:   missingAdd.AdditionalRune,
			},
			StakeUnits: missingAdd.AdditionalUnits,
		}
		Recorder.OnStake(&stake, meta)
	}
	for k := range corrections {
		AdditionalEvents.Add(k, correct)
	}
}

var mainnetArtificialDepthChanges = artificialPoolBallanceChanges{}

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
	type Correction struct {
		asset    string
		fromAddr string
		toAddr   string
		amountE8 int64
	}
	heightCorrections := map[int64][]Correction{}
	for height, corrections := range heightCorrections {
		fn := func(meta *Metadata) {
			for _, c := range corrections {
				transfer := Transfer{
					FromAddr: []byte(c.fromAddr),
					ToAddr:   []byte(c.toAddr),
					Asset:    []byte(c.asset),
					AmountE8: c.amountE8,
				}
				Recorder.OnTransfer(&transfer, meta)
			}
		}
		AdditionalEvents.Add(height, fn)
	}
}

//////////////////////// Preregister Thornames

// The pre-registered thornames were loaded directly into the thornode KV in a store
// migration at V88 (height 5531995) and did not properly emit events. These are loaded
// from the configuration found in <thornode>/x/thorchain/preregister_thornames.json.

//go:embed preregister_thornames.json
var preregisterThornamesData []byte

func loadMainnetPreregisterThornames() {
	// unmarshal the configuration
	type preregisterThorname struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	}
	thornames := []preregisterThorname{}
	err := json.Unmarshal(preregisterThornamesData, &thornames)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to unmarshal preregistered thornames")
	}

	// fake an event for each of the preregisterd thornames
	for _, tn := range thornames {
		tnc := tn // copy in scope
		AdditionalEvents.Add(5531995, func(meta *Metadata) {
			thorNameChange := THORNameChange{
				Name:         []byte(tnc.Name),
				Address:      []byte(tnc.Address),
				Owner:        []byte(tnc.Address),
				Chain:        []byte("THOR"),
				ExpireHeight: 10787995,
			}
			Recorder.OnTHORNameChange(&thorNameChange, meta)
		})
	}
}

//////////////////////// THORNode invariants

// see here: https://gitlab.com/thorchain/thornode/-/merge_requests/2814#e751f3a359cd1a5d6a635a4f468271d2b6fe57bf
// fix for synth supply missmatch that happened in v116

func loadMainnetTHORNodeInvariats() {
	burnedSynths := []Burn{}
	for _, burnEvent := range burnedSynths {
		bn := burnEvent
		AdditionalEvents.Add(11782453, func(meta *Metadata) {
			Recorder.OnBurn(&bn, meta)
		})
	}

	mintSynths := []Coinbase{}
	for _, mintEvent := range mintSynths {
		bn := mintEvent
		AdditionalEvents.Add(12145978, func(meta *Metadata) {
			Recorder.OnCoinbase(&bn, meta)
		})
	}
}
