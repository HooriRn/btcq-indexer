// Package event provides the blockchain data in a structured way.
//
// All asset amounts are fixed to 8 decimals. The resolution is made
// explicit with an E8 in the respective names.
//
// Numeric values are 64 bits wide, instead of the conventional 256
// bits used by most blockchains.
//
//	9 223 372 036 854 775 807  64-bit signed integer maximum
//	               00 000 000  decimals for fractions
//	   50 000 000 0·· ··· ···  500 M Rune total
//	    2 100 000 0·· ··· ···  21 M BitCoin total
//	   20 000 000 0·· ··· ···  200 M Ether total
package record

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/util"
	"github.com/btcq/btcq-indexer/internal/util/miderr"
)

// Asset Labels
const (
	// Native asset on THORChain.
	nativeRune = "THOR.RUNE"
	// Asset on Binance test net.
	rune67C = "BNB.RUNE-67C"
	// Asset on Binance main net.
	runeB1A = "BNB.RUNE-B1A"
	// TCY asset on THORChain.
	nativeTCY = "THOR.TCY"
	// RUJI asset on THORChain.
	nativeRUJI = "THOR.RUJI"
	// NAMI asset on THORChain.
	nativeNAMI = "THOR.NAMI"
)

// IsRune returns whether asset matches any of the supported $RUNE assets.
func IsRune(asset []byte) bool {
	switch string(asset) {
	case nativeRune, rune67C, runeB1A:
		return true
	}
	return false
}

// IsTcy returns whether asset matches any of the $TCY asset.
func IsTcy(asset []byte) bool {
	switch string(asset) {
	case nativeTCY:
		return true
	}
	return false
}

// IsRuji returns whether asset matches any of the $RUJI asset.
func IsRuji(asset []byte) bool {
	switch string(asset) {
	case nativeRUJI:
		return true
	}
	return false
}

func IsNami(asset []byte) bool {
	switch string(asset) {
	case nativeNAMI:
		return true
	}
	return false
}

type CoinType int

const (
	// Rune rune coin type
	Rune CoinType = iota
	// AssetNative coin native to a chain
	AssetNative
	// AssetSynth synth coin
	AssetSynth
	// AssetTrade trade account asset
	AssetTrade
	// UnknownCoin unknown coin
	UnknownCoin
	// Derived Asset coin, mostly made for THORFi
	AssetDerived
	// Secure Asset coin, v3 IBC
	AssetSecure
)

var (
	nativeSeparator = []byte(".")
	synthSeparator  = []byte("/")
	derivedAsset    = []byte("THOR.")
	contractAsset   = []byte("X/")
	tradeSeparator  = []byte("~")
	secureSeparator = []byte("-")
)

func GetCoinType(asset []byte) CoinType {
	if IsRune(asset) {
		return Rune
	}
	if IsTcy(asset) {
		return AssetNative
	}
	if IsRuji(asset) {
		return AssetNative
	}
	if IsNami(asset) {
		return AssetNative
	}
	if bytes.HasPrefix(bytes.ToUpper(asset), contractAsset) {
		return AssetNative
	}
	if bytes.Contains(asset, synthSeparator) {
		return AssetSynth
	}
	if bytes.HasPrefix(bytes.ToUpper(asset), derivedAsset) {
		return AssetDerived
	}
	if bytes.Contains(asset, nativeSeparator) {
		return AssetNative
	}
	if bytes.Contains(asset, tradeSeparator) {
		return AssetTrade
	}
	if bytes.Contains(asset, secureSeparator) && !bytes.Contains(asset, nativeSeparator) {
		return AssetSecure
	}
	return UnknownCoin
}

// RuneAsset returns a matching RUNE asset given a running environment
// (Logic is copied from THORnode code)
func RuneAsset() string {
	return nativeRune
}

// ParseAsset decomposes the notation.
//
//	asset  :≡ chain '.' symbol | symbol
//	symbol :≡ ticker '-' ID | ticker
func ParseAsset(asset []byte) (chain, ticker, id []byte) {
	if len(asset) == 0 {
		return
	}
	re := regexp.MustCompile("[~./-]")
	match := re.Find(asset)
	var symbol []byte
	var sep []byte
	if bytes.Equal(match, nativeSeparator) {
		sep = nativeSeparator
	}
	if bytes.Equal(match, synthSeparator) {
		sep = synthSeparator
	}
	if bytes.Equal(match, tradeSeparator) {
		sep = tradeSeparator
	}
	if bytes.Equal(match, secureSeparator) {
		sep = secureSeparator
	}
	parts := bytes.SplitN(asset, sep, 2)
	if len(parts) == 0 {
		return
	}
	if len(parts) == 1 {
		symbol = parts[0]
	} else {
		chain = parts[0]
		symbol = parts[1]
	}
	parts = bytes.SplitN(symbol, []byte("-"), 2)
	ticker = parts[0]
	if len(parts) > 1 {
		id = parts[1]
	}
	return
}

// GetNativeAsset returns native asset from a synth
func GetNativeAsset(asset []byte) []byte {
	if GetCoinType(asset) == AssetSynth || GetCoinType(asset) == AssetTrade || GetCoinType(asset) == AssetSecure {
		chain, ticker, ID := ParseAsset(asset)
		if len(ID) == 0 {
			return []byte(fmt.Sprintf("%s%s%s", chain, nativeSeparator, ticker))
		}
		return []byte(fmt.Sprintf("%s%s%s-%s", chain, nativeSeparator, ticker, ID))
	}
	return asset
}

// CoinSep is the separator for coin lists.
var coinSep = []byte{',', ' '}

type Amount struct {
	Asset []byte
	E8    int64
}

/*************************************************************/
/* Data models with Tendermint bindings in alphabetic order: */

// BUG(pascaldekloe): Duplicate keys in Tendermint transactions overwrite on another.

// ActiveVault defines the "ActiveVault" event type.
type ActiveVault struct {
	AddAsgardAddr []byte
}

func (e *ActiveVault) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		switch string([]byte(attr.Key)) {
		case "add new asgard vault":
			e.AddAsgardAddr = []byte(attr.Value)

		default:
			miderr.LogEventParseErrorF(
				"unknown ActiveVault event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// Add defines the "donate" event type.
type Add struct {
	Tx       []byte
	Chain    []byte
	FromAddr []byte
	ToAddr   []byte
	Asset    []byte
	AssetE8  int64 // Asset quantity times 100 M
	Memo     []byte

	RuneE8 int64 // Number of runes times 100 M

	Pool []byte
}

func (e *Add) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "id":
			e.Tx = []byte(attr.Value)
		case "chain":
			e.Chain = []byte(attr.Value)
		case "from":
			e.FromAddr = []byte(attr.Value)
		case "to":
			e.ToAddr = []byte(attr.Value)
		case "coin":
			b := []byte(attr.Value)
			for len(b) != 0 {
				var asset []byte
				var amountE8 int64
				if i := bytes.Index(b, coinSep); i >= 0 {
					asset, amountE8, err = parseCoin(b[:i])
					b = b[i+len(coinSep):]
				} else {
					asset, amountE8, err = parseCoin(b)
					b = nil
				}
				if err != nil {
					return fmt.Errorf("malformed coin: %w", err)
				}

				if IsRune(asset) {
					e.RuneE8 = amountE8
				} else {
					e.AssetE8 = amountE8
					e.Asset = asset
				}
			}
		case "memo":
			e.Memo = sanitizeBytes([]byte(attr.Value))

		case "pool":
			e.Pool = []byte(attr.Value)

		default:
			miderr.LogEventParseErrorF("unknown add event attribute %q=%q", []byte(attr.Key), []byte(attr.Value))
		}
	}

	if config.Global.CaseInsensitiveChains[string(e.Chain)] {
		e.FromAddr = util.ToLowerBytes(e.FromAddr)
		e.ToAddr = util.ToLowerBytes(e.ToAddr)
	}

	return nil
}

// AsgardFundYggdrasil defines the "asgard_fund_yggdrasil" event type.
type AsgardFundYggdrasil struct {
	Tx       []byte // THORChain transaction identifier
	Asset    []byte
	AssetE8  int64  // Asset quantity times 100 M
	VaultKey []byte // public key of yggdrasil
}

func (e *AsgardFundYggdrasil) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {

		case "tx":
			e.Tx = []byte(attr.Value)
		case "coins":
			e.Asset, e.AssetE8, err = parseCoin([]byte(attr.Value))
			if err != nil {
				return fmt.Errorf("malformed coins: %w", err)
			}
		case "pubkey":
			e.VaultKey = []byte(attr.Value)

		default:
			miderr.LogEventParseErrorF(
				"unknown asgard_fund_yggdrasil event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// Bond defines the "bond" event type.
type Bond struct {
	Tx         []byte
	Chain      []byte
	FromAddr   []byte
	ToAddr     []byte
	Asset      []byte
	AssetE8    int64 // Asset quantity times 100 M
	Memo       []byte
	BondAddr   []byte
	NodeAddr   []byte
	SignerAddr []byte

	BondType string
	E8       int64
}

func (e *Bond) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "id":
			e.Tx = []byte(attr.Value)
		case "chain":
			e.Chain = []byte(attr.Value)
		case "from":
			e.FromAddr = []byte(attr.Value)
		case "to":
			e.ToAddr = []byte(attr.Value)
		case "coin":
			if []byte(attr.Value) != nil {
				e.Asset, e.AssetE8, err = parseCoin([]byte(attr.Value))
				if err != nil {
					return fmt.Errorf("malformed coin: %w | %s", err, []byte(attr.Value))
				}
			}
		case "memo":
			e.Memo = []byte(attr.Value)
		case "amount":
			e.E8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed amount: %w", err)
			}
		case "bond_type", "bound_type":
			// e.BondType = []byte(attr.Value)
			// In older versions of THORNode, this returned an int representation of an enum rather than a string.
			// To account for both, we'll attempt to decode single-byte values (ints) as well as the proper string
			// encoding:
			// 0: "bond_paid"
			// 1: "bond_returned"
			// 2: "bond_reward"
			// 3: "bond_cost"
			if len([]byte(attr.Value)) == 1 {
				// NOTE: Only has a byte that's either 0 or 1 so don't really need to do any fancy decoding
				switch uint8([]byte(attr.Value)[0]) {
				case 0:
					e.BondType = "bond_paid"
				case 1:
					e.BondType = "bond_returned"
				case 2:
					e.BondType = "bond_reward"
				case 3:
					e.BondType = "bond_cost"
				default:
					return fmt.Errorf("malformed bond_type: %q", []byte(attr.Value))
				}
			} else if string([]byte(attr.Value)[:5]) == "bond_" {
				e.BondType = string([]byte(attr.Value))
			} else {
				return fmt.Errorf("malformed bond_type: should be a single byte or 'bond_' string but value is %q", []byte(attr.Value))
			}
		case "bond_address":
			e.BondAddr = []byte(attr.Value)
		case "node_address":
			e.NodeAddr = []byte(attr.Value)
		case "signer":
			e.SignerAddr = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown bond event attribute %q=%q", []byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// Errata defines the "errata" event type.
type Errata struct {
	InTx    []byte
	Asset   []byte
	AssetE8 int64 // Asset quantity times 100 M
	RuneE8  int64 // Number of runes times 100 M
}

func (e *Errata) LoadTendermint(attrs []abci.EventAttribute) error {
	var flipAsset, flipRune bool

	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "in_tx_id":
			e.InTx = []byte(attr.Value)
		case "asset":
			e.Asset = []byte(attr.Value)
		case "asset_amt":
			e.AssetE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed asset_amt: %w", err)
			}
		case "asset_add":
			add, err := strconv.ParseBool(string([]byte(attr.Value)))
			if err != nil {
				return fmt.Errorf("malformed asset_add: %w", err)
			}
			flipAsset = !add
		case "rune_amt":
			e.RuneE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed rune_amt: %w", err)
			}
		case "rune_add":
			add, err := strconv.ParseBool(string([]byte(attr.Value)))
			if err != nil {
				return fmt.Errorf("malformed rune_add: %w", err)
			}
			flipRune = !add
		default:
			miderr.LogEventParseErrorF("unknown errata event attribute %q=%q", []byte(attr.Key), []byte(attr.Value))
		}
	}

	if flipAsset {
		e.AssetE8 = -e.AssetE8
	}
	if flipRune {
		e.RuneE8 = -e.RuneE8
	}

	return nil
}

// Fee defines the "fee" event type, which records network fees applied to outbound transactions
type Fee struct {
	Tx         []byte // THORChain transaction identifier
	Asset      []byte
	AssetE8    int64 // Asset quantity times 100 M
	PoolDeduct int64 // rune quantity times 100 M
}

func (e *Fee) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "tx_id":
			e.Tx = []byte(attr.Value)
		case "coins":
			e.Asset, e.AssetE8, err = parseCoin([]byte(attr.Value))
			if err != nil {
				return fmt.Errorf("malformed coins: %w", err)
			}
		case "pool_deduct":
			e.PoolDeduct, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed pool_deduct: %w", err)
			}
		default:
			miderr.LogEventParseErrorF("unknown fee event attribute %q=%q", []byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// Gas defines the "gas" event type.
type Gas struct {
	Asset   []byte
	AssetE8 int64 // Asset quantity times 100 M
	RuneE8  int64 // Number of runes times 100 M
	TxCount int64
}

func (e *Gas) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "asset":
			e.Asset = []byte(attr.Value)
		case "asset_amt":
			e.AssetE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed asset_amt: %w", err)
			}
		case "rune_amt":
			e.RuneE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed rune_amt: %w", err)
			}
		case "transaction_count":
			e.TxCount, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed transaction_count: %w", err)
			}

		default:
			miderr.LogEventParseErrorF("unknown gas event attribute %q=%q", []byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// InactiveVault defines the "InactiveVault" event type.
type InactiveVault struct {
	AddAsgardAddr []byte
}

func (e *InactiveVault) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		switch string([]byte(attr.Key)) {
		case "set asgard vault to inactive":
			e.AddAsgardAddr = []byte(attr.Value)

		default:
			miderr.LogEventParseErrorF(
				"unknown InactiveVault event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// Message defines the "message" event type.
type Message struct {
	FromAddr []byte // optional sender
	Action   []byte
}

func (e *Message) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		switch string([]byte(attr.Key)) {
		case "sender":
			e.FromAddr = []byte(attr.Value)
		case "action":
			e.Action = []byte(attr.Value)
		case "module":
			// TODO(acsaba): this is discarded now, but figure out what it is and store it.
			//     currently seen values: "module"="governance"

		default:
			miderr.LogEventParseErrorF(
				"unknown message event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// NewNode defines the "new_node" event type.
type NewNode struct {
	NodeAddr []byte // THOR address
}

func (e *NewNode) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		switch string([]byte(attr.Key)) {
		case "address":
			e.NodeAddr = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF(
				"unknown new_node event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// Outbound defines the "outbound" event type, which records a transfer
// confirmation from pools. Each Swap, Withdraw, UnBond or Refunds event is
// completed with an Outbound.
//
// All zeros on Tx are ignored, thus keeping a nil value. E.g., the Outbound of
// the “to RUNE swap” on double-swaps has no transaction ID.
type Outbound struct {
	Tx       []byte // THORChain transaction ID
	Chain    []byte // transfer backend ID
	FromAddr []byte // transfer pool address
	ToAddr   []byte // transfer contender address
	Asset    []byte // transfer unit ID
	AssetE8  int64  // transfer quantity times 100 M
	Memo     []byte // transfer description
	InTx     []byte // THORChain transaction ID reference
}

func (e *Outbound) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "id":
			// omit all-zero placeholders
			for _, c := range []byte(attr.Value) {
				if c != '0' {
					e.Tx = []byte(attr.Value)
					break
				}
			}
		case "chain":
			e.Chain = []byte(attr.Value)
		case "from":
			e.FromAddr = []byte(attr.Value)
		case "to":
			e.ToAddr = []byte(attr.Value)
		case "coin":
			e.Asset, e.AssetE8, err = parseCoin([]byte(attr.Value))
			if err != nil {
				return fmt.Errorf("malformed coin: %w", err)
			}
		case "memo":
			e.Memo = []byte(attr.Value)

		case "in_tx_id":
			e.InTx = []byte(attr.Value)

		default:
			miderr.LogEventParseErrorF(
				"unknown outbound event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	if config.Global.CaseInsensitiveChains[string(e.Chain)] {
		e.FromAddr = util.ToLowerBytes(e.FromAddr)
		e.ToAddr = util.ToLowerBytes(e.ToAddr)
	}

	return nil
}

type ScheduledOutbound struct {
	Chain         []byte // transfer backend ID
	ToAddr        []byte // transfer contender address
	Asset         []byte // transfer unit ID
	AssetE8       int64  // transfer quantity times 100 M
	AssetDecimals int64
	Memo          []byte // transfer description
	InHash        []byte // THORChain transaction ID reference
	OutHash       []byte // THORChain transaction ID reference
	MaxGas        []int64
	MaxGasAsset   []string
	MaxGasDecimal []int64
	GasRate       int64
	ModuleName    []byte
	VaultPubKey   []byte
}

func (e *ScheduledOutbound) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "chain":
			e.Chain = []byte(attr.Value)
		case "coin_amount":
			e.AssetE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed coin amount: %w", err)
			}
		case "coin_asset":
			e.Asset = []byte(attr.Value)
		case "coin_decimals":
			e.AssetDecimals, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed coin decimals: %w", err)
			}
		case "gas_rate":
			e.GasRate, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed coin decimals: %w", err)
			}
		case "module_name":
			e.ModuleName = []byte(attr.Value)
		case "in_hash":
			e.InHash = []byte(attr.Value)
		case "to_address":
			e.ToAddr = []byte(attr.Value)
		case "memo":
			e.Memo = []byte(attr.Value)
		case "out_hash":
			e.OutHash = []byte(attr.Value)
		case "vault_pub_key":
			e.VaultPubKey = []byte(attr.Value)

		default:
			// Seems the max_gas_(amount/decimals/asset) can be multiple so:
			if strings.HasPrefix(string([]byte(attr.Key)), "max_gas") &&
				len(strings.Split(string([]byte(attr.Key)), "_")) == 4 {
				s := strings.Split(string([]byte(attr.Key)), "_")[2]
				switch s {
				case "amount":
					gas, err := strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
					if err != nil {
						return fmt.Errorf("malformed max gas amount: %w", err)
					}
					e.MaxGas = append(e.MaxGas, gas)
				case "asset":
					e.MaxGasAsset = append(e.MaxGasAsset, string([]byte(attr.Value)))
				case "decimals":
					decimals, err := strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
					if err != nil {
						return fmt.Errorf("malformed max gas decimals: %w", err)
					}
					e.MaxGasDecimal = append(e.MaxGasDecimal, decimals)
				}
			} else {
				miderr.LogEventParseErrorF(
					"unknown outbound event attribute %q=%q",
					[]byte(attr.Key), []byte(attr.Value))
			}
		}
	}

	if config.Global.CaseInsensitiveChains[string(e.Chain)] {
		e.ToAddr = util.ToLowerBytes(e.ToAddr)
	}

	return nil
}

// Pool defines the "pool" event type.
type Pool struct {
	Asset  []byte
	Status []byte
}

func (e *Pool) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		switch string([]byte(attr.Key)) {
		case "pool":
			e.Asset = []byte(attr.Value)
		case "pool_status":
			e.Status = []byte(attr.Value)

		default:
			miderr.LogEventParseErrorF("unknown pool event attribute %q=%q", []byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// Refund defines the "refund" event type.
type Refund struct {
	Tx         []byte
	Chain      []byte
	FromAddr   []byte
	ToAddr     []byte
	Asset      []byte
	AssetE8    int64 // Asset quantity times 100 M
	Asset2nd   []byte
	Asset2ndE8 int64 // Asset2 quantity times 100 M
	Memo       []byte

	Code   int64
	Reason []byte
}

// Correct v if it's not valid utf8 or it contains 0 bytes.
// Sometimes refund attribute is not valid utf8 and can't be inserted into the DB as is.
// Unfortunately bytes.ToValidUTF8 is not enough to fix because golang accepts
// 0 bytes as valid utf8 but Postgres doesn't.
func sanitizeBytes(v []byte) []byte {
	if utf8.Valid(v) && !bytes.ContainsRune(v, 0) {
		return v
	} else {
		return []byte("MidgardBadUTF8EncodedBase64: " + base64.StdEncoding.EncodeToString(v))
	}
}

func (e *Refund) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "id":
			e.Tx = []byte(attr.Value)
		case "chain":
			e.Chain = []byte(attr.Value)
		case "from":
			e.FromAddr = []byte(attr.Value)
		case "to":
			e.ToAddr = []byte(attr.Value)
		case "coin":
			v := []byte(attr.Value)
			if i := bytes.Index(v, []byte{',', ' '}); i >= 0 {
				e.Asset2nd, e.Asset2ndE8, err = parseCoin(v[i+2:])
				if err != nil {
					return fmt.Errorf("malformed coin: %w", err)
				}

				v = v[:i]
			}
			e.Asset, e.AssetE8, err = parseCoin(v)
			if err != nil {
				return fmt.Errorf("malformed coin: %w", err)
			}

		case "memo":
			e.Memo = sanitizeBytes([]byte(attr.Value))
		case "code":
			e.Code, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed code: %w", err)
			}
		case "reason":
			e.Reason = sanitizeBytes([]byte(attr.Value))
		default:
			miderr.LogEventParseErrorF("unknown refund event attribute %q=%q", []byte(attr.Key), []byte(attr.Value))
		}
	}

	if config.Global.CaseInsensitiveChains[string(e.Chain)] {
		e.FromAddr = util.ToLowerBytes(e.FromAddr)
		e.ToAddr = util.ToLowerBytes(e.ToAddr)
	}

	return nil
}

// Reserve defines the "reserve" event type.
type Reserve struct {
	Tx       []byte
	Chain    []byte // redundant to asset
	FromAddr []byte
	ToAddr   []byte // may have multiple, separated by space
	Asset    []byte
	AssetE8  int64 // Asset quantity times 100 M
	Memo     []byte

	Addr []byte
	E8   int64 // Number of runes times 100 M
}

func (e *Reserve) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		// thornode: common.Tx
		case "id":
			e.Tx = []byte(attr.Value)
		case "chain":
			e.Chain = []byte(attr.Value)
		case "from":
			e.FromAddr = []byte(attr.Value)
		case "to":
			e.ToAddr = []byte(attr.Value)
		case "coin":
			e.Asset, e.AssetE8, err = parseCoin([]byte(attr.Value))
			if err != nil {
				return fmt.Errorf("malformed coin: %w", err)
			}
		case "memo":
			e.Memo = []byte(attr.Value)

		case "contributor_address":
			e.Addr = []byte(attr.Value)
		case "amount":
			e.E8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed amount: %w", err)
			}

		default:
			miderr.LogEventParseErrorF(
				"unknown reserve event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// Rewards defines the "rewards" event type.
type Rewards struct {
	BondE8 int64 // rune amount times 100 M
	// PerPool has the RUNE amounts specified per pool (in .Asset).
	PerPool []Amount
}

func (e *Rewards) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "bond_reward":
			e.BondE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed bond_reward: %w", err)
			}

		default:
			v, err := strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				miderr.LogEventParseErrorF(
					"unknown rewards event attribute %q=%q",
					[]byte(attr.Key), []byte(attr.Value))
				break
			}
			e.PerPool = append(e.PerPool, Amount{[]byte(attr.Key), v})
		}
	}

	return nil
}

// SetIPAddr defines the "set_ip_address" event type.
type SetIPAddress struct {
	NodeAddr []byte // THOR address
	IPAddr   []byte
}

func (e *SetIPAddress) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		switch string([]byte(attr.Key)) {
		case "thor_address":
			e.NodeAddr = []byte(attr.Value)
		case "address":
			e.IPAddr = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF(
				"unknown set_ip_address event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// SetMimir defines the "set_mimir" event type.
type SetMimir struct {
	Key   []byte
	Value []byte
}

func (e *SetMimir) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		switch string([]byte(attr.Key)) {
		case "key":
			e.Key = []byte(attr.Value)
		case "value":
			e.Value = []byte(attr.Value)

		default:
			miderr.LogEventParseErrorF(
				"unknown set_mimir event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// SetNodeKeys defines the "set_node_keys" event type.
type SetNodeKeys struct {
	NodeAddr           []byte // THOR address
	Secp256k1          []byte // public key
	Ed25519            []byte // public key
	ValidatorConsensus []byte // public key
}

func (e *SetNodeKeys) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		switch string([]byte(attr.Key)) {
		case "node_address":
			e.NodeAddr = []byte(attr.Value)
		case "node_secp256k1_pubkey":
			e.Secp256k1 = []byte(attr.Value)
		case "node_ed25519_pubkey":
			e.Ed25519 = []byte(attr.Value)
		case "validator_consensus_pub_key":
			e.ValidatorConsensus = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF(
				"unknown set_node_keys event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// SetVersion defines the "set_version" event type.
type SetVersion struct {
	NodeAddr []byte // THOR address
	Version  string
}

func (e *SetVersion) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		switch string([]byte(attr.Key)) {
		case "thor_address":
			e.NodeAddr = []byte(attr.Value)
		case "version":
			e.Version = string([]byte(attr.Value))
		default:
			miderr.LogEventParseErrorF(
				"unknown set_version event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

type AddBase struct {
	Pool       []byte // asset ID
	AssetTx    []byte // transfer transaction ID (may equal RuneTx)
	AssetChain []byte // transfer backend ID
	AssetAddr  []byte // pool contender address
	AssetE8    int64  // transfer asset quantity times 100 M
	RuneTx     []byte // pool transaction ID
	RuneChain  []byte // pool backend ID
	RuneAddr   []byte // pool contender address
	Memo       []byte // Memo
	RuneE8     int64  // pool transaction quantity times 100 M
}

var txIDSuffix = []byte("_txid")

func (e *AddBase) parse(attrs []abci.EventAttribute) (
	remainder []abci.EventAttribute, err error,
) {
	remainder = nil

	for _, attr := range attrs {
		switch string([]byte(attr.Key)) {
		case "pool":
			e.Pool = []byte(attr.Value)
		case "THOR_txid":
			// Old unsuported values: "THORChain_txid", "BNBChain_txid", "BNB_txid"
			// https://gitlab.com/thorchain/thornode/-/blob/90b225b248856565195a21b323595dcf6bc3e1a2/common/chain.go#L18
			// https://gitlab.com/thorchain/thornode/-/blob/develop/x/thorchain/types/type_event.go#L148
			e.RuneTx = []byte(attr.Value)
			e.RuneChain = []byte(attr.Key)[:len([]byte(attr.Key))-len(txIDSuffix)]
		case "rune_address":
			e.RuneAddr = []byte(attr.Value)
		case "rune_amount":
			e.RuneE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				err = fmt.Errorf("malformed rune_amount: %w", err)
				return
			}
		case "asset_amount":
			e.AssetE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				err = fmt.Errorf("malformed asset_amount: %w", err)
				return
			}
		case "asset_address":
			e.AssetAddr = []byte(attr.Value)
		case "memo":
			e.Memo = []byte(attr.Value)
		default:
			switch {
			case bytes.HasSuffix([]byte(attr.Key), txIDSuffix):
				if e.AssetChain != nil {
					// It should not be that there are two *_txid attrs of which neither is the RUNE one
					err = fmt.Errorf("%q preceded by %q%s", []byte(attr.Key), e.AssetChain, txIDSuffix)
					return
				}
				e.AssetChain = []byte(attr.Key)[:len([]byte(attr.Key))-len(txIDSuffix)]

				e.AssetTx = []byte(attr.Value)

			default:
				remainder = append(remainder, attr)
			}
		}
	}

	if config.Global.CaseInsensitiveChains[string(e.AssetChain)] {
		e.AssetAddr = util.ToLowerBytes(e.AssetAddr)
	}

	return
}

// PendingLiquidity defines the "pending_liquidity" event type,
// which records a partially received add_liquidity.
type PendingLiquidity struct {
	AddBase
	PendingType []byte
}

func (e *PendingLiquidity) LoadTendermint(attrs []abci.EventAttribute) error {
	remainder, err := e.parse(attrs)
	if err != nil {
		return err
	}

	for _, attr := range remainder {
		switch string([]byte(attr.Key)) {
		case "type":
			sValue := string([]byte(attr.Value))
			if sValue == "add" || sValue == "withdraw" {
				e.PendingType = []byte(attr.Value)
			} else {
				miderr.LogEventParseErrorF("unknown pending_liquidity type: %q", []byte(attr.Value))
			}
		default:
			miderr.LogEventParseErrorF(
				"unknown pending_liquidity event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// Stake defines the "stake" event type, which records a participation result."
type Stake struct {
	AddBase
	StakeUnits int64 // pool's liquidiy tokens—gained quantity
}

func (e *Stake) LoadTendermint(attrs []abci.EventAttribute) error {
	remainder, err := e.parse(attrs)
	if err != nil {
		return err
	}

	for _, attr := range remainder {
		switch string([]byte(attr.Key)) {
		case "liquidity_provider_units":
			// TODO(acsaba): rename e.StakeUnits to e.LiquidityProviderUnits
			e.StakeUnits, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed liquidity_provider_units: %w", err)
			}
		default:
			miderr.LogEventParseErrorF("unknown stake event attribute %q=%q", []byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// Slash defines the "slash" event type.
type Slash struct {
	Pool    []byte
	Amounts []Amount
}

func (e *Slash) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		switch string([]byte(attr.Key)) {
		case "pool":
			e.Pool = []byte(attr.Value)

		default:
			v, err := strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				miderr.LogEventParseErrorF(
					"unknown slash event attribute %q=%q",
					[]byte(attr.Key), []byte(attr.Value))
				break
			}
			e.Amounts = append(e.Amounts, Amount{[]byte(attr.Key), v})
		}
	}

	return nil
}

// Swap defines the "swap" event type, which records an exchange
// between the .Pool asset and RUNE.
//
// FromAsset is the input unit of a Swap. The value equals .Pool
// if and only if the trader sells the pool's asset for RUNE. In
// all other cases .FromAsset will be a RUNE, because the trader
// buys .Pool asset.
//
// The liquidity fee is included. Network fees are recorded as
// separate Fee events (with a matching .Tx value).
type Swap struct {
	Tx                []byte // THOR transaction identifier
	Chain             []byte // backend identifier
	FromAddr          []byte // input address on Chain
	ToAddr            []byte // output address on Chain
	FromAsset         []byte // input unit
	FromE8            int64  // FromAsset quantity times 100 M
	ToAsset           []byte // output unit
	ToE8              int64  // ToAsset quantity times 100 M
	Memo              []byte // encoded parameters
	Pool              []byte // asset identifier
	ToE8Min           int64  // output quantity constraint
	SwapSlipBP        int64  // ‱ the trader experienced
	LiqFeeE8          int64  // Pool asset quantity times 100 M
	LiqFeeInRuneE8    int64  // equivalent in RUNE times 100 M
	StreamingQuantity int64  // Streaming: Number of swaps events which already happened
	StreamingCount    int64  // Streaming: Number of swaps which thorchain is planning to execute
}

func (e *Swap) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "id":
			e.Tx = []byte(attr.Value)
		case "chain":
			e.Chain = []byte(attr.Value)
		case "from":
			e.FromAddr = []byte(attr.Value)
		case "to":
			e.ToAddr = []byte(attr.Value)
		case "coin":
			e.FromAsset, e.FromE8, err = parseCoin([]byte(attr.Value))
			if err != nil {
				return fmt.Errorf("malformed coins: %w", err)
			}
		case "emit_asset":
			e.ToAsset, e.ToE8, err = parseCoin([]byte(attr.Value))
			if err != nil {
				return fmt.Errorf("malformed emit_asset: %w", err)
			}
		case "memo":
			e.Memo = []byte(attr.Value)

		case "pool":
			e.Pool = []byte(attr.Value)
		case "price_target", "swap_target":
			e.ToE8Min, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed price_target: %w", err)
			}
		case "trade_slip", "swap_slip":
			e.SwapSlipBP, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed swap_slip: %w", err)
			}
		case "liquidity_fee":
			e.LiqFeeE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed liquidity_fee: %w", err)
			}
		case "liquidity_fee_in_rune":
			e.LiqFeeInRuneE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed liquidity_fee_in_rune: %w", err)
			}
		case "streaming_swap_count":
			e.StreamingCount, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed streaming_swap_count: %w", err)
			}
		case "streaming_swap_quantity":
			e.StreamingQuantity, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed streaming_swap_quantity: %w", err)
			}
		case "pool_slip":
		default:
			miderr.LogEventParseErrorF("unknown swap event attribute %q=%q", []byte(attr.Key), []byte(attr.Value))
		}
	}

	if config.Global.CaseInsensitiveChains[string(e.Chain)] {
		e.FromAddr = util.ToLowerBytes(e.FromAddr)
		e.ToAddr = util.ToLowerBytes(e.ToAddr)
	}

	// This check is due to the recent refund actions emitted with null `tx` attribute
	// More info: https://gitlab.com/thorchain/thornode/-/merge_requests/2716
	if e.Tx == nil {
		return fmt.Errorf("swap transaction with malformed id")
	}

	return nil
}

// Upgrade Rune to Native rune.
type Switch struct {
	Tx        []byte
	FromAddr  []byte
	ToAddr    []byte
	BurnAsset []byte
	MintAsset []byte
	BurnE8    int64
	MintE8    int64
}

func (e *Switch) LoadTendermint(attrs []abci.EventAttribute) error {
	hadMintValue := false

	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "txid", "tx_id":
			e.Tx = []byte(attr.Value)
		case "from", "asset_address":
			e.FromAddr = []byte(attr.Value)
		case "to", "rune_address":
			e.ToAddr = []byte(attr.Value)
		case "burn":
			e.BurnAsset, e.BurnE8, err = parseCoin([]byte(attr.Value))
			if err != nil {
				return fmt.Errorf("malformed coins in switch event: %w", err)
			}
		case "burn_asset":
			e.BurnAsset = []byte(attr.Value)
		case "burn_amount":
			e.BurnE8, err = strconv.ParseInt(attr.Value, 10, 64)
		case "amount":
			hadMintValue = true
			e.MintE8, err = strconv.ParseInt(attr.Value, 10, 64)
		case "asset":
			e.MintAsset = []byte(attr.Value)
		case "mint":
			hadMintValue = true
			e.MintE8, err = strconv.ParseInt(attr.Value, 10, 64)
			if err != nil {
				return fmt.Errorf("malformed mint value in switch event: %w", err)
			}
		default:
			miderr.LogEventParseErrorF("unknown switch event attribute %q=%q", []byte(attr.Key), []byte(attr.Value))
		}
	}
	if !hadMintValue {
		// In the beginning all switch was 1:1, e.g. 12345 BNB.RUNE-B1A was switched to 12345 Rune.
		// After a while this becomes less then 1:1 and a new field was introduced to differentiate
		// mint from burn.
		// For old values we set Mint value to Burn.
		e.MintE8 = e.BurnE8
	}

	return nil
}

// Transfer defines the "transfer" event type.
// https://github.com/cosmos/cosmos-sdk/blob/da064e13d56add466548135739c5860a9f7ed842/x/bank/keeper/send.go#L136
type Transfer struct {
	FromAddr []byte // sender
	ToAddr   []byte // recipient
	Asset    []byte // asset converted to uppercase
	AmountE8 int64  // amount of asset
}

func (e *Transfer) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "sender":
			e.FromAddr = []byte(attr.Value)
		case "recipient":
			e.ToAddr = []byte(attr.Value)
		case "amount":
			e.Asset, e.AmountE8, err = parseCosmosCoin([]byte(attr.Value))
			if err != nil {
				return err
			}
		default:
			miderr.LogEventParseErrorF("unknown transfer event attribute %q=%q", []byte(attr.Key), []byte(attr.Value))
		}
	}

	if e.Asset == nil && e.AmountE8 == 0 {
		return fmt.Errorf("empty amount")
	}

	return nil
}

// Withdraw defines the "withdraw" event type, which records a pool withdrawal request.
// Requests are made by wiring a (probably small) “donation” to the reserve.
// The actual withdrawal that follows is confirmed by an Outbound.
type Withdraw struct {
	Tx                  []byte  // THORChain transaction ID
	Chain               []byte  // transfer backend ID
	FromAddr            []byte  // transfer staker address
	ToAddr              []byte  // transfer pool address
	Asset               []byte  // transfer unit ID
	AssetE8             int64   // transfer quantity times 100 M
	EmitAssetE8         int64   // asset amount withdrawn
	EmitRuneE8          int64   // rune amount withdrawn
	Memo                []byte  // description code which triggered the event
	Pool                []byte  // asset ID
	StakeUnits          int64   // pool's liquidiy tokens—lost quantity
	BasisPoints         int64   // ‱ of total owned liquidity withdrawn
	Asymmetry           float64 // lossy conversion of what?
	ImpLossProtectionE8 int64   // rune amount added as impermanent loss protection
}

func (e *Withdraw) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "id":
			e.Tx = []byte(attr.Value)
		case "chain":
			e.Chain = []byte(attr.Value)
		case "from":
			e.FromAddr = []byte(attr.Value)
		case "to":
			e.ToAddr = []byte(attr.Value)
		case "coin":
			if []byte(attr.Value) == nil {
				// When a pool gets suspended a withdraw removing all pool units is emitted.
				// For that event most fields are nil, we discard this event.
				return fmt.Errorf(
					"Skipping withdraw event because of nil coin, probably pool get's suspended")
			}
			// This is a minimal amount which is needed to have the initiating transfer.
			// Typical value: "1 THOR.RUNE"
			// The actual amount to withdraw is mentioned in the memo field of the initiating
			// transfer.
			// This field is useful to know which network was used to initiate the transfer.

			e.Asset, e.AssetE8, err = parseCoin([]byte(attr.Value))
			if err != nil {
				return fmt.Errorf("malformed coin: %w", err)
			}
		case "emit_asset":
			e.EmitAssetE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed emit_asset: %w", err)
			}
		case "emit_rune":
			e.EmitRuneE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed emit_asset: %w", err)
			}
		case "memo":
			e.Memo = []byte(attr.Value)

		case "pool":
			e.Pool = []byte(attr.Value)
		case "liquidity_provider_units":
			// TODO(acsaba): StakeUnits->LiquidityProviderUnits
			e.StakeUnits, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed stake_units: %w", err)
			}
		case "basis_points":
			e.BasisPoints, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed basis_points: %w", err)
			}
		case "asymmetry":
			e.Asymmetry, err = strconv.ParseFloat(string([]byte(attr.Value)), 64)
			if err != nil {
				return fmt.Errorf("malformed asymmetry: %w", err)
			}
		case "imp_loss_protection":
			e.ImpLossProtectionE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed emit_asset: %w", err)
			}

		default:
			miderr.LogEventParseErrorF(
				"unknown withdraw event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	// When there is no coin
	if e.Asset == nil && e.AssetE8 == 0 {
		// When a pool gets suspended a withdraw removing all pool units is emitted.
		// For that event most fields are nil, we discard this event.
		return fmt.Errorf(
			"Skipping withdraw event because of nil coin, probably pool get's suspended")
	}

	if config.Global.CaseInsensitiveChains[string(e.Chain)] {
		e.FromAddr = util.ToLowerBytes(e.FromAddr)
		e.ToAddr = util.ToLowerBytes(e.ToAddr)
	}

	// TODO(muninn): POL withdraws are missing memos for now, but the plan is for ThorNode to fill
	//   the memo in the future. Remove this default value when ThorNode is adopted.
	// Context:
	// https://discord.com/channels/838986635756044328/1027399282678054962
	if e.Memo == nil {
		e.Memo = []byte("MEMO-MISSING-PROBABLY-POL")
	}

	return nil
}

// UpdateNodeAccountStatus defines the "UpdateNodeAccountStatus" event type.
type UpdateNodeAccountStatus struct {
	NodeAddr []byte // THORChain address
	Former   []byte // previous status label
	Current  []byte // new status label
}

func (e *UpdateNodeAccountStatus) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		switch string([]byte(attr.Key)) {
		case "Address":
			e.NodeAddr = []byte(attr.Value)
		case "Former:":
			e.Former = []byte(attr.Value)
		case "Current:":
			e.Current = []byte(attr.Value)

		default:
			miderr.LogEventParseErrorF(
				"unknown UpdateNodeAccountStatus event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

// ValidatorRequestLeave defines the "validator_request_leave" event type.
type ValidatorRequestLeave struct {
	Tx       []byte // THORChain transaction identifier
	FromAddr []byte // signer THOR node
	NodeAddr []byte // subject THOR node
}

func (e *ValidatorRequestLeave) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		switch string([]byte(attr.Key)) {
		case "txid", "tx":
			e.Tx = []byte(attr.Value)
		case "signer", "signer bnb address":
			e.FromAddr = []byte(attr.Value)
		case "node", "destination":
			e.NodeAddr = []byte(attr.Value)

		default:
			miderr.LogEventParseErrorF(
				"unknown validator_request_leave event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

func ParseBool(s string) (bool, error) {
	switch s {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("Not a bool: %v", s)
	}
}

func ParseInt(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

// PoolBalanceChange defines the "pool_balance_change" event type.
// https://gitlab.com/thorchain/thornode/-/blob/63ae90ef91a178fdfb6834189820bf368027fd00/proto/thorchain/v1/x/thorchain/types/type_events.proto#L142
type PoolBalanceChange struct {
	Asset    []byte // pool
	RuneAmt  int64  // RuneE8
	RuneAdd  bool   // add or remove, ThorNode uses uints
	AssetAmt int64  // AssetE8
	AssetAdd bool   // add or remove, ThorNode uses uints
	Reason   string
}

func (e *PoolBalanceChange) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		key := string([]byte(attr.Key))
		value := string([]byte(attr.Value))
		switch key {
		case "asset":
			e.Asset = []byte(attr.Value)
		case "rune_amt":
			e.RuneAmt, err = ParseInt(value)
		case "rune_add":
			e.RuneAdd, err = ParseBool(value)
		case "asset_amt":
			e.AssetAmt, err = ParseInt(value)
		case "asset_add":
			e.AssetAdd, err = ParseBool(value)
		case "reason":
			// TODO(acsaba): Reason is not in the events, raise with core team.
			e.Reason = value
		default:
			miderr.LogEventParseErrorF("unknown validator_request_leave event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		// TODO(HooriRn): rewrite other Load functions to handle errors after the switch.
		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", value, err)
		}
	}

	return nil
}

type THORNameChange struct {
	Name              []byte
	Chain             []byte
	Address           []byte
	RegistrationFeeE8 int64
	FundAmountE8      int64
	ExpireHeight      int64
	Owner             []byte
	TxID              []byte
	Memo              []byte
	Sender            []byte
}

func (e *THORNameChange) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "name":
			e.Name = []byte(attr.Value)
		case "chain":
			e.Chain = []byte(attr.Value)
		case "address":
			e.Address = []byte(attr.Value)
		case "registration_fee":
			e.RegistrationFeeE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed registration_fee: %w", err)
			}
		case "fund_amount":
			e.FundAmountE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed fund_amount: %w", err)
			}
		case "expire":
			e.ExpireHeight, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed expire_height: %w", err)
			}
		case "owner":
			e.Owner = []byte(attr.Value)
		case "tx_id":
			e.TxID = []byte(attr.Value)
		case "memo":
			e.Memo = []byte(attr.Value)
		case "signer":
			e.Sender = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF(
				"unknown thorname event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}

	if config.Global.CaseInsensitiveChains[string(e.Chain)] {
		e.Address = util.ToLowerBytes(e.Address)
	}

	return nil
}

var errNoSep = errors.New("separator not found")

func parseCoin(b []byte) (asset []byte, amountE8 int64, err error) {
	i := bytes.IndexByte(b, ' ')
	if i < 0 {
		return nil, 0, errNoSep
	}
	asset = b[i+1:]
	amountE8, err = strconv.ParseInt(string(b[:i]), 10, 64)
	return
}

var amountRegex = regexp.MustCompile(`^[0-9]+`)

// Parses the cosmos amount format. E.g. "123btc/btc"
// Returns uppercased. e.g. "BTC/BTC" 123
func parseCosmosCoin(b []byte) (asset []byte, amountE8 int64, err error) {
	if len(b) == 0 {
		err = fmt.Errorf("empty amount")
		return
	}
	s := string(b)
	matchIndexes := amountRegex.FindStringIndex(s)
	if matchIndexes == nil {
		err = fmt.Errorf("no numbers in amount %q", b)
		return
	}
	numStr := s[:matchIndexes[1]]
	amountE8, err = ParseInt(numStr)
	if err != nil {
		err = fmt.Errorf("couldn't parse amount value: %q", b)
		return
	}

	unit := strings.TrimSpace(s[matchIndexes[1]:])
	switch unit {
	case "":
		err = fmt.Errorf("no units given in amount %q", b)
		return
	case "rune":
		asset = []byte(nativeRune)
	default:
		asset = []byte(strings.ToUpper(unit))
	}
	return
}

type SlashPoints struct {
	NodeAddress []byte
	SlashPoints int64
	Reason      []byte
}

func (e *SlashPoints) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "reason":
			e.Reason = []byte(attr.Value)
		case "node_address":
			e.NodeAddress = []byte(attr.Value)
		case "slash_points":
			e.SlashPoints, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed slash points: %w", err)
			}
		default:
			miderr.LogEventParseErrorF(
				"unknown slash points event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}
	return nil
}

type SetNodeMimir struct {
	Address []byte
	Key     int64
	Value   []byte
}

func (e *SetNodeMimir) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "address":
			e.Address = []byte(attr.Value)
		case "key":
			e.Value = []byte(attr.Value)
		case "value":
			e.Key, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed value: %w", err)
			}
		default:
			miderr.LogEventParseErrorF(
				"unknown set_node_mimir event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}
	return nil
}

type MintBurn struct {
	Asset   []byte
	AssetE8 int64 // Asset quantity times 100 M
	Reason  []byte
	Supply  []byte
}

func (e *MintBurn) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "denom":
			if []byte(attr.Value) != nil {
				// Code based on normalizeAsset .
				asset := string([]byte(attr.Value))
				if asset == "rune" {
					e.Asset = []byte("THOR.RUNE")
				} else {
					e.Asset = []byte(strings.ToUpper(asset))
				}
			}
		case "amount":
			e.AssetE8, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed amount: %w", err)
			}
		case "reason":
			e.Reason = []byte(attr.Value)
		case "supply":
			sValue := string([]byte(attr.Value))
			if sValue == "mint" || sValue == "burn" {
				e.Supply = []byte(attr.Value)
			} else {
				miderr.LogEventParseErrorF("unknown supply type: %q", []byte(attr.Value))
			}
		default:
			miderr.LogEventParseErrorF("unknown mint_burn event attribute %q=%q", []byte(attr.Key), []byte(attr.Value))
		}
	}

	return nil
}

type Version struct {
	Version []byte
}

func (e *Version) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		switch string([]byte(attr.Key)) {
		case "version":
			e.Version = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown version event attribute %q=%q", []byte(attr.Key), []byte(attr.Value))
		}
	}
	return nil
}

type LoanOpen struct {
	CollateralDeposited    int64
	DebtIssued             int64
	CollateralAsset        []byte
	CollateralizationRatio int64
	Owner                  []byte
	TargetAsset            []byte
	TxID                   []byte
}

func (e *LoanOpen) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "collateral_up", "collateral_deposited":
			e.CollateralDeposited, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed value: %w", err)
			}
		case "debt_up", "debt_issued":
			e.DebtIssued, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed value: %w", err)
			}
		case "collateralization_ratio":
			e.CollateralizationRatio, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed value: %w", err)
			}
		case "collateral_asset":
			e.CollateralAsset = []byte(attr.Value)
		case "target_asset":
			e.TargetAsset = []byte(attr.Value)
		case "owner":
			e.Owner = []byte(attr.Value)
		case "tx_id":
			e.TxID = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF(
				"unknown loan_open event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}
	return nil
}

type LoanRepayment struct {
	CollateralWithdrawn int64
	DebtRepaid          int64
	CollateralAsset     []byte
	Owner               []byte
	TxID                []byte
}

func (e *LoanRepayment) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "collateral_down", "collateral_withdrawn":
			e.CollateralWithdrawn, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed value: %w", err)
			}
		case "debt_down", "debt_repaid":
			e.DebtRepaid, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed value: %w", err)
			}
		case "collateral_asset":
			e.CollateralAsset = []byte(attr.Value)
		case "owner":
			e.Owner = []byte(attr.Value)
		case "tx_id":
			e.TxID = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF(
				"unknown loan_repayment event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}
	return nil
}

type StreamingSwapDetails struct {
	TxID          []byte
	Interval      int64
	Quantity      int64
	Count         int64
	LastHeight    int64
	DepositAsset  []byte
	DepoitE8      int64
	InAsset       []byte
	InE8          int64
	OutAsset      []byte
	OutE8         int64
	FailedSwaps   []int64
	FailedReasons []string
}

func ParseArrayInt(s []string) ([]int64, error) {
	ret := []int64{}
	for _, si := range s {
		r, err := ParseInt(si)
		if err != nil {
			return nil, err
		}
		ret = append(ret, r)
	}
	return ret, nil
}

func (e *StreamingSwapDetails) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch string([]byte(attr.Key)) {
		case "tx_id":
			e.TxID = []byte(attr.Value)
		case "last_height":
			e.LastHeight, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed last height value: %w", err)
			}
		case "count":
			e.Count, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed count value: %w", err)
			}
		case "quantity":
			e.Quantity, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed quantity value: %w", err)
			}
		case "interval":
			e.Interval, err = strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed interval value: %w", err)
			}
		case "in":
			e.InAsset, e.InE8, err = parseCoin([]byte(attr.Value))
			if err != nil {
				return fmt.Errorf("malformed in value: %w", err)
			}
		case "out":
			e.OutAsset, e.OutE8, err = parseCoin([]byte(attr.Value))
			if err != nil {
				return fmt.Errorf("malformed out value: %w", err)
			}
		case "deposit":
			e.DepositAsset, e.DepoitE8, err = parseCoin([]byte(attr.Value))
			if err != nil {
				return fmt.Errorf("malformed deposit value: %w", err)
			}
		case "failed_swaps":
			if []byte(attr.Value) != nil {
				e.FailedSwaps, err = ParseArrayInt(strings.Split(string([]byte(attr.Value)), ", "))
				if err != nil {
					return fmt.Errorf("malformed streaming failed swaps: %w", err)
				}
			}
		case "failed_swap_reasons":
			if []byte(attr.Value) != nil {
				e.FailedReasons = strings.Split(string([]byte(attr.Value)), "\n ")
			}
		default:
			miderr.LogEventParseErrorF(
				"unknown streaming_swap event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}
	}
	return nil
}

type TSSKeygenFailure struct {
	Reason     []byte
	IsUniCast  bool
	BlameNodes []string
	Round      []byte
	Height     int64
}

func (e *TSSKeygenFailure) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		key := string([]byte(attr.Key))
		value := string([]byte(attr.Value))
		switch key {
		case "blame":
			e.BlameNodes = strings.Split(value, ", ")
		case "height":
			e.Height, err = ParseInt(value)
		case "round":
			e.Round = []byte(attr.Value)
		case "is_unicast":
			e.IsUniCast, err = ParseBool(value)
		case "reason":
			e.Reason = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown tss_keygen_failure event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", value, err)
		}
	}

	return nil
}

type TSSKeygenSuccess struct {
	PubKey  []byte
	Members []string
	Height  int64
}

func (e *TSSKeygenSuccess) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		key := string([]byte(attr.Key))
		value := string([]byte(attr.Value))
		switch key {
		case "pubkey":
			e.PubKey = []byte(attr.Value)
		case "height":
			e.Height, err = ParseInt(value)
		case "members":
			e.Members = strings.Split(value, ", ")
		default:
			miderr.LogEventParseErrorF("unknown tss_keygen_success event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", value, err)
		}
	}

	return nil
}

type TradeAccountWithdraw struct {
	Tx        []byte
	Asset     []byte
	AssetAddr []byte
	RuneAddr  []byte
	AmtE8     int64
}

func (e *TradeAccountWithdraw) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		key := string([]byte(attr.Key))
		value := string([]byte(attr.Value))
		switch key {
		case "tx_id":
			e.Tx = []byte(attr.Value)
		case "amount":
			e.AmtE8, err = ParseInt(value)
		case "asset":
			e.Asset = []byte(attr.Value)
		case "asset_address":
			e.AssetAddr = []byte(attr.Value)
		case "rune_address":
			e.RuneAddr = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown trade_account_withdraw event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", value, err)
		}
	}

	return nil
}

type TradeAccountDeposit struct {
	Tx        []byte
	Asset     []byte
	AssetAddr []byte
	RuneAddr  []byte
	AmtE8     int64
}

func (e *TradeAccountDeposit) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		key := string([]byte(attr.Key))
		value := string([]byte(attr.Value))
		switch key {
		case "tx_id":
			e.Tx = []byte(attr.Value)
		case "amount":
			e.AmtE8, err = ParseInt(value)
		case "asset":
			e.Asset = []byte(attr.Value)
		case "asset_address":
			e.AssetAddr = []byte(attr.Value)
		case "rune_address":
			e.RuneAddr = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown trade_account_deposit event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", value, err)
		}
	}

	return nil
}

type Coinbase struct {
	Asset    []byte
	AssetE8  int64
	Receiver []byte
}

type Burn struct {
	Asset   []byte
	AssetE8 int64
	Burner  []byte
}

type RunePoolDeposit struct {
	Tx        []byte
	RuneAddr  []byte
	RuneAmtE8 int64
	Units     int64
}

func (e *RunePoolDeposit) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		key := string([]byte(attr.Key))
		value := string([]byte(attr.Value))
		switch key {
		case "tx_id":
			e.Tx = []byte(attr.Value)
		case "rune_address":
			e.RuneAddr = []byte(attr.Value)
		case "rune_amoumt", "rune_amount":
			e.RuneAmtE8, err = ParseInt(value)
		case "units":
			e.Units, err = ParseInt(value)
		default:
			miderr.LogEventParseErrorF("unknown rune_pool_deposit event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", value, err)
		}
	}

	return nil
}

type RunePoolWithdraw struct {
	Tx            []byte
	RuneAddr      []byte
	RuneAmtE8     int64
	BasisPoints   int64
	AffiliateBPs  int64
	AffiliateAmt  int64
	AffiliateAddr []byte
	Units         int64
}

func (e *RunePoolWithdraw) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		key := string([]byte(attr.Key))
		value := string([]byte(attr.Value))
		switch key {
		case "tx_id":
			e.Tx = []byte(attr.Value)
		case "rune_address":
			e.RuneAddr = []byte(attr.Value)
		case "rune_amoumt", "rune_amount":
			e.RuneAmtE8, err = ParseInt(value)
		case "basis_points":
			e.BasisPoints, err = ParseInt(value)
		case "affiliate_basis_pts", "affiliate_basis_points":
			e.AffiliateBPs, err = ParseInt(value)
		case "affiliate_amount":
			e.AffiliateAmt, err = ParseInt(value)
		case "affiliate_address":
			e.AffiliateAddr = []byte(attr.Value)
		case "units":
			e.Units, err = ParseInt(value)
		default:
			miderr.LogEventParseErrorF("unknown rune_pool_withdraw event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", value, err)
		}
	}

	return nil
}

type SecureAssetWithdraw struct {
	Tx        []byte
	Asset     []byte
	AssetAddr []byte
	RuneAddr  []byte
	AmtE8     int64
}

func (e *SecureAssetWithdraw) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		key := string([]byte(attr.Key))
		value := string([]byte(attr.Value))
		switch key {
		case "tx_id":
			e.Tx = []byte(attr.Value)
		case "amount":
			e.AmtE8, err = ParseInt(value)
		case "asset":
			e.Asset = []byte(attr.Value)
		case "asset_address":
			e.AssetAddr = []byte(attr.Value)
		case "rune_address":
			e.RuneAddr = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown secure_asset_withdraw event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", value, err)
		}
	}

	return nil
}

type SecureAssetDeposit struct {
	Tx        []byte
	Asset     []byte
	AssetAddr []byte
	RuneAddr  []byte
	AmtE8     int64
}

func (e *SecureAssetDeposit) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		key := string([]byte(attr.Key))
		value := string([]byte(attr.Value))
		switch key {
		case "tx_id":
			e.Tx = []byte(attr.Value)
		case "amount":
			e.AmtE8, err = ParseInt(value)
		case "asset":
			e.Asset = []byte(attr.Value)
		case "asset_address":
			e.AssetAddr = []byte(attr.Value)
		case "rune_address":
			e.RuneAddr = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown secure_asset_deposit event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", value, err)
		}
	}

	return nil
}

type AffiliateFee struct {
	Tx       []byte
	FeeAmt   int64
	GrossAmt int64
	FeeBps   int64
	THORName []byte
	RuneAddr []byte
	Memo     []byte
	Asset    []byte
}

func (e *AffiliateFee) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		key := string(attr.Key)
		value := string(attr.Value)
		switch key {
		case "tx_id":
			e.Tx = []byte(attr.Value)
		case "rune_address":
			e.RuneAddr = []byte(attr.Value)
		case "fee_amount":
			e.FeeAmt, err = ParseInt(value)
		case "gross_amount":
			e.GrossAmt, err = ParseInt(value)
		case "fee_bps":
			e.FeeBps, err = ParseInt(value)
		case "memo":
			e.Memo = []byte(attr.Value)
		case "thorname":
			e.THORName = []byte(attr.Value)
		case "asset":
			e.Asset = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown affiliate_fee event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", value, err)
		}
	}

	return nil
}

type CosmWasmEvent struct {
	TxID            []byte
	ContractAddress []byte
	Sender          []byte
	Type            []byte
	Msg             []byte
	Funds           []byte
	Attributes      map[string]string
}

func (e *CosmWasmEvent) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch attr.Key {
		case "tx_id":
			e.TxID = []byte(attr.Value)
		case "_contract_address":
			e.ContractAddress = []byte(attr.Value)
		case "sender":
			e.Sender = []byte(attr.Value)
		case "type":
			e.Type = []byte(attr.Value)
		case "msg":
			e.Msg = []byte(attr.Value)
		case "funds":
			e.Funds = []byte(attr.Value)
		default:
			// Fill Attribute
			if e.Attributes == nil {
				e.Attributes = make(map[string]string)
			}
			e.Attributes[attr.Key] = attr.Value
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", attr.Value, err)
		}
	}

	return nil
}

type Instantiate struct {
	TxID            []byte
	ContractAddress []byte
	Label           []byte
	CodeID          int64
	Sender          []byte
	Admin           []byte
	Msg             []byte
	Funds           []byte
}

func (e *Instantiate) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch attr.Key {
		case "tx_id":
			e.TxID = []byte(attr.Value)
		case "_contract_address":
			e.ContractAddress = []byte(attr.Value)
		case "label":
			e.Label = []byte(attr.Value)
		case "code_id":
			e.CodeID, err = ParseInt(attr.Value)
		case "sender":
			e.Sender = []byte(attr.Value)
		case "admin_address":
			e.Admin = []byte(attr.Value)
		case "msg":
			e.Msg = []byte(attr.Value)
		case "funds":
			e.Funds = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown instantiate event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", attr.Value, err)
		}
	}

	return nil
}

type TcyClaim struct {
	TxID     []byte
	RuneAddr []byte
	L1Asset  []byte
	L1Addr   []byte
	Memo     []byte
	TcyAmtE8 int64
}

func (e *TcyClaim) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch attr.Key {
		case "tx_id":
			e.TxID = []byte(attr.Value)
		case "rune_address":
			e.RuneAddr = []byte(attr.Value)
		case "asset":
			e.L1Asset = []byte(attr.Value)
		case "l1_address":
			e.L1Addr = []byte(attr.Value)
		case "tcy_amount":
			e.TcyAmtE8, err = ParseInt(attr.Value)
		case "memo":
			e.Memo = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown tcy_claim event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", attr.Value, err)
		}
	}

	return nil
}

type TcyDistribution struct {
	RuneAddr  []byte
	RuneAmtE8 int64
}

func (e *TcyDistribution) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch attr.Key {
		case "rune_address":
			e.RuneAddr = []byte(attr.Value)
		case "rune_amount":
			e.RuneAmtE8, err = ParseInt(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown tcy_distribution event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", attr.Value, err)
		}
	}

	return nil
}

type TcyStake struct {
	TxID    []byte
	Amount  int64
	Address []byte
	Memo    []byte
}

func (e *TcyStake) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch attr.Key {
		case "tx_id":
			e.TxID = []byte(attr.Value)
		case "amount":
			e.Amount, err = ParseInt(attr.Value)
		case "address":
			e.Address = []byte(attr.Value)
		case "memo":
			e.Memo = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown tcy_stake event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", attr.Value, err)
		}
	}

	return nil
}

type TcyUnstake struct {
	TxID    []byte
	Amount  int64
	Address []byte
	Memo    []byte
}

func (e *TcyUnstake) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch attr.Key {
		case "tx_id":
			e.TxID = []byte(attr.Value)
		case "amount":
			e.Amount, err = ParseInt(attr.Value)
		case "address":
			e.Address = []byte(attr.Value)
		case "memo":
			e.Memo = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown tcy_unstake event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", attr.Value, err)
		}
	}

	return nil
}

type LimitSwap struct {
	TxID     []byte
	FromAddr []byte
	ToAddr   []byte
	InAsset  []byte
	InAmtE8  int64
	OutAsset []byte
	OutAmtE8 int64
	Memo     []byte
}

func (e *LimitSwap) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch attr.Key {
		case "txid":
			e.TxID = []byte(attr.Value)
		case "source":
			e.InAsset, e.InAmtE8, err = parseCoin([]byte(attr.Value))
		case "target":
			e.OutAsset, e.OutAmtE8, err = parseCoin([]byte(attr.Value))
		case "from_address":
			e.FromAddr = []byte(attr.Value)
		case "to_address":
			e.ToAddr = []byte(attr.Value)
		case "memo":
			e.Memo = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown limit_swap event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", attr.Value, err)
		}
	}

	return nil
}

type Rebond struct {
	TxIn           []byte
	Amount         int64
	NodeAddress    []byte
	OldBondAddress []byte
	NewBondAddress []byte
	Memo           []byte
	ToAddrress     []byte
}

func (e *Rebond) LoadTendermint(attrs []abci.EventAttribute) error {
	for _, attr := range attrs {
		var err error
		switch attr.Key {
		case "tx_in", "tx", "id":
			e.TxIn = []byte(attr.Value)
		case "amount":
			e.Amount, err = ParseInt(attr.Value)
		case "node_address":
			e.NodeAddress = []byte(attr.Value)
		case "old_bond_address":
			e.OldBondAddress = []byte(attr.Value)
		case "new_bond_address":
			e.NewBondAddress = []byte(attr.Value)
		case "memo":
			e.Memo = []byte(attr.Value)
		case "to":
			e.ToAddrress = []byte(attr.Value)
		default:
			miderr.LogEventParseErrorF("unknown rebond event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", attr.Value, err)
		}
	}

	return nil
}
