package record

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pascaldekloe/metrics"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/fetch/sync/chain"
	"github.com/btcq/btcq-indexer/internal/util/btcqerr"
	"github.com/btcq/btcq-indexer/internal/util/timer"

	"github.com/btcq-org/qbtc/x/qbtc/types"
)

// Package Metrics
var (
	blockProcTimer = timer.NewTimer("block_write_process")
	EventProcTime  = metrics.Must1LabelHistogram("btcq_indexer_chain_event_process_seconds", "type", 0.001, 0.01, 0.1)

	EventTotal           = metrics.Must1LabelCounter("btcq_indexer_chain_events_total", "group")
	DeliverTxEventsTotal = EventTotal("deliver_tx")
	FinalizedEventsTotal = EventTotal("finalized")
	IgnoresTotal         = metrics.MustCounter("btcq_indexer_chain_event_ignores_total", "Number of known types not in use seen.")
	UnknownsTotal        = metrics.MustCounter("btcq_indexer_chain_event_unknowns_total", "Number of unknown types discarded.")

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

	// Process all FinalizeBlockEvents (begin/end block distinction not used).
	m.EventId.Location = db.FinalizedBlockEvents
	m.EventId.EventIndex = 1
	finalizedCount := 0
	for eventIndex, event := range block.Results.FinalizeBlockEvents {
		if err := processEvent(event, &m); err != nil {
			btcqerr.LogEventParseErrorF("block height %d finalize event %d type %q skipped: %s",
				block.Height, eventIndex, event.Type, err)
		}
		finalizedCount++
		m.EventId.EventIndex++
	}
	FinalizedEventsTotal.Add(uint64(finalizedCount))

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
		m.EventId.TxIndex++
	}

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

		// filter empty values attributes
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
	case "transfer":
		if !config.Global.EventRecorder.OnTransferEnabled {
			return nil
		}
		var x Transfer
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnTransfer(&x, meta)
	case "tx":
	case "coin_spent", "coin_received":
	case "coinbase":
	case "security":
	case "execute":
	case "mint":
	case "wasm":
	// BTCQ specific events
	case "commission":
	case "message":
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
		btcqerr.LogEventParseErrorF("block height %d unknown event type: %s, attributes: %s",
			meta.BlockHeight, event.Type, FormatAttributes(attrs))
		UnknownsTotal.Add(1)
		return errEventType
	}
	return nil
}

func processTx(tx DecodedTx, result *abci.ExecTxResult, meta *Metadata) error {
	for eventIndex, event := range result.Events {
		if err := processEvent(event, meta); err != nil {
			btcqerr.LogEventParseErrorF("block height %d tx %d event %d type %q skipped: %s",
				meta.BlockHeight, meta.EventId.TxIndex, eventIndex, event.Type, err)
		}
		meta.EventId.EventIndex++
	}

	for _, msg := range tx.Msgs {
		switch m := msg.(type) {
		case *types.MsgBtcBlock:
			// qbtc block submission (qbtc.qbtc.v1.MsgBtcBlock), no indexer action
		case *types.MsgSetNodePeerAddress:
			// set node peer address (qbtc.qbtc.v1.MsgSetNodePeerAddress), no indexer action
		default:
			btcqerr.LogEventParseErrorF("block height %d tx %d unknown message type: %T, tx hash: %s",
				meta.BlockHeight, meta.EventId.TxIndex, m, tx.Hash)
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
