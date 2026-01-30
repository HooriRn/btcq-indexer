package main

import (
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/std"
	"github.com/cosmos/cosmos-sdk/store"
	"github.com/cosmos/cosmos-sdk/store/gaskv"
	"github.com/cosmos/cosmos-sdk/store/prefix"
	stypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/syndtr/goleveldb/leveldb/opt"
	dbm "github.com/tendermint/tm-db"
	//"gitlab.com/thorchain/thornode/v3/app"
)

func main() {
	amino := codec.NewLegacyAmino()
	interfaceRegistry := types.NewInterfaceRegistry()
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	std.RegisterLegacyAminoCodec(amino)
	std.RegisterInterfaces(interfaceRegistry)

	//cfg.Marshaler = codec.NewProtoCodec(cfg.InterfaceRegistry)

	db, err := dbm.NewGoLevelDBWithOpts(
		"application",
		"/root/tmp/data",
		&opt.Options{ReadOnly: true},
	)
	if err != nil {
		os.Exit(1)
	}
	keys := sdk.NewKVStoreKeys(banktypes.StoreKey)
	ms := store.NewCommitMultiStore(db)
	ms.MountStoreWithDB(keys[banktypes.StoreKey], sdk.StoreTypeIAVL, nil)
	height := 4786515
	ms.LoadVersion(int64(height))
	//store := prefix.NewStore(ms.GetKVStore(keys[banktypes.StoreKey]), []byte("balances"))
	store := prefix.NewStore(
		gaskv.NewStore(ms.GetKVStore(keys[banktypes.StoreKey]), stypes.NewInfiniteGasMeter(), stypes.KVGasConfig()), []byte("balances"))

	it := store.Iterator(nil, nil)
	defer it.Close()
	for it.Valid() {
		k, v := it.Key(), it.Value()
		var balance sdk.Coin
		marshaler.MustUnmarshal(v, &balance)
		addr, err := banktypes.AddressFromBalancesStore(k)

		it.Next()
		if err != nil {
			fmt.Printf("ADDR ERROR! key:[%v],value:[%v]\n", k, balance)
			continue
		}
		fmt.Printf("key:[%s],value:[%v]\n", addr.String(), balance)
	}

	/*
		ctx := sdk.NewContext(ms, tmproto.Header{ChainID: "thorchain"}, false, log.NewNopLogger())
		//interfaceRegistry := types.NewInterfaceRegistry()
		//marshaler := codec.NewProtoCodec(interfaceRegistry)
		view := keeper.NewBaseViewKeeper(
			cfg.Marshaler,
			keys[banktypes.StoreKey],
			nil)
		view.IterateAllBalances(ctx, func(aa sdk.AccAddress, c sdk.Coin) bool {
			fmt.Printf("%v,%v\n", aa, c)
			return true
		})
		for _, b := range view.GetAccountsBalances(ctx) {
			fmt.Printf("%v", b)
		}

		sdk.KVStoreReversePrefixIterator(ctx.KVStore(keys[banktypes.StoreKey]), banktypes.BalancesPrefix)
	*/
}
