package record

import (
	"fmt"
	"strconv"
	"strings"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
)

// handle metadata after each insertion
func increaseMetadata(m *Metadata) {
	m.EventId.EventIndex++
}

func recordGenPools(m *Metadata) {
	for _, e := range db.GenesisData.AppState.Thorchain.Pools {

		// pool balance change events
		cols := []string{
			"asset", "rune_amt", "rune_add", "asset_amt", "asset_add", "reason"}
		err := InsertWithMeta("pool_balance_change_events", m, cols,
			e.Asset, e.BalanceRune, true, e.BalanceAsset, true, "genesisAdd")
		if err != nil {
			miderr.LogEventParseErrorF(
				"failed to insert add event from genesis, err: %s", err)
		}

		// pool events - pool status
		cols = []string{"asset", "status"}
		err = InsertWithMeta("pool_events", m, cols, e.Asset, e.Status)
		if err != nil {
			miderr.LogEventParseErrorF("failed to insert pool event from genesis, err: %s", err)
		}

		// recorder
		Recorder.chainInfo.SetPoolStatus(e.Asset, e.Status)
		Recorder.AddPoolAssetE8Depth([]byte(e.Asset), e.BalanceAsset)
		Recorder.AddPoolRuneE8Depth([]byte(e.Asset), e.BalanceRune)

		increaseMetadata(m)
	}
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
				miderr.LogEventParseErrorF("failed to get the account address, index: %d", index)
			}

			for _, c := range b.Coins {
				cols := []string{"from_addr", "to_addr", "asset", "amount_e8"}

				coin, err := parseCosmosDenom(c.Denom)
				if err != nil {
					miderr.LogEventParseErrorF("failed to parse denom from genesis, err: %s", err)
				}

				err = InsertWithMeta("transfer_events", m, cols,
					"genesis", b.Address, coin, c.Amount)
				if err != nil {
					miderr.LogEventParseErrorF(
						"failed to insert transfer event from genesis, err: %s", err)
				}

				increaseMetadata(m)
			}
		}
	}
}

func recordGenLPs(m *Metadata) {
	for _, e := range db.GenesisData.AppState.Thorchain.LPs {
		// pending liquidity events
		if e.PendingAsset > 0 || e.PendingRune > 0 {
			cols := []string{
				"pool", "asset_tx", "asset_chain", "asset_addr", "asset_e8",
				"rune_tx", "rune_addr", "rune_e8",
				"pending_type"}
			err := InsertWithMeta("pending_liquidity_events", m, cols, e.Pool,
				"genesisTx", util.AssetFromString(e.Pool).Chain, util.WrapString(e.AssetAddr), e.AssetE8,
				"genesisTx", util.WrapString(e.RuneAddr), e.RuneE8, "add")

			if err != nil {
				miderr.LogEventParseErrorF(
					"failed to insert pending liquidity event, err: %s", err)
			}
		}

		// stake events
		if e.Units > 0 {
			aE8, rE8, _ := Recorder.CurrentDepths([]byte(e.Pool))
			var assetInRune int64
			if aE8 != 0 {
				assetInRune = int64(float64(e.AssetE8)*(float64(rE8)/float64(aE8)) + 0.5)
			}

			cols := []string{
				"pool", "asset_tx", "asset_chain",
				"asset_addr", "asset_e8", "stake_units", "rune_tx", "rune_addr", "rune_e8",
				"_asset_in_rune_e8"}
			err := InsertWithMeta(
				"stake_events", m, cols,
				e.Pool, "genesisTx", util.AssetFromString(e.Pool).Chain,
				util.WrapString(e.AssetAddr), e.AssetE8, e.Units, "genesisTx", util.WrapString(e.RuneAddr), e.RuneE8,
				assetInRune)

			if err != nil {
				miderr.LogEventParseErrorF("failed to insert stake event, err: %s", err)
			}
		}

		increaseMetadata(m)
	}
}

func recordGenTHORNames(m *Metadata) {
	// thorname events
	for _, e := range db.GenesisData.AppState.Thorchain.THORNames {
		for _, a := range e.Aliases {
			if config.Global.CaseInsensitiveChains[string(a.Chain)] {
				a.Address = strings.ToLower(a.Address)
			}

			cols := []string{
				"name", "chain", "address", "registration_fee_e8", "fund_amount_e8", "expire", "owner"}
			err := InsertWithMeta("thorname_change_events", m, cols,
				e.Name, a.Chain, a.Address, 0, 0, e.ExpireBlockHeight, e.Owner)
			if err != nil {
				miderr.LogEventParseErrorF("failed to insert thorname change event, err: %s", err)
			}

			increaseMetadata(m)
		}
	}
}

func recordGenNodes(m *Metadata) {
	for _, e := range db.GenesisData.AppState.Thorchain.Nodes {
		// node events
		cols := []string{"node_addr", "former", "current"}
		err := InsertWithMeta("update_node_account_status_events", m, cols,
			e.NodeAddress, []byte{}, e.Status)
		if err != nil {
			miderr.LogEventParseErrorF("failed to insert node account status event, err: %s", err)
		}

		// bond events
		cols = []string{
			"tx", "chain", "from_addr", "to_addr", "asset", "asset_e8", "memo", "bond_type", "e8"}
		err = InsertWithMeta("bond_events", m, cols,
			"genesisTx", "THOR", e.BondAddr, "", "THOR.RUNE", 0, "", "bond_paid", e.BondE8)
		if err != nil {
			miderr.LogEventParseErrorF("failed to insert bond event, err: %s", err)
		}

		increaseMetadata(m)
	}
}

func recordGenLoans(m *Metadata) {
	for _, e := range db.GenesisData.AppState.Thorchain.Loans {
		// loan open events
		cols := []string{"owner", "collateral_deposited", "debt_issued",
			"collateralization_ratio", "collateral_asset", "target_asset"}

		tm := m
		tm.BlockHeight = e.LastOpenHeight
		tm.EventId = db.EventId{
			BlockHeight: e.LastOpenHeight,
		}

		err := InsertWithMeta("loan_open_events", m, cols,
			e.Owner, e.CollateralDeposited, e.DebtIssued, 0, e.Asset, "")
		if err != nil {
			miderr.LogEventParseErrorF("failed to insert loan open event, err: %s", err)
		}

		// loan repayment events
		if e.CollateralWithdrawn > 0 || e.DebtRepaid > 0 {
			cols = []string{"owner", "collateral_withdrawn", "debt_repaid", "collateral_asset"}
			err = InsertWithMeta("loan_repayment_events", m, cols,
				e.Owner, e.CollateralWithdrawn, e.DebtRepaid, e.Asset)
			if err != nil {
				miderr.LogEventParseErrorF("failed to insert loan repayment event, err: %s", err)
			}
		}

		increaseMetadata(m)
	}
}

// TODO(HooriRn): check if the mimir memory snapshot works with this
func recordGenMimirs(m *Metadata) {
	// mimir events
	for _, e := range db.GenesisData.AppState.Thorchain.Mimirs {
		cols := []string{"key", "value"}
		err := InsertWithMeta("set_mimir_events", m, cols, e.Key, strconv.FormatInt(e.Value, 10))
		if err != nil {
			miderr.LogEventParseErrorF("failed to insert mimir event, err: %s", err)
		}

		Recorder.chainInfo.SetMimirStatus(e.Key, e.Value)
		increaseMetadata(m)
	}
}
