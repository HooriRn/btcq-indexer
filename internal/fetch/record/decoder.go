package record

import (
	"encoding/hex"
	"strings"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	tendtypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/std"
	ctypes "github.com/cosmos/cosmos-sdk/types"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	btypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
	prefix "gitlab.com/thorchain/thornode/v3/cmd"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/ebifrost"
	stypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

var protoCodec *codec.ProtoCodec

const (
	Bech32PrefixAccAddr  = "sthor"
	Bech32PrefixAccPub   = "sthorpub"
	Bech32PrefixValAddr  = "sthorv"
	Bech32PrefixValPub   = "sthorvpub"
	Bech32PrefixConsAddr = "sthorc"
	Bech32PrefixConsPub  = "sthorcpub"
)

// create tx decoder
// from https://gitlab.com/thorchain/thornode/-/blob/737151bd77b11cf134a2e14105fb9116170711dc/cmd/thornode/main.go#L19
func init() {
	interfaceRegistry := types.NewInterfaceRegistry()
	std.RegisterInterfaces(interfaceRegistry)
	btypes.RegisterInterfaces(interfaceRegistry)
	stypes.RegisterInterfaces(interfaceRegistry)
	ctypes.RegisterInterfaces(interfaceRegistry)
	wasmtypes.RegisterInterfaces(interfaceRegistry)
	protoCodec = codec.NewProtoCodec(interfaceRegistry)

	config := cosmos.GetConfig()
	if strings.Contains(db.RootChain.Get().Name, "stagenet") {
		config.SetBech32PrefixForAccount(Bech32PrefixAccAddr, Bech32PrefixAccPub)
		config.SetBech32PrefixForValidator(Bech32PrefixValAddr, Bech32PrefixValPub)
		config.SetBech32PrefixForConsensusNode(Bech32PrefixConsAddr, Bech32PrefixConsPub)

	} else {
		config.SetBech32PrefixForAccount(prefix.Bech32PrefixAccAddr, prefix.Bech32PrefixAccPub)
		config.SetBech32PrefixForValidator(prefix.Bech32PrefixValAddr, prefix.Bech32PrefixValPub)
		config.SetBech32PrefixForConsensusNode(prefix.Bech32PrefixConsAddr, prefix.Bech32PrefixConsPub)
	}

	config.SetCoinType(prefix.THORChainCoinType)
	config.SetPurpose(prefix.THORChainCoinPurpose)
	config.Seal()
}

type DecodedTx struct {
	Hash string       `json:"hash"`
	Memo string       `json:"memo"`
	Msgs []ctypes.Msg `json:"msgs"`
}

func decodeTx(tx tendtypes.Tx) (ret DecodedTx) {
	// each tx got a memo and hash which it comes from tmHash module
	ret.Hash = strings.ToUpper(hex.EncodeToString(tx.Hash()))

	// TODO:
	// After cometBFT upgrade some txs fail
	// non-positive integer: tx parse error [cosmos/cosmos-sdk@v0.50.9/x/auth/tx/decoder.go:49]
	dtx, err := ebifrost.TxDecoder(protoCodec, authtx.DefaultTxDecoder(protoCodec))(tx)
	if err != nil {
		midlog.WarnF("fail to decode tx block endpoint tx: %v", err)
		return
	}

	if mem, ok := dtx.(ctypes.TxWithMemo); ok {
		ret.Memo = mem.GetMemo()
	}

	ret.Msgs = dtx.GetMsgs()

	return
}
