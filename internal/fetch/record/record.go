package record

import (
	"encoding/json"
	"strings"

	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/util/btcqerr"
)

func AddressIsQbtc(address string) bool {
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

	cols2 := []string{"pool", "qbtc_e8"}
	for _, p := range e.PerPool {
		err := InsertWithMeta("rewards_event_entries", meta, cols2, p.Asset, p.E8)
		if err != nil {
			btcqerr.LogEventParseErrorF(
				"reserve event pools from height %d lost on %s",
				meta.BlockHeight, err)
			return
		}
	}

	for _, a := range e.PerPool {
		r.AddPoolQbtcE8Depth(a.Asset, a.E8)
	}
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

func (r *eventRecorder) OnTransfer(e *Transfer, meta *Metadata) {
	cols := []string{"from_addr", "to_addr", "asset", "amount_e8"}
	err := InsertWithMeta("transfer_events", meta, cols, e.FromAddr, e.ToAddr, e.Asset, e.AmountE8)
	if err != nil {
		btcqerr.LogEventParseErrorF("transfer event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}
