package record

import (
	"encoding/hex"
	"encoding/json"

	abci "github.com/cometbft/cometbft/abci/types"
	tendtypes "github.com/cometbft/cometbft/types"

	"github.com/btcq/btcq-indexer/internal/fetch/sync/chain"
)

// eventPayload is the JSON shape for one event (finalized or tx).
type eventPayload struct {
	Type       string          `json:"type"`
	Attributes []attrPayload    `json:"attributes"`
}

type attrPayload struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// txPayload is the JSON shape for one tx in the blocks.txs array.
type txPayload struct {
	TxHash string         `json:"tx_hash"`
	TxHex  string         `json:"tx_hex"`
	Events []eventPayload `json:"events"`
}

func eventToPayload(e abci.Event) eventPayload {
	attrs := make([]attrPayload, 0, len(e.Attributes))
	for _, a := range e.Attributes {
		attrs = append(attrs, attrPayload{
			Key:   string(a.Key),
			Value: string(a.Value),
		})
	}
	return eventPayload{Type: e.Type, Attributes: attrs}
}

// BuildBlocksPayload serializes block.Results.FinalizeBlockEvents and block txs+events
// into JSON bytes for the blocks table columns finalized_events and txs.
// Returns (finalizedEventsJSON, txsJSON, nil) or (nil, nil, err).
func BuildBlocksPayload(block *chain.Block) (finalizedEvents, txs []byte, err error) {
	if block.Results == nil {
		finalizedEvents = []byte("[]")
		txs = []byte("[]")
		return finalizedEvents, txs, nil
	}

	// Finalized block events
	finalized := make([]eventPayload, 0, len(block.Results.FinalizeBlockEvents))
	for _, e := range block.Results.FinalizeBlockEvents {
		finalized = append(finalized, eventToPayload(e))
	}
	finalizedEvents, err = json.Marshal(finalized)
	if err != nil {
		return nil, nil, err
	}

	// Txs: for each tx, tx_hash (from decode), tx_hex, events
	var rawTxs [][]byte
	if block.PureBlock != nil && block.PureBlock.Block.Txs != nil {
		rawTxs = block.PureBlock.Block.Txs.ToSliceOfBytes()
	}
	txResults := block.Results.TxsResults
	n := len(rawTxs)
	if len(txResults) < n {
		n = len(txResults)
	}
	txList := make([]txPayload, 0, n)
	for i := 0; i < n; i++ {
		decoded := decodeTx(tendtypes.Tx(rawTxs[i]))
		events := make([]eventPayload, 0, len(txResults[i].Events))
		for _, e := range txResults[i].Events {
			events = append(events, eventToPayload(e))
		}
		txList = append(txList, txPayload{
			TxHash: decoded.Hash,
			TxHex:  hex.EncodeToString(rawTxs[i]),
			Events: events,
		})
	}
	txs, err = json.Marshal(txList)
	if err != nil {
		return nil, nil, err
	}
	return finalizedEvents, txs, nil
}
