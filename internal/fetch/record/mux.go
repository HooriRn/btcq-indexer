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
	btypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
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
	case "ActiveVault":
		var x ActiveVault
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnActiveVault(&x, meta)
	case "donate":
		// TODO(acsaba): rename add to donate
		var x Add
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnAdd(&x, meta)
	case "asgard_fund_yggdrasil":
		var x AsgardFundYggdrasil
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnAsgardFundYggdrasil(&x, meta)
	case "bond":
		var x Bond
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnBond(&x, meta)
	case "errata":
		var x Errata
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnErrata(&x, meta)
	case "fee":
		var x Fee
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		if CorrectionsFeeEventIsOK(&x, meta) {
			Recorder.OnFee(&x, meta)
		}
	case "InactiveVault":
		var x InactiveVault
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnInactiveVault(&x, meta)
	case "gas":
		var x Gas
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnGas(&x, meta)
	case "message":
		var x Message
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnMessage(&x, meta)
	case "new_node":
		var x NewNode
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnNewNode(&x, meta)
	case "outbound":
		var x Outbound
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnOutbound(&x, meta)
	case "pool":
		var x Pool
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnPool(&x, meta)
	case "refund":
		var x Refund
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnRefund(&x, meta)
	case "reserve":
		var x Reserve
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnReserve(&x, meta)
	case "rewards":
		var x Rewards
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		PoolRewardsTotal.Add(uint64(len(x.PerPool)))
		Recorder.OnRewards(&x, meta)
	case "set_ip_address":
		var x SetIPAddress
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSetIPAddress(&x, meta)
	case "set_mimir":
		var x SetMimir
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSetMimir(&x, meta)
	case "set_node_keys":
		var x SetNodeKeys
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSetNodeKeys(&x, meta)
	case "set_version":
		var x SetVersion
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSetVersion(&x, meta)
	case "slash":
		var x Slash
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSlash(&x, meta)
	case "pending_liquidity":
		var x PendingLiquidity
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnPendingLiquidity(&x, meta)
	case "add_liquidity", "add":
		var x Stake
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnStake(&x, meta)
	case "transfer":
		var x Transfer
		if err := x.LoadTendermint(attrs); err != nil {
			if err.Error() == "empty amount" {
				// Ignore transfers with null amount.
				// TODO(huginn): investigate why this happens.
				return nil
			}
			return err
		}
		Recorder.OnTransfer(&x, meta)
	case "withdraw":
		var x Withdraw
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		if CorrectWithdraw(&x, meta) == Discard {
			break
		}
		Recorder.OnWithdraw(&x, meta)
	case "UpdateNodeAccountStatus":
		var x UpdateNodeAccountStatus
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnUpdateNodeAccountStatus(&x, meta)
	case "validator_request_leave":
		var x ValidatorRequestLeave
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnValidatorRequestLeave(&x, meta)
	case "pool_balance_change":
		var x PoolBalanceChange
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnPoolBalanceChange(&x, meta)
	case "thorname":
		var x THORNameChange
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnTHORNameChange(&x, meta)
	case "switch":
		var x Switch
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSwitch(&x, meta)
	case "slash_points":
		var x SlashPoints
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSlashPoints(&x, meta)
	case "instantiate":
		var x Instantiate
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnInstantiate(&x, meta)
	case "tx":
	case "coin_spent", "coin_received":
	case "coinbase":
	case "burn":
	case "tss_keygen", "tss_keysign":
	case "create_client", "update_client":
	case "connection_open_init":
	case "store_code":
	case "pin_code":
	case "security":
	case "execute":
	case "approve_upgrade":
	case "reply":
	case "oracle_price":
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
		case *stypes.MsgSend:
			var x Send
			if err := x.LoadTendermint(tx, result, *m); err != nil {
				return err
			}
			Recorder.OnMsgSend(&x, meta)
		case *btypes.MsgSend:
			var x Send
			if err := x.LoadTendermintBank(tx, result, *m); err != nil {
				return err
			}
			Recorder.OnMsgSend(&x, meta)
		case *stypes.MsgDeposit:
			var x Deposit
			// for now just parse error deposits
			if result.Code == 0 || TxState[x.Hash] != nil {
				return nil
			}
			if err := x.LoadTendermint(tx, result, *m); err != nil {
				return err
			}
			// Add the deposit to the state global variable
			TxState[x.Hash] = x
			Recorder.OnDeposit(&x, meta)
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
	case "thorname":
		var signer string
		var memo string = tx.Memo

		for _, msg := range tx.Msgs {
			switch m := msg.(type) {
			case *stypes.MsgDeposit:
				signer = m.Signer.String()
				if memo == "" {
					memo = m.Memo
				}
				break
			case *stypes.MsgSend:
				signer = m.GetFromAddress().String()
				break
			}
		}

		event.Attributes = append(event.Attributes, abci.EventAttribute{
			Key:   "tx_id",
			Value: tx.Hash,
		}, abci.EventAttribute{
			Key:   "memo",
			Value: tx.Memo,
		}, abci.EventAttribute{
			Key:   "signer",
			Value: signer,
		})
	case "add_liquidity", "add":
		// Add missing memo to the add_liquidity event
		var memo string
		for _, msg := range tx.Msgs {
			switch m := msg.(type) {
			case *stypes.MsgDeposit:
				memo = m.Memo
			case *stypes.MsgObservedTxIn:
				memo = m.Txs[0].Tx.Memo
			case *stypes.MsgObservedTxQuorum:
				memo = m.QuoTx.ObsTx.Tx.Memo
			}
		}

		event.Attributes = append(event.Attributes, abci.EventAttribute{
			Key:   "memo",
			Value: memo,
		})
	case "switch":
		var depositCoin string
		var depositAmt string
		var txID string
		for _, v := range event.Attributes {
			if v.Key == "tx_id" {
				txID = v.Value
			}
		}

		for _, msg := range tx.Msgs {
			switch m := msg.(type) {
			case *stypes.MsgObservedTxIn:
				for _, tx := range m.Txs {
					if tx.Tx.ID.String() == txID {
						depositCoin = tx.Tx.Coins[0].Asset.String()
						depositAmt = tx.Tx.Coins[0].Amount.String()
					}
				}
			case *stypes.MsgObservedTxQuorum:
				tx := m.QuoTx.ObsTx
				if tx.Tx.ID.String() == txID {
					depositCoin = tx.Tx.Coins[0].Asset.String()
					depositAmt = tx.Tx.Coins[0].Amount.String()
				}
			}
		}

		event.Attributes = append(event.Attributes, abci.EventAttribute{
			Key:   "burn_asset",
			Value: depositCoin,
		}, abci.EventAttribute{
			Key:   "burn_amount",
			Value: depositAmt,
		})
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
	case "tcy_claim", "tcy_stake", "tcy_unstake":
		hash := tx.Hash
		memo := tx.Memo
		for _, msg := range tx.Msgs {
			switch m := msg.(type) {
			case *stypes.MsgDeposit:
				memo = m.Memo
			case *stypes.MsgObservedTxIn:
				memo = m.Txs[0].Tx.Memo
				hash = m.Txs[0].Tx.ID.String()
			case *stypes.MsgObservedTxQuorum:
				memo = m.QuoTx.ObsTx.Tx.Memo
				hash = m.QuoTx.ObsTx.Tx.ID.String()
			}
		}

		event.Attributes = append(event.Attributes, abci.EventAttribute{
			Key:   "memo",
			Value: memo,
		}, abci.EventAttribute{
			Key:   "tx_id",
			Value: hash,
		})
	case "bond":
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
			case *stypes.MsgDeposit:
				event.Attributes = append(event.Attributes, abci.EventAttribute{
					Key:   "signer",
					Value: m.Signer.String(),
				})
			case *stypes.MsgSetVersion:
				event.Attributes = append(event.Attributes, abci.EventAttribute{
					Key:   "signer",
					Value: m.Signer.String(),
				})
			}
			break
		}
	case "limit_swap":
		var toAddress string
		var fromAddr string
		var memo string

		for _, msg := range tx.Msgs {
			switch m := msg.(type) {
			case *stypes.MsgDeposit:
				memo = m.Memo
				fromAddr = m.Signer.String()
				// Third part of the memo is the to address
				if strings.Contains(m.Memo, ":") && len(strings.Split(m.Memo, ":")) > 2 {
					toAddress = strings.Split(m.Memo, ":")[2]
				}
			case *stypes.MsgObservedTxIn:
				memo = m.Txs[0].Tx.Memo
				fromAddr = m.Txs[0].Tx.FromAddress.String()
				toAddress = m.Txs[0].Tx.ToAddress.String()
			case *stypes.MsgObservedTxQuorum:
				memo = m.QuoTx.ObsTx.Tx.Memo
				fromAddr = m.QuoTx.ObsTx.Tx.FromAddress.String()
				toAddress = m.QuoTx.ObsTx.Tx.ToAddress.String()
			}
		}

		event.Attributes = append(event.Attributes, abci.EventAttribute{
			Key:   "memo",
			Value: memo,
		}, abci.EventAttribute{
			Key:   "from_address",
			Value: fromAddr,
		}, abci.EventAttribute{
			Key:   "to_address",
			Value: toAddress,
		})
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
