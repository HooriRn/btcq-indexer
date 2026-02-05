package record

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pascaldekloe/metrics"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/fetch/sync/chain"
	"github.com/btcq/btcq-indexer/internal/util/btcqerr"
	"github.com/btcq/btcq-indexer/internal/util/timer"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/btcq-org/qbtc/x/qbtc/types"
)

// Package Metrics
var (
	blockProcTimer = timer.NewTimer("block_write_process")
	EventProcTime  = metrics.Must1LabelHistogram("btcq_indexer_chain_event_process_seconds", "type", 0.001, 0.01, 0.1)

	EventTotal            = metrics.Must1LabelCounter("btcq_indexer_chain_events_total", "group")
	DeliverTxEventsTotal  = EventTotal("deliver_tx")
	BeginBlockEventsTotal = EventTotal("begin_block")
	EndBlockEventsTotal   = EventTotal("end_block")
	IgnoresTotal          = metrics.MustCounter("btcq_indexer_chain_event_ignores_total", "Number of known types not in use seen.")
	UnknownsTotal         = metrics.MustCounter("btcq_indexer_chain_event_unknowns_total", "Number of unknown types discarded.")

	AttrPerEvent = metrics.MustHistogram("btcq_indexer_chain_event_attrs", "Number of attributes per event.", 0, 1, 7, 21, 144)

	PoolRewardsTotal = metrics.MustCounter("btcq_indexer_pool_rewards_total", "Number of asset amounts on rewards events seen.")
)

// Metadata has metadata for a block (from the chain).
type Metadata struct {
	BlockHeight    int64
	BlockTimestamp time.Time
	EventId        db.EventId
}

// combine the tx msg and the endblock for better info
var TxState map[string]interface{}

// Block invokes Listener for each transaction event in block.
func ProcessBlock(block *chain.Block) {
	defer blockProcTimer.One()()

	applyBlockCorrections(block)

	// Initialize on the process block
	TxState = make(map[string]interface{})

	m := Metadata{
		BlockHeight:    block.Height,
		BlockTimestamp: block.Time,
		EventId:        db.EventId{BlockHeight: block.Height},
	}

	// “The BeginBlock ABCI message is sent from the underlying Tendermint
	// engine when a block proposal created by the correct proposer is
	// received, before DeliverTx is run for each transaction in the block.
	// It allows developers to have logic be executed at the beginning of
	// each block.”
	// — https://docs.cosmos.network/master/core/baseapp.html#beginblock
	m.EventId.Location = db.BeginBlockEvents
	m.EventId.EventIndex = 1
	beginBlockEventsCount := 0
	for eventIndex, event := range block.Results.FinalizeBlockEvents {
		hasMode := false
		isBeginBlock := false
		// Check if the event is a BeginBlock or if it doesn't have a mode attribute
		for _, attr := range event.Attributes {
			if attr.Key == "mode" {
				hasMode = true
				if attr.Value == "BeginBlock" {
					isBeginBlock = true
				}
			}
		}
		if isBeginBlock || !hasMode {
			if err := processEvent(event, &m); err != nil {
				btcqerr.LogEventParseErrorF("block height %d begin event %d type %q skipped: %s",
					block.Height, eventIndex, event.Type, err)
			}
			beginBlockEventsCount++
		}
		m.EventId.EventIndex++
	}
	BeginBlockEventsTotal.Add(uint64(beginBlockEventsCount))

	m.EventId.Location = db.TxsResults
	m.EventId.TxIndex = 1
	for txIndex, tx := range block.Results.TxsResults {
		DeliverTxEventsTotal.Add(uint64(len(tx.Events)))
		m.EventId.EventIndex = 1
		decodedTx := decodeTx(block.PureBlock.Block.Txs[txIndex])
		if err := processTx(decodedTx, tx, &m); err != nil {
			btcqerr.LogEventParseErrorF("block height %d tx %d skipped: %s",
				block.Height, txIndex, err)
		}
		for eventIndex, event := range tx.Events {
			// Update the event according to its tx result
			if err := processParentTx(decodedTx, &event); err != nil {
				btcqerr.LogEventParseErrorF("block height %d tx %d event %d type %q skipped: %s (can't process parent)",
					block.Height, txIndex, eventIndex, event.Type, err)
			}
			if err := processEvent(event, &m); err != nil {
				btcqerr.LogEventParseErrorF("block height %d tx %d event %d type %q skipped: %s",
					block.Height, txIndex, eventIndex, event.Type, err)
			}
			m.EventId.EventIndex++
		}
		m.EventId.TxIndex++
	}

	// “The EndBlock ABCI message is sent from the underlying Tendermint
	// engine after DeliverTx as been run for each transaction in the block.
	// It allows developers to have logic be executed at the end of each
	// block.”
	// — https://docs.cosmos.network/master/core/baseapp.html#endblock
	endBlockEventsCount := 0
	m.EventId.Location = db.EndBlockEvents
	m.EventId.EventIndex = 1
	for eventIndex, event := range block.Results.FinalizeBlockEvents {
		for _, attr := range event.Attributes {
			if attr.Key == "mode" {
				if attr.Value == "EndBlock" {
					if err := processEvent(event, &m); err != nil {
						btcqerr.LogEventParseErrorF("block height %d end event %d type %q skipped: %s",
							block.Height, eventIndex, event.Type, err)
					}
					m.EventId.EventIndex++
					endBlockEventsCount++
				}
			}
		}
	}
	EndBlockEventsTotal.Add(uint64(endBlockEventsCount))

	AddMissingEvents(&m)
}

var errEventType = errors.New("unknown event type")

// Block notifies Listener for the transaction event.
// Errors do not include the event type in the message.
func processEvent(event abci.Event, meta *Metadata) error {
	defer EventProcTime(event.Type).AddSince(time.Now())

	attrs := event.Attributes
	AttrPerEvent.Add(float64(len(attrs)))

	// filter attributes
	newAttrs := make([]abci.EventAttribute, 0, len(attrs))
	for _, attr := range attrs {
		// drop the mode and msg_index attributes
		switch attr.Key {
		case "mode", "msg_index":
			continue
		}

		// filter empty values attributes - post V50 empty string should behave like nil
		if len(attr.Value) == 0 {
			continue
		}

		newAttrs = append(newAttrs, attr)
	}
	attrs = newAttrs

	switch event.Type {
	case "rewards":
		var x Rewards
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		PoolRewardsTotal.Add(uint64(len(x.PerPool)))
		Recorder.OnRewards(&x, meta)
	case "instantiate":
		var x Instantiate
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnInstantiate(&x, meta)
	case "tx":
	case "coin_spent", "coin_received":
	case "coinbase":
	case "security":
	case "execute":
	case "mint":
	case "wasm":
	case "limit_swap_close":
	// BTCQ specific events
	case "commission":
	default:
		// Check if the string starts with "wasm-"
		if strings.HasPrefix(event.Type, "cosmos.epochs.") {
			return nil
		}
		if strings.HasPrefix(event.Type, "wasm-") {
			var x CosmWasmEvent
			// Add type as attributes
			attrs = append(attrs, abci.EventAttribute{
				Key:   "type",
				Value: event.Type,
			})

			if err := x.LoadTendermint(attrs); err != nil {
				return err
			}
			Recorder.OnCosmWasm(&x, meta)
			break
		}
		btcqerr.LogEventParseErrorF("Unknown event type: %s, attributes: %s",
			event.Type, FormatAttributes(attrs))
		UnknownsTotal.Add(1)
		return errEventType
	}
	return nil
}

func processTx(tx DecodedTx, result *abci.ExecTxResult, meta *Metadata) error {
	// Thornode txs seems to have mainly one message
	for _, msg := range tx.Msgs {
		switch m := msg.(type) {
		case *types.MsgBtcBlock:
			// qbtc block submission (qbtc.qbtc.v1.MsgBtcBlock), no indexer action
		default:
			fmt.Println("Unknown message type:", m)
		}
	}

	return nil
}

func processParentTx(tx DecodedTx, event *abci.Event) error {
	// If the tx cannot be decoded
	if tx.Msgs == nil {
		return nil
	}

	switch event.Type {
	case "instantiate":
		msgIndex := 0
		for _, v := range event.Attributes {
			if v.Key == "msg_index" {
				var err error
				msgIndex, err = strconv.Atoi(v.Value)
				if err != nil {
					return fmt.Errorf("can't parse msg_index: %w", err)
				}
				break
			}
		}

		for i, msg := range tx.Msgs {
			if i != msgIndex {
				continue
			}

			switch m := msg.(type) {
			case *wasmtypes.MsgInstantiateContract:
				event.Attributes = append(event.Attributes, abci.EventAttribute{
					Key:   "sender",
					Value: m.Sender,
				}, abci.EventAttribute{
					Key:   "label",
					Value: m.Label,
				}, abci.EventAttribute{
					Key:   "msg",
					Value: string(m.Msg),
				}, abci.EventAttribute{
					Key:   "funds",
					Value: m.Funds.String(),
				}, abci.EventAttribute{
					Key:   "admin_address",
					Value: m.Admin,
				}, abci.EventAttribute{
					Key:   "tx_id",
					Value: tx.Hash,
				})
			}
			break
		}
	default:
		if strings.HasPrefix(event.Type, "wasm-") {
			msgIndex := 0
			for _, v := range event.Attributes {
				if v.Key == "msg_index" {
					var err error
					msgIndex, err = strconv.Atoi(v.Value)
					if err != nil {
						return fmt.Errorf("can't parse msg_index: %w", err)
					}
					break
				}
			}

			for i, msg := range tx.Msgs {
				if msgIndex != i {
					continue
				}

				switch m := msg.(type) {
				case *wasmtypes.MsgExecuteContract:
					event.Attributes = append(event.Attributes, abci.EventAttribute{
						Key:   "tx_id",
						Value: tx.Hash,
					}, abci.EventAttribute{
						Key:   "sender",
						Value: m.Sender,
					}, abci.EventAttribute{
						Key:   "msg",
						Value: string(m.Msg),
					}, abci.EventAttribute{
						Key:   "funds",
						Value: m.Funds.String(),
					})
				case *wasmtypes.MsgInstantiateContract:
					event.Attributes = append(event.Attributes, abci.EventAttribute{
						Key:   "tx_id",
						Value: tx.Hash,
					}, abci.EventAttribute{
						Key:   "sender",
						Value: m.Sender,
					}, abci.EventAttribute{
						Key:   "msg",
						Value: string(m.Msg),
					}, abci.EventAttribute{
						Key:   "funds",
						Value: m.Funds.String(),
					})
				}
			}

			break
		}
	}

	return nil
}

func FormatAttributes(attrs []abci.EventAttribute) string {
	buf := bytes.Buffer{}
	fmt.Fprint(&buf, "{")
	for _, attr := range attrs {
		fmt.Fprint(&buf, `"`, string(attr.Key), `": "`, string(attr.Value), `"`)
	}
	fmt.Fprint(&buf, "}")
	return buf.String()
}
