package record

import (
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
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
	{"BNB.RUNE-B1A", "BNB", "RUNE", "B1A", Qbtc, "BNB.RUNE-B1A"},
	{"THOR.RUNE", "THOR", "RUNE", "", Qbtc, "THOR.RUNE"},
	{"", "", "", "", UnknownCoin, ""},
	{".", "", "", "", AssetNative, "."},
	{".-", "", "", "", AssetNative, ".-"},
	{"1.", "1", "", "", AssetNative, "1."},
	{"2.-", "2", "", "", AssetNative, "2.-"},
	{"A", "", "A", "", UnknownCoin, "A"},
	{".B", "", "B", "", AssetNative, ".B"},
	{".D-", "", "D", "", AssetNative, ".D-"},
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

func toAttrs(m map[string]string) []abci.EventAttribute {
	a := make([]abci.EventAttribute, 0, len(m))
	for k, v := range m {
		a = append(a, abci.EventAttribute{Key: k, Value: v, Index: true})
	}
	return a
}
