package util

import (
	"bytes"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

// Chains work with integers which represent fixed point decimals.
// E.g. on BTC 1 is 1e-8 bitcoin, but on ETH 1 is 1e-18 ethereum.
// This information is not important for Midgard, all the values are converted to E8 by ThorNode
// before they are sent to Midgard.
// This information is gathered only for clients.
type NativeDecimalMap map[string]NativeDecimalSingle

type NativeDecimalSingle struct {
	NativeDecimals int64    `json:"decimals"` // -1 means that only the asset name was observed without the decimal count.
	AssetSeen      []string `json:"asset_seen"`
	DecimalSource  []string `json:"decimal_source"`
}

func IntStr(v int64) string {
	return strconv.FormatInt(v, 10)
}

func FloatStr(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

type Asset struct {
	Chain  string
	Ticker string
	Symbol string
	Synth  bool
	Trade  bool
}

func AssetFromString(s string) (asset Asset) {
	var parts []string
	var sym string
	if strings.Count(s, "/") > 0 {
		parts = strings.SplitN(s, "/", 2)
		asset.Synth = true
	} else if strings.Count(s, "~") > 0 {
		parts = strings.SplitN(s, "~", 2)
		asset.Trade = true
	} else {
		parts = strings.SplitN(s, ".", 2)
		asset.Synth = false
		asset.Trade = false
	}

	if len(parts) == 1 {
		asset.Chain = "THOR"
		sym = parts[0]
	} else {
		asset.Chain = strings.ToUpper(parts[0])
		sym = parts[1]
	}

	parts = strings.SplitN(sym, "-", 2)
	asset.Symbol = strings.ToUpper(sym)
	asset.Ticker = strings.ToUpper(parts[0])

	return
}

func ConvertNativePoolToSynth(poolName string) string {
	return strings.Replace(poolName, ".", "/", 1)
}

func ConvertSynthPoolToNative(poolName string) string {
	return strings.Replace(poolName, "/", ".", 1)
}

func ConsumeUrlParam(urlParams *url.Values, key string) (value string) {
	value = urlParams.Get(key)
	urlParams.Del(key)
	return
}

func CheckUrlEmpty(urlParams url.Values) miderr.Err {
	for k := range urlParams {
		return miderr.BadRequestF("Unknown key: %s", k)
	}
	return nil
}

// It's like bytes.ToLower but returns nil for nil.
func ToLowerBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	return bytes.ToLower(b)
}

type Number interface {
	int64 | float64
}

func Max[T Number](x, y T) T {
	if y < x {
		return x
	} else {
		return y
	}
}

func MustParseInt64(v string) int64 {
	res, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		midlog.ErrorE(err, "Cannot parse int64")
	}
	return res
}

// This function is used for identifying the old swap events without streaming_quantity attribute
// as streaming swap
func CheckMemoIsStreamingSwap(memo string) bool {
	mem := strings.Split(memo, ":")
	if len(mem) > 3 && strings.Contains(mem[3], "/") {
		return true
	}
	return false
}

// From THORNode code
func GetMedian(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}

	sort.SliceStable(vals, func(i, j int) bool {
		return vals[i] < vals[j]
	})

	var median float64
	if len(vals)%2 > 0 {
		// odd number of figures in our slice. Take the middle figure. Since
		// slices start with an index of zero, just need to length divide by two.
		medianSpot := len(vals) / 2
		median = vals[medianSpot]
	} else {
		// even number of figures in our slice. Average the middle two figures.
		pt1 := vals[len(vals)/2-1]
		pt2 := vals[len(vals)/2]
		median = (pt1 + pt2) / 2
	}
	return median
}

// From THORNode
// MEMO: TXTYPE:STATE1:STATE2:STATE3:FINALMEMO
type TxType string

const (
	TxUnknown                TxType = "unknown"
	TxAdd                    TxType = "add"
	TxWithdraw               TxType = "withdraw"
	TxSwap                   TxType = "swap"
	TxLimitOrder             TxType = "limitOrder"
	TxOutbound               TxType = "outbound"
	TxDonate                 TxType = "donate"
	TxBond                   TxType = "bond"
	TxUnbond                 TxType = "unbond"
	TxLeave                  TxType = "leave"
	TxYggdrasilFund          TxType = "yggdrasilFund"
	TxYggdrasilReturn        TxType = "yggdrasilReturn"
	TxReserve                TxType = "reserve"
	TxRefund                 TxType = "refund"
	TxMigrate                TxType = "migrate"
	TxRagnarok               TxType = "ragnarok"
	TxSwitch                 TxType = "switch"
	TxNoOp                   TxType = "noOp"
	TxConsolidate            TxType = "consolidate"
	TxTHORName               TxType = "thorname"
	TxLoanOpen               TxType = "loanOpen"
	TxLoanRepayment          TxType = "loanRepayment"
	TxTradeAccountDeposit    TxType = "tradeAccountDeposit"
	TxTradeAccountWithdrawal TxType = "tradeAccountWithdraw"
)

var StringToTxTypeMap = map[string]TxType{
	"add":         TxAdd,
	"+":           TxAdd,
	"withdraw":    TxWithdraw,
	"wd":          TxWithdraw,
	"-":           TxWithdraw,
	"swap":        TxSwap,
	"s":           TxSwap,
	"=":           TxSwap,
	"limito":      TxLimitOrder,
	"lo":          TxLimitOrder,
	"out":         TxOutbound,
	"donate":      TxDonate,
	"d":           TxDonate,
	"bond":        TxBond,
	"unbond":      TxUnbond,
	"leave":       TxLeave,
	"yggdrasil+":  TxYggdrasilFund,
	"yggdrasil-":  TxYggdrasilReturn,
	"reserve":     TxReserve,
	"refund":      TxRefund,
	"migrate":     TxMigrate,
	"ragnarok":    TxRagnarok,
	"switch":      TxSwitch,
	"noop":        TxNoOp,
	"consolidate": TxConsolidate,
	"name":        TxTHORName,
	"n":           TxTHORName,
	"~":           TxTHORName,
	"$+":          TxLoanOpen,
	"loan+":       TxLoanOpen,
	"$-":          TxLoanRepayment,
	"loan-":       TxLoanRepayment,
	"trade+":      TxTradeAccountDeposit,
	"trade-":      TxTradeAccountWithdrawal,
}

func TxTypeFromMemo(memo string) (txType TxType) {
	action := strings.ToLower(strings.SplitN(string(memo), ":", 2)[0])
	if t, ok := StringToTxTypeMap[action]; ok {
		txType = t
	} else {
		txType = TxUnknown
	}
	return
}

func WrapMapInterface(value map[string]interface{}) *map[string]interface{} {
	if value == nil {
		return nil
	}
	return &value
}

func WrapString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func WrapBoolean(value bool) *bool {
	if !value {
		return nil
	}
	return &value
}
