package record

import (
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/stretchr/testify/require"
)

var GoldenAssets = []struct {
	Asset, Chain, Ticker, ID string
	Type                     CoinType
	NativeAsset              string
}{
	{"BTC.BTC", "BTC", "BTC", "", AssetNative, "BTC.BTC"},
	{"ETH.ETH", "ETH", "ETH", "", AssetNative, "ETH.ETH"},
	{"ETH.USDT-0xdac17f958d2ee523a2206206994597c13d831ec7", "ETH", "USDT", "0xdac17f958d2ee523a2206206994597c13d831ec7", AssetNative, "ETH.USDT-0xdac17f958d2ee523a2206206994597c13d831ec7"},
	{"BNB.BNB", "BNB", "BNB", "", AssetNative, "BNB.BNB"},
	{"BNB.RUNE-B1A", "BNB", "RUNE", "B1A", Rune, "BNB.RUNE-B1A"},
	{"THOR.RUNE", "THOR", "RUNE", "", Rune, "THOR.RUNE"},
	{"BNB/BNB", "BNB", "BNB", "", AssetSynth, "BNB.BNB"},
	{"ETH/USDT-0xdac17f958d2ee523a2206206994597c13d831ec7", "ETH", "USDT", "0xdac17f958d2ee523a2206206994597c13d831ec7", AssetSynth, "ETH.USDT-0xdac17f958d2ee523a2206206994597c13d831ec7"},
	{"", "", "", "", UnknownCoin, ""},
	{".", "", "", "", AssetNative, "."},
	{"-", "", "", "", AssetSecure, "-"},
	{".-", "", "", "", AssetNative, ".-"},
	{"1.", "1", "", "", AssetNative, "1."},
	{"2.-", "2", "", "", AssetNative, "2.-"},
	{"A", "", "A", "", UnknownCoin, "A"},
	{".B", "", "B", "", AssetNative, ".B"},
	{"C-", "C", "", "", AssetSecure, "C"},
	{".D-", "", "D", "", AssetNative, ".D-"},
	{"-a", "", "a", "", AssetSecure, "a"},
	{".-b", "", "", "b", AssetNative, ".-b"},
}

func TestParseAsset(t *testing.T) {
	for _, gold := range GoldenAssets {
		chain, ticker, ID := ParseAsset([]byte(gold.Asset))
		if string(chain) != gold.Chain || string(ticker) != gold.Ticker || string(ID) != gold.ID {
			t.Errorf("%q got [%q %q %q], want [%q %q %q]", gold.Asset, chain, ticker, ID, gold.Chain, gold.Ticker, gold.ID)
		}
	}
}

func TestGetCoinType(t *testing.T) {
	for _, gold := range GoldenAssets {
		coinType := GetCoinType([]byte(gold.Asset))
		if coinType != gold.Type {
			t.Errorf("%q got [%q], want [%q]", gold.Asset, coinType, gold.Type)
		}
	}
}

func TestGetNativeAsset(t *testing.T) {
	for _, gold := range GoldenAssets {
		coinType := GetCoinType([]byte(gold.Asset))
		if coinType != gold.Type {
			t.Errorf("%q got [%q], want [%q]", gold.Asset, coinType, gold.Type)
		}
	}
}

func TestTransfer(t *testing.T) {
	var event Transfer
	err := event.LoadTendermint(toAttrs(map[string]string{
		"sender":    "tthoraddr1",
		"recipient": "tthoraddr2",
		"amount":    "123rune",
	}))
	require.NoError(t, err)
	require.Equal(t, int64(123), event.AmountE8)
	require.Equal(t, nativeRune, string(event.Asset))
	require.Equal(t, "tthoraddr1", string(event.FromAddr))
	require.Equal(t, "tthoraddr2", string(event.ToAddr))

	event = Transfer{}
	err = event.LoadTendermint(toAttrs(map[string]string{
		"sender":    "tthoraddr1",
		"recipient": "tthoraddr2",
		"amount":    "987bnb/bnb",
	}))
	require.NoError(t, err)
	require.Equal(t, int64(987), event.AmountE8)
	require.Equal(t, "BNB/BNB", string(event.Asset))
}

func TestBond(t *testing.T) {
	var event Bond
	err := event.LoadTendermint(toAttrs(map[string]string{
		// "bond_type": "0", // Because of the nature of this test
		// (and THORNode's EventBond Attributes() using string(m.BondType) rather than m.BondType.String()),
		// non-string bond_type cannot be represented.
		"amount": "100",
		"chain":  "THOR",
		"coin":   "100 THOR.RUNE",
		"from":   "tthor1zf3gsk7edzwl9syyefvfhle37cjtql35h6k85m",
		"id":     "98C1864036571E805BB0E0CCBAFF0F8D80F69BDEA32D5B26E0DDB95301C74D0C",
		"memo":   "BOND:tthor1zf3gsk7edzwl9syyefvfhle37cjtql35h6k85m",
		"to":     "tthor17gw75axcnr8747pkanye45pnrwk7p9c3uhzgff",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if event.E8 != 100 || event.AssetE8 != 100 || string(event.Asset) != "THOR.RUNE" {
		t.Errorf(`got %d / %d / %q when expecting 100 / 100 / THOR.RUNE"`, event.E8, event.AssetE8, event.Asset)
	}
}

func toAttrs(m map[string]string) []abci.EventAttribute {
	a := make([]abci.EventAttribute, 0, len(m))
	for k, v := range m {
		a = append(a, abci.EventAttribute{Key: k, Value: v, Index: true})
	}
	return a
}
