package record

import (
	"encoding/json"
	"strings"

	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/util"
	"github.com/btcq/btcq-indexer/internal/util/btcqerr"
)

func AddressIsRune(address string) bool {
	return (strings.HasPrefix(address, "thor") ||
		strings.HasPrefix(address, "tthor") ||
		strings.HasPrefix(address, "sthor"))
}

// Empty prevents the SQL driver from writing NULL values.
var empty = []byte{}

// Recorder gets initialised by Setup.
var Recorder = &eventRecorder{
	runningTotals: *newRunningTotals(),
	chainInfo:     *newChainInfo(),
}

type eventRecorder struct {
	runningTotals
	chainInfo
}

func InsertWithMeta(table string, meta *Metadata, cols []string, values ...interface{}) error {
	cols = append(cols, "event_id", "block_timestamp")
	values = append(values, meta.EventId.AsBigint(), meta.BlockTimestamp.UnixNano())
	return db.Inserter.Insert(table, cols, values...)
}

func (*eventRecorder) OnBond(e *Bond, meta *Metadata) {
	if e.Memo == nil {
		e.Memo = empty
	}
	if e.Tx == nil {
		e.Tx = empty
	}
	txType := util.TxTypeFromMemo(string(e.Memo))
	cols := []string{
		"tx", "chain", "from_addr", "to_addr", "asset", "asset_e8", "memo", "bond_type", "e8", "bond_addr",
		"node_addr", "signer_addr", "_tx_type"}
	err := InsertWithMeta("bond_events", meta, cols,
		e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.Asset, e.AssetE8, e.Memo, e.BondType, e.E8, e.BondAddr,
		e.NodeAddr, e.SignerAddr, txType)
	if err != nil {
		btcqerr.LogEventParseErrorF("bond event from height %d lost on %s", meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnErrata(e *Errata, meta *Metadata) {
	cols := []string{"in_tx", "asset", "asset_e8", "rune_e8"}
	err := InsertWithMeta("errata_events", meta, cols,
		e.InTx, e.Asset, e.AssetE8, e.RuneE8)
	if err != nil {
		btcqerr.LogEventParseErrorF("errata event from height %d lost on %s", meta.BlockHeight, err)
		return
	}

	r.AddPoolAssetE8Depth(e.Asset, e.AssetE8)
	r.AddPoolRuneE8Depth(e.Asset, e.RuneE8)
}

func (r *eventRecorder) OnFee(e *Fee, meta *Metadata) {
	cols := []string{"tx", "asset", "asset_e8", "pool_deduct"}
	err := InsertWithMeta("fee_events", meta, cols,
		e.Tx, e.Asset, e.AssetE8, e.PoolDeduct)
	if err != nil {
		btcqerr.LogEventParseErrorF("fee event from height %d lost on %s", meta.BlockHeight, err)
	}

	// NOTE: Fee applies to an outbound transaction amount and
	// is then sent to the reserve, so the pool is not involved in principle.
	// However, when the outbound amount is not RUNE,
	// the asset amount correspoinding to the fee is left in its pool,
	// and the RUNE equivalent is deducted from the pool's RUNE and sent to the reserve
	coinType := GetCoinType(e.Asset)
	pool := GetNativeAsset(e.Asset)
	if !IsRune(e.Asset) {
		if coinType == AssetNative {
			r.AddPoolAssetE8Depth(pool, e.AssetE8)
		}
		if coinType == AssetSynth {
			r.AddPoolSynthE8Depth(pool, -e.AssetE8)
		}
		r.AddPoolRuneE8Depth(pool, -e.PoolDeduct)
	}
}

func (r *eventRecorder) OnGas(e *Gas, meta *Metadata) {
	cols := []string{"asset", "asset_e8", "rune_e8", "tx_count"}
	err := InsertWithMeta("gas_events", meta, cols,
		e.Asset, e.AssetE8, e.RuneE8, e.TxCount)
	if err != nil {
		btcqerr.LogEventParseErrorF("gas event from height %d lost on %s", meta.BlockHeight, err)
		return
	}

	r.AddPoolAssetE8Depth(e.Asset, -e.AssetE8)
	r.AddPoolRuneE8Depth(e.Asset, e.RuneE8)
}

func (r *eventRecorder) OnPool(e *Pool, meta *Metadata) {
	cols := []string{"asset", "status"}
	err := InsertWithMeta("pool_events", meta, cols, e.Asset, e.Status)
	if err != nil {
		btcqerr.LogEventParseErrorF("pool event from height %d lost on %s", meta.BlockHeight, err)
	}
	if strings.ToLower(string(e.Status)) == "suspended" {
		pool := string(e.Asset)
		r.SetAssetDepth(pool, 0)
		r.SetRuneDepth(pool, 0)
		r.SetSynthDepth(pool, 0)
	}

	r.chainInfo.SetPoolStatus(string(e.Asset), string(e.Status))
}

func (*eventRecorder) OnReserve(e *Reserve, meta *Metadata) {
	if e.Memo == nil {
		e.Memo = empty
	}
	txType := util.TxTypeFromMemo(string(e.Memo))

	cols := []string{
		"tx", "chain", "from_addr", "to_addr", "asset", "asset_e8", "memo", "addr", "e8", "_tx_type"}
	err := InsertWithMeta("reserve_events", meta, cols,
		e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.Asset, e.AssetE8, e.Memo, e.Addr, e.E8, txType)

	if err != nil {
		btcqerr.LogEventParseErrorF("reserve event from height %d lost on %s", meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnRewards(e *Rewards, meta *Metadata) {
	cols := []string{"bond_e8", "validator"}
	err := InsertWithMeta("rewards_events", meta, cols, e.BondE8, e.Validator)
	if err != nil {
		btcqerr.LogEventParseErrorF("reserve event from height %d lost on %s", meta.BlockHeight, err)
		return
	}

	if len(e.PerPool) == 0 {
		return
	}

	cols2 := []string{"pool", "rune_e8", "saver_e8"}
	for _, p := range e.PerPool {
		err := InsertWithMeta("rewards_event_entries", meta, cols2, p.Asset, p.E8, 0)
		if err != nil {
			btcqerr.LogEventParseErrorF(
				"reserve event pools from height %d lost on %s",
				meta.BlockHeight, err)
			return
		}
	}

	for _, a := range e.PerPool {
		r.AddPoolRuneE8Depth(a.Asset, a.E8)
	}
}

func (*eventRecorder) OnSetIPAddress(e *SetIPAddress, meta *Metadata) {
	cols := []string{"node_addr", "ip_addr"}
	err := InsertWithMeta("set_ip_address_events", meta, cols, e.NodeAddr, e.IPAddr)

	if err != nil {
		btcqerr.LogEventParseErrorF(
			"set_ip_address event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnSetNodeKeys(e *SetNodeKeys, meta *Metadata) {
	cols := []string{"node_addr", "secp256k1", "ed25519", "validator_consensus"}
	err := InsertWithMeta("set_node_keys_events", meta, cols,
		e.NodeAddr, string(e.Secp256k1), string(e.Ed25519), e.ValidatorConsensus)

	if err != nil {
		btcqerr.LogEventParseErrorF(
			"set_node_keys event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnSetVersion(e *SetVersion, meta *Metadata) {
	cols := []string{"node_addr", "version"}
	err := InsertWithMeta("set_version_events", meta, cols, e.NodeAddr, e.Version)
	if err != nil {
		btcqerr.LogEventParseErrorF("set_version event from height %d lost on %s", meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnSlash(e *Slash, meta *Metadata) {
	if len(e.Amounts) == 0 {
		btcqerr.LogEventParseErrorF("slash event on pool %q ignored: zero amounts", e.Pool)
	}
	for _, a := range e.Amounts {
		cols := []string{"pool", "asset", "asset_e8"}
		err := InsertWithMeta("slash_events", meta, cols, e.Pool, a.Asset, a.E8)
		if err != nil {
			btcqerr.LogEventParseErrorF("slash event from height %d lost on %s", meta.BlockHeight, err)
		}
		coinType := GetCoinType(a.Asset)
		switch coinType {
		case Rune:
			r.AddPoolRuneE8Depth(e.Pool, a.E8)
		case AssetNative:
			r.AddPoolAssetE8Depth(e.Pool, a.E8)
		default:
			btcqerr.LogEventParseErrorF("Unhandled slash coin type: %s", a.Asset)
		}
	}
}

// OnStake is kept for corrections/historical data fixes
func (r *eventRecorder) OnStake(e *Stake, meta *Metadata) {
	// TODO(muninn): Separate this side data calculation from the sync process.
	aE8, rE8, _ := r.CurrentDepths(e.Pool)
	aE8 += e.AssetE8
	rE8 += e.RuneE8
	var assetInRune int64
	if aE8 != 0 {
		assetInRune = int64(float64(e.AssetE8)*(float64(rE8)/float64(aE8)) + 0.5)
	}
	cols := []string{
		"pool", "asset_tx", "asset_chain",
		"asset_addr", "asset_e8", "stake_units", "rune_tx", "rune_addr", "rune_e8",
		"_asset_in_rune_e8", "memo"}
	err := InsertWithMeta(
		"stake_events", meta, cols,
		e.Pool, e.AssetTx, e.AssetChain,
		e.AssetAddr, e.AssetE8, e.StakeUnits, e.RuneTx, e.RuneAddr, e.RuneE8,
		assetInRune, e.Memo)
	if err != nil {
		btcqerr.LogEventParseErrorF("stake event from height %d lost on %s", meta.BlockHeight, err)
		return
	}

	r.AddPoolAssetE8Depth(e.Pool, e.AssetE8)
	r.AddPoolRuneE8Depth(e.Pool, e.RuneE8)
}

func (*eventRecorder) OnTransfer(e *Transfer, meta *Metadata) {
	if !config.Global.EventRecorder.OnTransferEnabled {
		return
	}

	cols := []string{"from_addr", "to_addr", "asset", "amount_e8"}
	err := InsertWithMeta("transfer_events", meta, cols,
		e.FromAddr, e.ToAddr, e.Asset, e.AmountE8)

	if err != nil {
		btcqerr.LogEventParseErrorF(
			"transfer event from height %d lost on %s",
			meta.BlockHeight, err)
		return
	}
}

func (r *eventRecorder) OnWithdraw(e *Withdraw, meta *Metadata) {
	// TODO(muninn): Separate this side data calculation from the sync process.
	aE8, rE8, _ := r.CurrentDepths(e.Pool)
	var emitAssetInRune int64
	if aE8 != 0 {
		emitAssetInRune = int64(float64(e.EmitAssetE8)*(float64(rE8)/float64(aE8)) + 0.5)
	}
	// there have been events from thornode with attribute `to` as null
	// mostly in pool cycle, for pool removal. although it's fixed on the newer versions
	// this check make sure that the eailer versions will make the pools membership correct.
	if e.ToAddr == nil {
		e.ToAddr = []byte("")
	}

	txType := util.TxTypeFromMemo(string(e.Memo))

	cols := []string{
		"tx", "chain", "from_addr", "to_addr", "asset",
		"asset_e8", "emit_asset_e8", "emit_rune_e8",
		"memo", "pool", "stake_units", "basis_points", "asymmetry", "imp_loss_protection_e8",
		"_emit_asset_in_rune_e8", "_tx_type"}
	err := InsertWithMeta(
		"withdraw_events", meta, cols,
		e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.Asset,
		e.AssetE8, e.EmitAssetE8, e.EmitRuneE8,
		e.Memo, e.Pool, e.StakeUnits, e.BasisPoints, e.Asymmetry, e.ImpLossProtectionE8,
		emitAssetInRune, txType)

	if err != nil {
		btcqerr.LogEventParseErrorF("withdraw event from height %d lost on %s", meta.BlockHeight, err)
	}
	// Rune/Asset withdrawn from pool
	r.AddPoolAssetE8Depth(e.Pool, -e.EmitAssetE8)
	r.AddPoolRuneE8Depth(e.Pool, -e.EmitRuneE8)

	// Rune added to pool from reserve as impermanent loss protection
	r.AddPoolRuneE8Depth(e.Pool, e.ImpLossProtectionE8)

	// Logic for withdraw changed since start of chaosnet 2021-04.
	//
	// Background: In order to initiate a withdraw the user needs to send a transaction with a memo.
	// On many asset chains one can't send 0 value, therefore they often send a small amount
	// of asset (e.g. 1e-8). The value sent in by the user is shown in withdraw.coin
	// (unstake.asset_e8)
	//
	// New logic: keeps withdraw.coin in the wallets but it doesn't increment depths with it.
	//
	// Old logic: Pools don't keep this amount, it's forwarded back to the user and included in
	// the EmitAssetE8. Therefore pool depth decreases with less then the EmitAssetE8
	// This was not applied for rune or assets different then the native asset of the chain:
	//   - for Rune there is no minimum amount, if some rune is sent it's kept as donation.
	//   - when for non chain native assets (e.g. ETH.USDT) the EmitAssetE8 could not have contained
	//     the coin sent in. After withdrawCoinPooledHeight THORNode does not add AssetE8 into
	//     the withdraw EmitAssetE8 instead it will be added to its corresponding pool
	if meta.BlockHeight < withdrawCoinKeptHeight {
		if e.AssetE8 != 0 && string(e.Pool) == string(e.Asset) {
			r.AddPoolAssetE8Depth(e.Pool, e.AssetE8)
		}
	}

	// In some cases a saver withdrawal to the LTC/LTC pool happens (saver vault) and the AssetE8
	// will be added to the LTC.LTC pool. Also, another example can be withdrawal
	// from ETH.USDC-ID (EmitAssetE8) with ETH.ETH (AssetE8) withdraw message,
	// then ETH.ETH (AssetE8) will be added to its pool.
	// Checking chain and depth here is just a sanity check.
	if meta.BlockHeight >= withdrawCoinPooledHeight && e.AssetE8 != 0 {
		if aD, rD, _ := r.CurrentDepths(e.Asset); (aD > 0 || rD > 0) &&
			string(e.Chain) == util.AssetFromString(string(e.Pool)).Chain {
			r.AddPoolAssetE8Depth(e.Asset, e.AssetE8)
		}
	}
}

func (*eventRecorder) OnValidatorRequestLeave(e *ValidatorRequestLeave, meta *Metadata) {
	cols := []string{"tx", "from_addr", "node_addr"}
	err := InsertWithMeta("validator_request_leave_events", meta, cols,
		e.Tx, e.FromAddr, e.NodeAddr)

	if err != nil {
		btcqerr.LogEventParseErrorF(
			"validator_request_leave event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnPoolBalanceChange(e *PoolBalanceChange, meta *Metadata) {
	cols := []string{
		"asset", "rune_amt", "rune_add", "asset_amt", "asset_add", "reason"}
	err := InsertWithMeta("pool_balance_change_events", meta, cols,
		e.Asset, e.RuneAmt, e.RuneAdd, e.AssetAmt, e.AssetAdd, e.Reason)

	if err != nil {
		btcqerr.LogEventParseErrorF(
			"pool_balance_change event from height %d lost on %s",
			meta.BlockHeight, err)
	}

	assetAmount := e.AssetAmt
	if assetAmount != 0 {
		if !e.AssetAdd {
			assetAmount *= -1
		}
		r.AddPoolAssetE8Depth(e.Asset, assetAmount)
	}
	runeAmount := e.RuneAmt
	if runeAmount != 0 {
		if !e.RuneAdd {
			runeAmount *= -1
		}
		r.AddPoolRuneE8Depth(e.Asset, runeAmount)
	}
}

func (*eventRecorder) OnSlashPoints(e *SlashPoints, meta *Metadata) {
	cols := []string{"node_address", "slash_points", "reason"}
	err := InsertWithMeta("slash_points_events", meta, cols,
		e.NodeAddress, e.SlashPoints, e.Reason)
	if err != nil {
		btcqerr.LogEventParseErrorF(
			"slash_points event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

// OnTHORNameChange is kept for corrections/historical data fixes
func (*eventRecorder) OnTHORNameChange(e *THORNameChange, meta *Metadata) {
	cols := []string{
		"name", "chain", "address", "registration_fee_e8", "fund_amount_e8", "expire", "owner", "tx_id", "memo", "sender"}
	err := InsertWithMeta("thorname_change_events", meta, cols,
		e.Name, e.Chain, e.Address, e.RegistrationFeeE8, e.FundAmountE8, e.ExpireHeight, e.Owner, e.TxID, e.Memo, e.Sender)

	if err != nil {
		btcqerr.LogEventParseErrorF(
			"thorname event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

// OnBurn is kept for corrections/historical data fixes
func (r *eventRecorder) OnBurn(e *Burn, meta *Metadata) {
	r.AddPoolSynthE8Depth([]byte(util.ConvertSynthPoolToNative(string(e.Asset))), -e.AssetE8)
}

// OnCoinbase is kept for corrections/historical data fixes
func (r *eventRecorder) OnCoinbase(e *Coinbase, meta *Metadata) {
	r.AddPoolSynthE8Depth([]byte(util.ConvertSynthPoolToNative(string(e.Asset))), e.AssetE8)
}

func (*eventRecorder) OnInstantiate(e *Instantiate, meta *Metadata) {
	cols := []string{"tx_id", "admin_address", "code_id", "sender", "label", "msg", "funds",
		"contract_address"}
	err := InsertWithMeta("instantiate_events", meta, cols,
		e.TxID, e.Admin, e.CodeID, e.Sender, e.Label, e.Msg, e.Funds, e.ContractAddress)

	if err != nil {
		btcqerr.LogEventParseErrorF(
			"instantiate_contract event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnCosmWasm(e *CosmWasmEvent, meta *Metadata) {
	attributes, err := json.Marshal(e.Attributes)
	if err != nil {
		btcqerr.LogEventParseErrorF(
			"wasm_contracts_events attributes event from height %d lost on %s",
			meta.BlockHeight, err)
	}

	cols := []string{"tx_id", "contract_address", "contract_type", "sender", "attributes", "msg",
		"funds"}
	err = InsertWithMeta("wasm_contracts_events", meta, cols, e.TxID, e.ContractAddress, e.Type,
		e.Sender, attributes, e.Msg, e.Funds)
	if err != nil {
		btcqerr.LogEventParseErrorF(
			"wasm_contracts_events event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}
