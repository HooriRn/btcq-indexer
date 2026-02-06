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
//	   50 000 000 0·· ··· ···  500 M QBTC total
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

	"github.com/btcq/btcq-indexer/internal/util/btcqerr"
)

// Asset Labels
const (
	// Native asset on QBTC chain.
	nativeQbtc = "QBTC.QBTC"
	// TCY asset on THORChain.
	nativeTCY = "THOR.TCY"
	// RUJI asset on THORChain.
	nativeRUJI = "THOR.RUJI"
	// NAMI asset on THORChain.
	nativeNAMI = "THOR.NAMI"
)

// IsQbtc returns whether asset matches the QBTC asset.
func IsQbtc(asset []byte) bool {
	return string(asset) == nativeQbtc
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
	// Qbtc qbtc coin type
	Qbtc CoinType = iota
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
	if IsQbtc(asset) {
		return Qbtc
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

// QbtcAsset returns the QBTC asset
// (Logic is copied from THORnode code)
func QbtcAsset() string {
	return nativeQbtc
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

// Rewards defines the "rewards" event type.
type Rewards struct {
	BondE8    int64  // qbtc amount times 100 M
	Validator []byte // validator address (THOR address), optional
	// PerPool has the QBTC amounts specified per pool (in .Asset).
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
		case "validator":
			e.Validator = []byte(attr.Value)

		default:
			v, err := strconv.ParseInt(string([]byte(attr.Value)), 10, 64)
			if err != nil {
				btcqerr.LogEventParseErrorF(
					"unknown rewards event attribute %q=%q",
					[]byte(attr.Key), []byte(attr.Value))
				break
			}
			e.PerPool = append(e.PerPool, Amount{[]byte(attr.Key), v})
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
	case "rune", "qbtc":
		asset = []byte(nativeQbtc)
	default:
		asset = []byte(strings.ToUpper(unit))
	}
	return
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
			btcqerr.LogEventParseErrorF("unknown instantiate event attribute %q=%q",
				[]byte(attr.Key), []byte(attr.Value))
		}

		if err != nil {
			return fmt.Errorf("malformed key: %v (%w)", attr.Value, err)
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
