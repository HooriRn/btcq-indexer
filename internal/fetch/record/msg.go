package record

import (
	"fmt"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	abci "github.com/cometbft/cometbft/abci/types"
	btypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type Send struct {
	FromAddr string
	ToAddr   string
	Asset    []byte
	AssetE8  int64
	Memo     string
	Hash     string
	Code     int64
	Log      string
}

func (e *Send) LoadTendermint(tx DecodedTx, result *abci.ExecTxResult, msg stypes.MsgSend) error {
	e.Hash = tx.Hash
	e.Memo = tx.Memo

	e.FromAddr = msg.GetFromAddress().String()
	e.ToAddr = msg.GetToAddress().String()
	e.Code = int64(result.Code)
	e.Log = result.Log

	var err error
	if e.Asset, e.AssetE8, err = parseCosmosCoin([]byte(msg.GetAmount().String())); err != nil {
		return fmt.Errorf("can't parse the coin in message (%w)", err)
	}

	return nil
}

func (e *Send) LoadTendermintBank(tx DecodedTx, result *abci.ExecTxResult, msg btypes.MsgSend) error {
	e.Hash = tx.Hash
	e.Memo = tx.Memo

	e.FromAddr = msg.FromAddress
	e.ToAddr = msg.ToAddress
	e.Code = int64(result.Code)
	e.Log = result.Log

	var err error
	if e.Asset, e.AssetE8, err = parseCosmosCoin([]byte(msg.Amount.String())); err != nil {
		return fmt.Errorf("can't parse the coin in message (%w)", err)
	}

	return nil
}

type Deposit struct {
	FromAddr string
	Asset    []byte
	Code     int64
	Log      string
	AssetE8  int64
	Memo     string
	Hash     string
}

func (e *Deposit) LoadTendermint(tx DecodedTx, result *abci.ExecTxResult, msg stypes.MsgDeposit) error {
	e.Hash = tx.Hash
	e.Memo = msg.Memo
	e.FromAddr = msg.GetSigner().String()
	e.Code = int64(result.Code)
	e.Log = result.Log

	var err error
	e.Asset, e.AssetE8, err = parseCoin([]byte(msg.Coins.String()))
	if err != nil {
		return fmt.Errorf("malformed coins in msg deposit: %w", err)
	}

	return nil
}

type ObservedTxIn struct {
	Hash     string
	FromAddr string
	ToAddr   string
	Asset    []byte
	AssetE8  int64
}

func (e *ObservedTxIn) LoadTendermint(tx DecodedTx, msg stypes.MsgObservedTxIn) error {
	e.Hash = msg.Txs[0].Tx.ID.String()
	e.FromAddr = msg.Txs[0].Tx.FromAddress.String()
	e.ToAddr = msg.Txs[0].Tx.ToAddress.String()

	var err error
	e.Asset, e.AssetE8, err = parseCoin([]byte(msg.Txs[0].Tx.Coins.String()))
	if err != nil {
		return fmt.Errorf("malformed coins in msg observed tx in: %w", err)
	}

	return nil
}

type InstantiateContract struct {
	Sender string
	Admin  string
	Label  string
	Msg    []byte
	Funds  []byte
	Hash   string
}

func (e *InstantiateContract) LoadTendermint(tx DecodedTx, result *abci.ExecTxResult, msg wasmtypes.MsgInstantiateContract) error {
	e.Admin = msg.Admin
	e.Sender = msg.Sender
	e.Label = msg.Label
	e.Msg = msg.Msg
	e.Funds = []byte(msg.Funds.String())

	e.Hash = tx.Hash

	return nil
}
