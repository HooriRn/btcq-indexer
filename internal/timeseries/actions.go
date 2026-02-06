package timeseries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/fetch/record"
	"github.com/btcq/btcq-indexer/internal/util"
	"github.com/btcq/btcq-indexer/internal/util/btcqerr"
	"github.com/btcq/btcq-indexer/internal/util/btcqlog"
	"github.com/btcq/btcq-indexer/openapi/generated/oapigen"
)

const DefaultLimit = 50

func floatStr(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

type action struct {
	pools      []string
	actionType string
	status     string
	in         []transaction
	out        []transaction
	date       int64
	height     int64
	metadata   oapigen.Metadata
	eventId    int64
}

func (a action) toOapigen() oapigen.Action {
	oapigenIn := make([]oapigen.Transaction, len(a.in))
	oapigenOut := make([]oapigen.Transaction, len(a.out))

	for i, tx := range a.in {
		oapigenIn[i] = tx.toOapigen()
	}

	for i, tx := range a.out {
		oapigenOut[i] = tx.toOapigen()
	}

	return oapigen.Action{
		Pools:    a.pools,
		Type:     oapigen.ActionType(a.actionType),
		Status:   oapigen.ActionStatus(a.status),
		In:       oapigenIn,
		Out:      oapigenOut,
		Date:     util.IntStr(a.date),
		Height:   util.IntStr(a.height),
		Metadata: a.metadata,
	}
}

type transaction struct {
	Address   string   `json:"address"`
	Coins     coinList `json:"coins"`
	TxID      string   `json:"txID"`
	Height    *string  `json:"height"`
	Internal  *bool    `json:"internal"`
	Affiliate *bool    `json:"affiliate"`
}

func (tx transaction) toOapigen() oapigen.Transaction {
	ret := oapigen.Transaction{
		Address:   tx.Address,
		TxID:      tx.TxID,
		Coins:     tx.Coins.toOapigen(),
		Affiliate: tx.Affiliate,
	}

	if tx.Height != nil {
		ret.Height = tx.Height
	}

	return ret
}

type transactionList []transaction

func (a *transactionList) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(b, &a)
}

type coin struct {
	Asset  string `json:"asset"`
	Amount int64  `json:"amount"`
}

func (c coin) toOapigen() oapigen.Coin {
	return oapigen.Coin{
		Asset:  c.Asset,
		Amount: util.IntStr(c.Amount),
	}
}

type coinList []coin

func (coins coinList) toOapigen() []oapigen.Coin {
	oapigenCoins := make([]oapigen.Coin, len(coins))
	for i, c := range coins {
		oapigenCoins[i] = c.toOapigen()
	}
	return oapigenCoins
}

func (a *coinList) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(b, &a)
}

// Combination of possible fields the jsonb `meta` column can have
type actionMeta struct {
	// refund:
	Reason string `json:"reason"`
	// withdraw:
	Asymmetry      float64 `json:"asymmetry"`
	BasisPoints    int64   `json:"basisPoints"`
	ImpLossProt    int64   `json:"impermanentLossProtection"`
	LiquidityUnits int64   `json:"liquidityUnits"`
	EmitAssetE8    int64   `json:"emitAssetE8"`
	EmitQbtcE8     int64   `json:"emitQbtcE8"`
	// swap:
	SwapSingle       bool   `json:"swapSingle"`
	LiquidityFee     int64  `json:"liquidityFee"`
	SwapTarget       int64  `json:"swapTarget"`
	SwapSlip         int64  `json:"swapSlip"`
	AffiliateFee     int64  `json:"affiliateFee"`
	AffiliateAddress string `json:"affiliateAddress"`
	Memo             string `json:"memo"`
	SwapStreaming    bool   `json:"swapStreaming"`
	TxType           string `json:"txType"`
	// addLiquidity:
	Status string `json:"status"`
	// also LiquidityUnits
	Thorname          string `json:"thorname"`
	ThornameAddress   string `json:"address"`
	ThornameOwner     string `json:"owner"`
	Expire            int64  `json:"expire"`
	Chain             string `json:"chain"`
	FundAmountE8      int64  `json:"fundAmount"`
	RegistrationFeeE8 int64  `json:"registrationFee"`
	// RUNEPool
	Units               int64 `json:"units"`
	AffiliateAmount     int64 `json:"affiliateAmount"`
	AffiliateBasisPoint int64 `json:"affiliate_basis_pts"`
	// Bond
	NodeAddress  string `json:"nodeAddress"`
	BondProvider string `json:"provider"`
	OperatorFee  string `json:"fee"`
	// failed
	Code int64 `json:"code"`
	// failed reason
	Log string `json:"log"`
	// action prices
	InPriceUSD  float64 `json:"inPriceUSD"`
	OutPriceUSD float64 `json:"outPriceUSD"`
	// contract
	ContractType       string                 `json:"type"`
	ContractAttributes map[string]interface{} `json:"attributes"`
	ContractMsg        map[string]interface{} `json:"msg"`
	ContractFunds      string                 `json:"funds"`
	// rebond
	NewBondAddress string `json:"newBondAddress"`
}

// TODO(huginn): switch to using native pgx interface, this would allow us to scan
// jsonb and array data automatically, without writing these methods and using libpq.
// It's also more efficient.
func (a *actionMeta) Scan(value interface{}) error {
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, &a)
	case nil:
		*a = actionMeta{}
		return nil
	}

	return errors.New("unsupported scan type for actionMeta")
}

// Out estimation is based on the first swap block * quantity
// https://gitlab.com/thorchain/thornode/-/blob/42cbf7c138c12e3387ac14c52558d626e56af2e9/x/thorchain/querier_quotes.go#L298
type streamingMetaType struct {
	Count             int64     `json:"count"`
	Quantity          int64     `json:"quantity"`
	Interval          int64     `json:"interval"`
	LastHeight        int64     `json:"last_height"`
	InAsset           string    `json:"in_asset"`
	InE8              int64     `json:"in_e8"`
	OutAsset          string    `json:"out_asset"`
	OutE8             int64     `json:"out_e8"`
	DepoitAsset       string    `json:"deposit_asset"`
	DepositE8         int64     `json:"deposit_e8"`
	FailedSwaps       *[]int64  `json:"failed_swaps"`
	FailedSwapReasons *[]string `json:"failed_swap_reasons"`
	OutEstimation     int64     `json:"out_estimation"`
}

func (a *streamingMetaType) Scan(value interface{}) error {
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, &a)
	case nil:
		*a = streamingMetaType{}
		return nil
	}

	return errors.New("unsupported scan type for actionMeta")
}

func (s streamingMetaType) toOapigen() *oapigen.StreamingSwapMeta {
	if s.Quantity <= 1 {
		return nil
	}

	showFailedSwaps := func(value *[]int64) *[]string {
		if s.FailedSwaps != nil {
			failedSwaps := []string{}
			for _, s := range *(s.FailedSwaps) {
				failedSwaps = append(failedSwaps, util.IntStr(s))
			}
			return &failedSwaps
		}
		return nil
	}

	return &oapigen.StreamingSwapMeta{
		Count:             util.IntStr(s.Count),
		Quantity:          util.IntStr(s.Quantity),
		Interval:          util.IntStr(s.Interval),
		LastHeight:        util.IntStr(s.LastHeight),
		InCoin:            oapigen.Coin{Amount: util.IntStr(s.InE8), Asset: s.InAsset},
		OutCoin:           oapigen.Coin{Amount: util.IntStr(s.OutE8), Asset: s.OutAsset},
		DepositedCoin:     oapigen.Coin{Amount: util.IntStr(s.DepositE8), Asset: s.DepoitAsset},
		FailedSwaps:       showFailedSwaps(s.FailedSwaps),
		FailedSwapReasons: s.FailedSwapReasons,
		OutEstimation:     util.IntStr(s.OutEstimation),
	}
}

type ActionsParams struct {
	Limit         string
	NextPageToken string
	PrevPageToken string
	Timestamp     string
	Height        string
	FromTimestamp string
	FromHeight    string
	Offset        string
	ActionType    string
	Address       string
	TXId          string
	Asset         string
	TxType        string
	Affiliate     string
}

type parsedActionsParams struct {
	limit              uint64
	nextPageToken      uint64
	prevPageToken      uint64
	timestamp          uint64
	height             uint64
	fromTimestamp      uint64
	fromHeight         uint64
	offset             uint64
	types              []string
	addresses          []string
	txid               string
	assets             []string
	txTypes            []string
	affiliateAddresses []string
	oldAction          bool
}

func isMutuallyExclusive(p map[string]string) error {
	count := 0
	errFmt := "Parameters "
	for key, val := range p {
		if val != "" {
			count++
		}
		errFmt += fmt.Sprintf("'%s' ", key)
	}
	if count > 1 {
		errFmt += "shouldn't be coming together"
		return errors.New(errFmt)
	}
	return nil
}

func (p parsedActionsParams) IsReverseLookup() bool {
	return p.prevPageToken != 0 || p.fromHeight != 0 || p.fromTimestamp != 0
}

func (p ActionsParams) parse() (parsedActionsParams, error) {
	MaxLimit := config.Global.Endpoints.ActionParams.MaxLimit
	MaxAddresses := config.Global.Endpoints.ActionParams.MaxAddresses
	MaxAssets := config.Global.Endpoints.ActionParams.MaxAssets
	MaxTxTypes := config.Global.Endpoints.ActionParams.MaxTxType

	var limit uint64
	if p.Limit != "" {
		var err error
		limit, err = strconv.ParseUint(p.Limit, 10, 64)
		if err != nil || limit < 1 || MaxLimit < limit {
			return parsedActionsParams{}, btcqerr.BadRequestF(
				"'limit' must be an integer between 1 and %d",
				MaxLimit)
		}
	} else {
		limit = DefaultLimit
	}

	var nextPageToken uint64
	if p.NextPageToken != "" {
		var err error
		nextPageToken, err = strconv.ParseUint(p.NextPageToken, 10, 64)
		if err != nil {
			return parsedActionsParams{}, errors.New("'nextPageToken' must be a non negative integer")
		}
	} else {
		nextPageToken = 0
	}

	var timestamp uint64
	if p.Timestamp != "" {
		var err error
		timestamp, err = strconv.ParseUint(p.Timestamp, 10, 64)
		if err != nil {
			return parsedActionsParams{}, errors.New("'timestamp' must be a non negative integer")
		}
		timestamp = timestamp * 1e9
	} else {
		timestamp = 0
	}

	var height uint64
	if p.Height != "" {
		var err error
		height, err = strconv.ParseUint(p.Height, 10, 64)
		if err != nil {
			return parsedActionsParams{}, errors.New("'height' must be a non negative integer")
		}
	} else {
		height = 0
	}

	var prevPageToken uint64
	if p.PrevPageToken != "" {
		var err error
		prevPageToken, err = strconv.ParseUint(p.PrevPageToken, 10, 64)
		if err != nil {
			return parsedActionsParams{}, errors.New("'prevPageToken' must be a non negative integer")
		}
	} else {
		prevPageToken = 0
	}

	var fromTimestamp uint64
	if p.FromTimestamp != "" {
		var err error
		fromTimestamp, err = strconv.ParseUint(p.FromTimestamp, 10, 64)
		if err != nil {
			return parsedActionsParams{}, errors.New("'fromTimestamp' must be a non negative integer")
		}
		fromTimestamp = fromTimestamp * 1e9
	} else {
		fromTimestamp = 0
	}

	var fromHeight uint64
	if p.FromHeight != "" {
		var err error
		fromHeight, err = strconv.ParseUint(p.FromHeight, 10, 64)
		if err != nil {
			return parsedActionsParams{}, errors.New("'fromHeight' must be a non negative integer")
		}
	} else {
		fromHeight = 0
	}

	var offset uint64
	if p.Offset != "" {
		var err error
		offset, err = strconv.ParseUint(p.Offset, 10, 64)
		if err != nil {
			return parsedActionsParams{}, errors.New("'offset' must be a non-negative integer")
		}
	} else {
		offset = 0
	}

	nextParams := map[string]string{"NextPageToken": p.NextPageToken, "Timestamp": p.Timestamp, "Height": p.Height}
	err := isMutuallyExclusive(nextParams)
	if err != nil {
		return parsedActionsParams{}, err
	}

	prevParams := map[string]string{"PrevPageToken": p.PrevPageToken, "FromTimestamp": p.FromTimestamp, "FromHeight": p.FromHeight}
	err = isMutuallyExclusive(prevParams)
	if err != nil {
		return parsedActionsParams{}, err
	}

	// set default actions param to be on oldactions
	oldAction := true
	if nextPageToken != 0 || timestamp != 0 || height != 0 || fromHeight != 0 || fromTimestamp != 0 || prevPageToken != 0 {
		oldAction = false
	}

	types := make([]string, 0)
	if p.ActionType != "" {
		types = strings.Split(p.ActionType, ",")
	}

	// check if it's in valid types
	validActions := map[string]bool{
		"swap":             true,
		"addLiquidity":     true,
		"withdraw":         true,
		"donate":           true,
		"refund":           true,
		"switch":           true,
		"send":             true,
		"thorname":         true,
		"runePoolDeposit":  true,
		"runePoolWithdraw": true,
		"bond":             true,
		"unbond":           true,
		"trade":            true,
		"secure":           true,
		"contract":         true,
		"tcy_claim":        true,
		"tcy_stake":        true,
		"tcy_unstake":      true,
		"limit_swap":       true,
		"rebond":           true,
	}
	for _, a := range types {
		if !validActions[a] {
			return parsedActionsParams{}, btcqerr.BadRequestF(
				"Your request for actions is '%s' and '%s' action type is unknown. Please see the docs",
				p.ActionType, a)
		}
	}

	var addresses []string
	if p.Address != "" {
		addresses = strings.Split(p.Address, ",")
		if MaxAddresses < len(addresses) {
			return parsedActionsParams{}, btcqerr.BadRequestF(
				"too many addresses: %d provided, maximum is %d",
				len(addresses), MaxAddresses)
		}
	}

	var assets []string
	if p.Asset != "" {
		assets = strings.Split(p.Asset, ",")
		if MaxAssets < len(assets) {
			return parsedActionsParams{}, btcqerr.BadRequestF(
				"too many assets: %d provided, maximum is %d",
				len(assets), MaxAssets)
		}
	}

	var txTypes []string
	if p.TxType != "" {
		txTypes = strings.Split(p.TxType, ",")
		if MaxTxTypes < len(txTypes) {
			return parsedActionsParams{}, btcqerr.BadRequestF(
				"too many tx types: %d provided, maximum is %d",
				len(assets), MaxTxTypes)
		}
	}

	var affiliateAddresses []string
	if p.Affiliate != "" {
		affiliateAddresses = strings.Split(p.Affiliate, ",")
		if DefaultLimit < len(affiliateAddresses) {
			return parsedActionsParams{}, btcqerr.BadRequestF(
				"too many affiliate addresses: %d provided, maximum is %d",
				len(assets), DefaultLimit)
		}
	}

	return parsedActionsParams{
		limit:              limit,
		nextPageToken:      nextPageToken,
		prevPageToken:      prevPageToken,
		timestamp:          timestamp,
		height:             height,
		fromTimestamp:      fromTimestamp,
		fromHeight:         fromHeight,
		offset:             offset,
		types:              types,
		addresses:          addresses,
		txid:               p.TXId,
		assets:             assets,
		txTypes:            txTypes,
		affiliateAddresses: affiliateAddresses,
		oldAction:          oldAction,
	}, nil
}

func runActionsQuery(ctx context.Context, q preparedSqlStatement) ([]action, error) {
	rows, err := db.Query(ctx, q.Query, q.Values...)
	if err != nil {
		return nil, fmt.Errorf("actions query: %w", err)
	}
	defer rows.Close()

	actions := []action{}

	for rows.Next() {
		var result action
		var ins transactionList
		var outs transactionList
		var fees coinList
		var meta actionMeta
		var streamingMeta streamingMetaType
		err := rows.Scan(
			&result.eventId,
			&result.date,
			&result.actionType,
			pq.Array(&result.pools),
			&ins,
			&outs,
			&fees,
			&meta,
			&streamingMeta,
		)
		if err != nil {
			return nil, fmt.Errorf("actions read: %w", err)
		}

		result.height = db.HeightFromEventId(result.eventId)
		result.in = ins
		result.out = outs
		result.completeFromDBRead(&meta, fees, streamingMeta)

		actions = append(actions, result)
	}

	return actions, nil
}

func (a *action) completeFromDBRead(meta *actionMeta, fees coinList, streamingMeta streamingMetaType) {
	if a.pools == nil {
		a.pools = []string{}
	}

	// process status
	a.status = "success"
	if meta.Status != "" {
		a.status = meta.Status
	}

	switch a.actionType {
	case "swap":
		// There might be multiple outs. Maybe we could check if the full sum was sent out.
		// toe8 of last swap (1st or 2nd) <= sum(outTxs.coin.amount) + networkfee.amount
		// We would need to query toe8 in txInSelectQueries.
		if len(a.out) == 0 {
			a.status = "pending"
			break
		}

		min, ind := int64(math.MaxInt64), 0
		hasOut := false
		for i, o := range a.out {
			// trade asset as out is an internal swap with same txid
			if o.Internal == nil || o.TxID != "" {
				hasOut = true
			}

			// Double swaps with QBTC output is affiliate
			isQbtc := record.IsQbtc([]byte(o.Coins[0].Asset))
			if isQbtc && o.Address != a.in[0].Address && len(a.pools) > 1 {
				a.out[i].Affiliate = util.WrapBoolean(true)
				// Show pending while only affiliate is available
				if len(a.out) == 1 {
					hasOut = false
				}
			}

			// Find the least amount of QBTC output
			if o.Coins[0].Amount < min && meta.AffiliateFee > 0 && isQbtc {
				min = o.Coins[0].Amount
				ind = i
			}
		}

		// Single swap affiliate detection BTC -> QBTC
		outAsset := []byte(a.out[ind].Coins[0].Asset)
		if len(a.pools) == 1 && len(a.out) > 1 && record.IsQbtc(outAsset) {
			a.out[ind].Affiliate = util.WrapBoolean(true)
		}

		// if streaming then pending
		if streamingMeta.Count < streamingMeta.Quantity {
			a.status = "pending"
		}

		if !hasOut {
			a.status = "pending"
		}
	case "refund":
		// success: either fee is greater than in amount or both
		// outbound and fees are present.
		// TODO(elfedy): Sometimes fee + outbound not equals in amount
		// The resons behind this must be investigated
		inBalances := make(map[string]int64)
		outBalances := make(map[string]int64)
		outFees := make(map[string]int64)

		for _, tx := range a.in {
			for _, coin := range tx.Coins {
				inBalances[coin.Asset] = coin.Amount
			}
		}
		for _, tx := range a.out {
			for _, coin := range tx.Coins {
				outBalances[coin.Asset] = coin.Amount
			}
		}
		for _, coin := range fees {
			outFees[coin.Asset] = coin.Amount
		}

		a.status = "success"
		for k, inBalance := range inBalances {
			if inBalance > outFees[k] && outBalances[k] == 0 {
				a.status = "pending"
				break
			}
		}
	case "withdraw":
		var qbtcOut, assetOut, qbtcFee, assetFee int64
		for _, tx := range a.out {
			for _, coin := range tx.Coins {
				if coin.Asset != "QBTC.QBTC" {
					assetOut = coin.Amount
				} else {
					qbtcOut = coin.Amount
				}
			}
		}
		for _, coin := range fees {
			if coin.Asset != "QBTC.QBTC" {
				assetFee = coin.Amount
			} else {
				qbtcFee = coin.Amount
			}
		}
		qbtcOk := meta.EmitQbtcE8 <= qbtcFee || qbtcOut != 0
		assetOk := meta.EmitAssetE8 <= assetFee || assetOut != 0

		a.status = "pending"
		if qbtcOk && assetOk {
			a.status = "success"
		}
	case "send":
		if meta.Code != 0 {
			a.status = "failed"
			break
		}
		a.status = "success"
	case "runePoolDeposit", "failed":
		a.status = "success"
	default:
	}

	switch a.actionType {
	case "swap":
		a.metadata.Swap = &oapigen.SwapMetadata{
			LiquidityFee:      util.IntStr(meta.LiquidityFee),
			SwapSlip:          util.IntStr(meta.SwapSlip),
			SwapTarget:        util.IntStr(meta.SwapTarget),
			NetworkFees:       fees.toOapigen(),
			AffiliateFee:      util.IntStr(meta.AffiliateFee),
			AffiliateAddress:  meta.AffiliateAddress,
			Memo:              meta.Memo,
			IsStreamingSwap:   meta.SwapStreaming,
			StreamingSwapMeta: streamingMeta.toOapigen(),
			TxType:            meta.TxType,
			InPriceUSD:        util.FloatStr(meta.InPriceUSD),
			OutPriceUSD:       util.FloatStr(meta.OutPriceUSD),
		}
	case "addLiquidity":
		if meta.LiquidityUnits != 0 {
			a.metadata.AddLiquidity = &oapigen.AddLiquidityMetadata{
				LiquidityUnits:   util.IntStr(meta.LiquidityUnits),
				Memo:             meta.Memo,
				AffiliateAddress: meta.AffiliateAddress,
				AffiliateFee:     util.IntStr(meta.AffiliateFee),
			}
		}
	case "withdraw":
		a.metadata.Withdraw = &oapigen.WithdrawMetadata{
			LiquidityUnits:            util.IntStr(meta.LiquidityUnits),
			Asymmetry:                 floatStr(meta.Asymmetry),
			BasisPoints:               util.IntStr(meta.BasisPoints),
			NetworkFees:               fees.toOapigen(),
			ImpermanentLossProtection: util.IntStr(meta.ImpLossProt),
			Memo:                      meta.Memo,
		}
	case "refund":
		a.metadata.Refund = &oapigen.RefundMetadata{
			NetworkFees:      fees.toOapigen(),
			Reason:           meta.Reason,
			Memo:             meta.Memo,
			AffiliateFee:     util.IntStr(meta.AffiliateFee),
			AffiliateAddress: meta.AffiliateAddress,
			TxType:           meta.TxType,
		}
	case "send":
		a.metadata.Send = &oapigen.SendMetadata{
			NetworkFees: fees.toOapigen(),
			Memo:        meta.Memo,
			Code:        util.IntStr(meta.Code),
			Reason:      meta.Log,
		}
	case "thorname":
		a.metadata.Thorname = &oapigen.ThornameMetadata{
			Memo:            meta.Memo,
			Thorname:        meta.Thorname,
			Address:         meta.ThornameAddress,
			Owner:           meta.ThornameOwner,
			Expire:          util.IntStr(meta.Expire),
			Chain:           meta.Chain,
			FundAmount:      util.IntStr(meta.FundAmountE8),
			RegistrationFee: util.IntStr(meta.RegistrationFeeE8),
			TxType:          meta.TxType,
		}
	case "qbtcPoolDeposit":
		a.metadata.QbtcPoolDeposit = &oapigen.QbtcPoolDepositMetadata{
			Units: util.IntStr(meta.Units),
		}
	case "qbtcPoolWithdraw":
		a.metadata.QbtcPoolWithdraw = &oapigen.QbtcPoolWithdrawMetadata{
			Units:               util.IntStr(meta.Units),
			AffiliateAddress:    meta.AffiliateAddress,
			AffiliateAmount:     util.IntStr(meta.AffiliateAmount),
			AffiliateBasisPoint: util.IntStr(meta.AffiliateBasisPoint),
			BasisPoints:         util.IntStr(meta.BasisPoints),
		}
	case "bond":
		a.metadata.Bond = &oapigen.BondMetadata{
			Memo:        meta.Memo,
			NodeAddress: meta.NodeAddress,
			Provider:    util.WrapString(meta.BondProvider),
			Fee:         util.WrapString(meta.OperatorFee),
		}
	case "unbond":
		a.metadata.Bond = &oapigen.BondMetadata{
			Memo:        meta.Memo,
			NodeAddress: meta.NodeAddress,
		}
	case "failed":
		a.metadata.Failed = &oapigen.FailedMetadata{
			Memo:   meta.Memo,
			Reason: meta.Reason,
			Code:   util.IntStr(meta.Code),
		}
	case "contract":
		a.metadata.Contract = &oapigen.ContractMetadata{
			ContractType: meta.ContractType,
			Attributes:   meta.ContractAttributes,
			Funds:        util.WrapString(meta.ContractFunds),
			Msg:          util.WrapMapInterface(meta.ContractMsg),
		}
	case "tcy_claim", "tcy_unstake", "tcy_stake":
		a.metadata.Tcy = &oapigen.TcyMetadata{
			Memo: meta.Memo,
		}
	case "limit_swap":
		a.metadata.LimitSwap = &oapigen.LimitSwapMetadata{
			Memo: meta.Memo,
		}
	case "rebond":
		a.metadata.Rebond = &oapigen.RebondMetadata{
			Memo:           meta.Memo,
			NewBondAddress: meta.NewBondAddress,
			NodeAddress:    meta.NodeAddress,
		}
	}
}

// Gets a list of actions generated by external transactions and return its associated data
func GetActions(ctx context.Context, moment time.Time, params ActionsParams) (
	oapigen.ActionsResponse, error) {
	// CHECK PARAMS
	// give latest value if zero moment
	_, timestamp, _ := LastBlock()
	if moment.IsZero() {
		moment = timestamp
	} else if timestamp.Before(moment) {
		return oapigen.ActionsResponse{}, errBeyondLast
	}

	parsedParams, err := params.parse()
	if err != nil {
		return oapigen.ActionsResponse{}, err
	}

	// Construct queries
	countPS, resultsPS, err := actionsPreparedStatements(moment, parsedParams)
	if err != nil {
		return oapigen.ActionsResponse{}, err
	}

	var totalCount int = -1
	if parsedParams.oldAction {
		// The actions endpoint is slow, often because the count query.
		// Until the new API is implemented we cancel the query if it's slow and return count=-1
		// https://discord.com/channels/838986635756044328/999334588583252078)
		countDeadlineCtx, _ := context.WithDeadline(ctx,
			time.Now().Add(config.Global.TmpActionsCountTimeout.Value()))
		countRows, err := db.Query(countDeadlineCtx, countPS.Query, countPS.Values...)
		if err != nil {
			if err, ok := err.(net.Error); ok && err.Timeout() {
				totalCount = -1
			} else {
				return oapigen.ActionsResponse{}, fmt.Errorf("actions count query: %w", err)
			}
		} else {
			defer countRows.Close()
			countRows.Next()
			err = countRows.Scan(&totalCount)
			if err != nil {
				return oapigen.ActionsResponse{}, fmt.Errorf("actions count read: %w", err)
			}
		}
	}

	// Get results
	actions, err := runActionsQuery(ctx, resultsPS)
	if err != nil {
		return oapigen.ActionsResponse{}, err
	}

	oapigenActions := make([]oapigen.Action, len(actions))
	metaData := oapigen.ActionMeta{}
	for i, action := range actions {
		if parsedParams.IsReverseLookup() {
			i = len(actions) - 1 - i
		}

		oapigenActions[i] = action.toOapigen()
		if i == len(actions)-1 {
			metaData.NextPageToken = util.IntStr(action.eventId)
		}
		if i == 0 {
			metaData.PrevPageToken = util.IntStr(action.eventId)
		}
	}

	totalCountStr := util.IntStr(int64(totalCount))
	totalCountPtr := &totalCountStr
	if !parsedParams.oldAction {
		totalCountPtr = nil
	}

	return oapigen.ActionsResponse{
		Count:   totalCountPtr,
		Actions: oapigenActions,
		Meta:    metaData}, nil
}

// Helper structs to build needed queries
// Query key is used in the query to then be replaced when parsed
// This way arguments can be dynamically inserted in query strings
type namedSqlValue struct {
	QueryKey string
	Value    interface{}
}

type preparedSqlStatement struct {
	Query  string
	Values []interface{}
}

// Builds SQL statements for Actions lookup. Two queries are needed, one to get the count
// of the total entries for the query, and one to get the subset that will actually be
// returned to the caller.
func actionsPreparedStatements(moment time.Time,
	params parsedActionsParams,
) (preparedSqlStatement, preparedSqlStatement, error) {
	var countPS, resultsPS preparedSqlStatement
	// Initialize query param slices (to dynamically insert query params)
	baseValues := make([]namedSqlValue, 0)
	subsetValues := make([]namedSqlValue, 0)

	subsetValues = append(subsetValues, namedSqlValue{"#LIMIT#", params.limit}, namedSqlValue{"#OFFSET#", params.offset})

	forceMainQuerySeparateEvaluation := false

	// build WHERE which is common to both queries, based on filter arguments
	// (types, txid, address, asset)
	// Due to the The Timescales' query planner mistake, we force the time query to be seprate when
	// txid is asked from Midgard.
	var actionFilters []string
	var timeFilters []string

	if len(params.types) != 0 {
		baseValues = append(baseValues, namedSqlValue{"#TYPE#", params.types})
		actionFilters = append(actionFilters, `action_type = ANY(#TYPE#)`)
	}

	if params.txid != "" {
		forceMainQuerySeparateEvaluation = true
		baseValues = append(baseValues, namedSqlValue{"#TXID#", strings.ToUpper(params.txid)})
		actionFilters = append(actionFilters, `transactions @> ARRAY[#TXID#]`)
	}

	if len(params.addresses) != 0 {
		baseValues = append(baseValues, namedSqlValue{"#ADDRESSES#", params.addresses})
		actionFilters = append(actionFilters, `addresses && #ADDRESSES#`)
	}

	if len(params.assets) != 0 {
		var assets []string
		for _, asset := range params.assets {
			switch asset {
			case "secure":
				actionFilters = append(actionFilters, `assets::TEXT ~ '[{,]\w+-\w+'`)
			case "trade":
				actionFilters = append(actionFilters, `assets::TEXT LIKE '%~%'`)
			case "notrade":
				actionFilters = append(actionFilters, `assets::TEXT NOT LIKE '%~%'`)
			case "synth":
				actionFilters = append(actionFilters, `assets::TEXT LIKE '%/%'`)
			case "nosynth":
				actionFilters = append(actionFilters, `assets::TEXT NOT LIKE '%/%'`)
			case "single":
				actionFilters = append(actionFilters, `array_length(pools, 1) = 1`)
			case "double":
				actionFilters = append(actionFilters, `array_length(pools, 1) = 2`)
			case "norune":
				actionFilters = append(actionFilters, `assets NOT @> 'QBTC.QBTC`)
			default:
				assets = append(assets, asset)
			}
		}

		if len(assets) > 0 {
			baseValues = append(baseValues, namedSqlValue{"#ASSET#", pq.Array(assets)})
			actionFilters = append(actionFilters, `assets && #ASSET#`)
		}
	}

	if len(params.txTypes) != 0 {
		baseValues = append(baseValues, namedSqlValue{"#LABEL#", pq.Array(params.txTypes)})
		actionFilters = append(actionFilters, `meta->'txType' ?| #LABEL#`)
	}

	if len(params.affiliateAddresses) != 0 {
		baseValues = append(baseValues, namedSqlValue{"#AFFILIATE#", pq.Array(params.affiliateAddresses)})
		actionFilters = append(actionFilters, `string_to_array(lower(meta->>'affiliateAddress'), '/') && #AFFILIATE#`)
	}

	// These pagination filters are also time sensetive
	if params.nextPageToken != 0 {
		baseValues = append(baseValues, namedSqlValue{"#NEXTPAGETOKEN#", params.nextPageToken})
		timeFilters = append(timeFilters, `event_id < #NEXTPAGETOKEN#`)
	}

	if params.timestamp != 0 {
		baseValues = append(baseValues, namedSqlValue{"#TIMESTAMP#", params.timestamp})
		timeFilters = append(timeFilters, `block_timestamp < #TIMESTAMP#`)
	}

	if params.height != 0 {
		baseValues = append(baseValues, namedSqlValue{"#HEIGHT#", db.HeightToEventId(params.height)})
		timeFilters = append(timeFilters, `event_id < #HEIGHT#`)
	}

	if params.prevPageToken != 0 {
		baseValues = append(baseValues, namedSqlValue{"#PREVPAGETOKEN#", params.prevPageToken})
		timeFilters = append(timeFilters, `event_id > #PREVPAGETOKEN#`)
	}

	if params.fromTimestamp != 0 {
		baseValues = append(baseValues, namedSqlValue{"#FROMTIMESTAMP#", params.fromTimestamp})
		timeFilters = append(timeFilters, `block_timestamp > #FROMTIMESTAMP#`)
	}

	if params.fromHeight != 0 {
		baseValues = append(baseValues, namedSqlValue{"#FROMHEIGHT#", db.HeightToEventId(params.fromHeight)})
		timeFilters = append(timeFilters, `event_id > #FROMHEIGHT#`)
	}

	if len(timeFilters) == 0 {
		timeFilters = []string{`event_id <= nano_event_id_up(#MOMENT#)`}
		baseValues = append(baseValues, namedSqlValue{"#MOMENT#", moment.UnixNano()})
	}

	// build and return final queries
	countQuery := `SELECT count(1) FROM btcq_indexer_agg.actions ` + db.Where(append(timeFilters, actionFilters...)...)

	if forceMainQuerySeparateEvaluation {
		countQuery = `WITH relevant_actions AS (SELECT * FROM btcq_indexer_agg.actions 
		 ` + db.Where(actionFilters...) + `
			OFFSET 0
		)
		SELECT COUNT(1) FROM relevant_actions ` + db.Where(timeFilters...)
	}

	btcqlog.Debug(countQuery)

	countQueryValues := make([]interface{}, 0)
	for i, queryValue := range baseValues {
		position := i + 1
		positionLabel := fmt.Sprintf("$%d", position)
		countQuery = strings.ReplaceAll(countQuery, queryValue.QueryKey, positionLabel)
		countQueryValues = append(countQueryValues, queryValue.Value)
	}
	countPS = preparedSqlStatement{countQuery, countQueryValues}

	mainQuery := `
		SELECT
			event_id,
			block_timestamp,
			action_type,
			pools,
			ins,
			outs,
			fees,
			meta,
			streaming_meta
		FROM btcq_indexer_agg.actions
	`

	// The Postgres' query planner is kinda dumb when we have a `txid` specified.
	// Because we also want to order by `event_id` and limit the number of results,
	// it chooses to do a scan on `event_id` index and filter for rows that have the given
	// txid, instead of using the `transactions` index and sorting afterwards. This is a very bad
	// decision in this case.
	// See https://github.com/btcq/btcq-indexer/-/issues/45 for details.
	//
	// The `OFFSET 0` in a sub-query is a semi-officially blessed hack to stop Postgres from
	// inlining a sub-query; thus forcing it to create an independent plan for it. In which case
	// it obviously uses the index on `transactions`.
	if forceMainQuerySeparateEvaluation {
		mainQuery = `WITH relevant_actions AS (` + mainQuery + db.Where(actionFilters...) + `
				OFFSET 0
			)
			SELECT * FROM relevant_actions
			` + db.Where(timeFilters...)
	} else {
		mainQuery += db.Where(append(timeFilters, actionFilters...)...)
	}

	orderQuery := `
		ORDER BY event_id DESC
		LIMIT #LIMIT#
		OFFSET #OFFSET#
	`

	if params.IsReverseLookup() {
		orderQuery = `
			ORDER BY event_id
			LIMIT #LIMIT#
			OFFSET #OFFSET#
		`
	}

	resultsQuery := mainQuery + orderQuery

	resultsQueryValues := make([]interface{}, 0)
	for i, queryValue := range append(baseValues, subsetValues...) {
		position := i + 1
		positionLabel := fmt.Sprintf("$%d", position)
		resultsQuery = strings.ReplaceAll(resultsQuery, queryValue.QueryKey, positionLabel)
		resultsQueryValues = append(resultsQueryValues, queryValue.Value)
	}
	resultsPS = preparedSqlStatement{resultsQuery, resultsQueryValues}

	return countPS, resultsPS, nil
}

func GetTopSwaps(ctx context.Context, period db.Buckets) (oapigen.ActionsResponse, error) {
	q := `
		WITH latest_qbtc_price AS (
		SELECT
			qbtc_price_e8
		FROM qbtc_price
		ORDER BY block_timestamp DESC
		LIMIT 1
		),
		latest_pool_depths AS (
		SELECT
			DISTINCT ON (pool) pool,
			asset_e8,
			qbtc_e8,
			block_timestamp
		FROM block_pool_depths
		ORDER BY pool, block_timestamp DESC
		)
		SELECT
			a.event_id,
			a.block_timestamp,
			a.action_type,
			a.pools,
			a.ins,
			a.outs,
			a.fees,
			a.meta,
			a.streaming_meta
		FROM btcq_indexer_agg.actions AS a
		JOIN latest_qbtc_price AS r
		ON TRUE -- Cross join with latest rune price
		JOIN latest_pool_depths AS l
		ON a.ins->0->'coins'->0->>'asset' = l.pool
		WHERE 
		$1 < a.block_timestamp
		AND a.block_timestamp < $2
		AND a.action_type = 'swap'
		ORDER BY 
		(l.qbtc_e8::DOUBLE PRECISION / l.asset_e8::DOUBLE PRECISION) * r.qbtc_price_e8 * (a.ins->0->'coins'->0->>'amount')::DOUBLE PRECISION DESC
		LIMIT 15;
	`

	statement := preparedSqlStatement{q, []interface{}{period.Start().ToNano(), period.End().ToNano()}}
	actions, err := runActionsQuery(ctx, statement)
	if err != nil {
		return oapigen.ActionsResponse{}, errors.New("can't query the top swaps")
	}

	oapigenActions := make([]oapigen.Action, len(actions))
	for i, action := range actions {
		oapigenActions[i] = action.toOapigen()
	}

	return oapigen.ActionsResponse{
		Actions: oapigenActions,
	}, nil
}

type affiliateItem struct {
	Affiliate string
	StartTime db.Second
	VolumeUSD float64
	Count     int64
}

func GetAffiliateStats(ctx context.Context, buckets db.Buckets, thorname string) (oapigen.AffiliateStatsResponse, error) {
	var actionFilters []string
	actionFilters = append(actionFilters, `action_type = 'swap'`, `$1 < a.block_timestamp`, `a.block_timestamp < $2`, `l.asset_e8 > 0 OR a.ins->0->'coins'->0->>'asset' = 'THOR.RUNE'`)
	queryValues := []interface{}{buckets.Start().ToNano(), buckets.End().ToNano()}

	thornames := strings.Split(strings.ToLower(thorname), ",")
	if len(thornames) > 0 && thornames[0] != "" {
		actionFilters = append(actionFilters, `string_to_array(lower(meta->>'affiliateAddress'), '/') && $3`)
		queryValues = append(queryValues, pq.Array(thornames))
	}

	q := `
		SELECT
			` + db.SelectTruncatedTimestamp("a.block_timestamp", buckets) + ` as date,
			SUM(
				(
					CASE 
					WHEN a.ins->0->'coins'->0->>'asset' = 'QBTC.QBTC' THEN 1
					WHEN l.asset_e8 > 0 THEN l.qbtc_e8::DOUBLE PRECISION / l.asset_e8::DOUBLE PRECISION
					ELSE 0
					END
				) * COALESCE(r.qbtc_price_e8, 0) * (a.ins->0->'coins'->0->>'amount')::DOUBLE PRECISION
			) AS total_volume,
			count(1) as count,
			COALESCE(meta->>'affiliateAddress', '') as affiliate
		FROM btcq_indexer_agg.actions AS a
		LEFT JOIN LATERAL (
			SELECT qbtc_price_e8
			FROM qbtc_price
			WHERE qbtc_price.block_timestamp <= a.block_timestamp
			ORDER BY block_timestamp DESC
			LIMIT 1
		) AS r ON TRUE
		LEFT JOIN LATERAL (
			SELECT asset_e8, rune_e8
			FROM block_pool_depths
			WHERE block_pool_depths.pool = a.pools[1]
			AND block_pool_depths.block_timestamp <= a.block_timestamp
			AND a.ins->0->'coins'->0->>'asset' <> 'QBTC.QBTC'
			ORDER BY block_timestamp DESC
			LIMIT 1
		) AS l ON TRUE
		` + db.Where(actionFilters...) + `
		GROUP BY date, affiliate
		ORDER BY date, affiliate;
	`

	rows, err := db.Query(ctx, q, queryValues...)
	if err != nil {
		return oapigen.AffiliateStatsResponse{}, fmt.Errorf("affiliate stats query: %w", err)
	}
	defer rows.Close()

	var stats []affiliateItem

	for rows.Next() {
		var date db.Second
		var volume float64
		var count int64
		var affiliate string
		err := rows.Scan(
			&date,
			&volume,
			&count,
			&affiliate,
		)
		if err != nil {
			return oapigen.AffiliateStatsResponse{}, fmt.Errorf("actions read: %w", err)
		}

		stats = append(stats, affiliateItem{
			StartTime: date,
			VolumeUSD: volume,
			Count:     count,
			Affiliate: affiliate,
		})
	}

	intervals := make([]oapigen.AffiliateStatsItem, buckets.Count())

	idx := 0
	for i := range buckets.Count() {
		current := &intervals[i]
		from, until := buckets.Bucket(i)
		current.StartTime = util.IntStr(from.ToI())
		current.EndTime = util.IntStr(until.ToI())
		current.Affiliates = []oapigen.AffiliateStatItem{}
		current.Count = "0"
		current.TotalVolumeUSD = "0"

		var totalCount int64
		var totalVolumeUSD int64

		for idx < len(stats) && stats[idx].StartTime == from {
			affiliate := &stats[idx]
			totalCount += affiliate.Count
			totalVolumeUSD += int64(affiliate.VolumeUSD / 1e6)

			// Fill Affiliate
			current.Affiliates = append(current.Affiliates, oapigen.AffiliateStatItem{
				Affiliate: affiliate.Affiliate,
				VolumeUSD: util.IntStr(int64(affiliate.VolumeUSD / 1e6)),
				Count:     util.IntStr(affiliate.Count),
			})

			idx++
		}

		// Convert totals to string format
		current.Count = util.IntStr(totalCount)
		current.TotalVolumeUSD = util.IntStr(totalVolumeUSD)
	}

	return intervals, nil
}

func GetAffiliateEarning(ctx context.Context, buckets db.Buckets, thorname string) (oapigen.AffiliateEarning, error) {
	var actionFilters []string
	actionFilters = append(actionFilters, `action_type = 'swap'`, `$1 < a.block_timestamp`, `a.block_timestamp < $2`)
	queryValues := []interface{}{buckets.Start().ToNano(), buckets.End().ToNano()}

	thornames := strings.Split(strings.ToLower(thorname), ",")
	if len(thornames) > 0 && thornames[0] != "" {
		actionFilters = append(actionFilters, `string_to_array(lower(a.meta->>'affiliateAddress'), '/') && $3`)
		queryValues = append(queryValues, pq.Array(thornames))
	}

	q := `
		WITH latest_rune_price AS (
			SELECT
				rune_price_e8
			FROM rune_price
			ORDER BY block_timestamp DESC
			LIMIT 1
		)
		SELECT
			` + db.SelectTruncatedTimestamp("a.block_timestamp", buckets) + ` as date,
			SUM((a.meta->>'liquidityFee')::BIGINT * COALESCE(r.qbtc_price_e8, 0) / 1e6)::BIGINT AS liquidity_fee_usd,
			SUM((a.meta->>'liquidityFee')::BIGINT) AS liquidity_fee_rune,
			count(1) as count,
			COALESCE(a.meta->>'affiliateAddress', '') as affiliate
		FROM btcq_indexer_agg.actions AS a
		JOIN latest_qbtc_price AS r ON TRUE
		` + db.Where(actionFilters...) + `
		GROUP BY date, affiliate
		ORDER BY date, affiliate;
	`

	rows, err := db.Query(ctx, q, queryValues...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Read all rows into a slice
	type rawBucket struct {
		Date             db.Second
		LiquidityFeeUSD  int64
		LiquidityFeeRune int64
		Count            int64
		Affiliate        string
	}

	rawBuckets := []rawBucket{}
	for rows.Next() {
		var bucket rawBucket
		err := rows.Scan(&bucket.Date, &bucket.LiquidityFeeUSD, &bucket.LiquidityFeeRune, &bucket.Count, &bucket.Affiliate)
		if err != nil {
			return nil, err
		}
		rawBuckets = append(rawBuckets, bucket)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Group by bucket and aggregate
	intervals := make([]oapigen.AffiliateEarningInterval, buckets.Count())
	idx := 0
	for i := range buckets.Count() {
		from, until := buckets.Bucket(i)
		current := &intervals[i]
		current.StartTime = util.IntStr(from.ToI())
		current.EndTime = util.IntStr(until.ToI())
		current.Affiliates = []oapigen.AffiliateEarningItem{}
		current.TotalEarningsRune = "0"
		current.TotalEarningsUSD = "0"
		current.Count = "0"

		var totalEarningsRune int64
		var totalEarningsUSD int64
		var totalCount int64

		// Collect all affiliates for this bucket
		for idx < len(rawBuckets) && rawBuckets[idx].Date == from {
			bucket := &rawBuckets[idx]
			earningsUSD := bucket.LiquidityFeeUSD
			earningsRUNE := bucket.LiquidityFeeRune

			totalEarningsRune += earningsRUNE
			totalEarningsUSD += earningsUSD
			totalCount += bucket.Count

			current.Affiliates = append(current.Affiliates, oapigen.AffiliateEarningItem{
				Affiliate:    bucket.Affiliate,
				EarningsUSD:  util.IntStr(earningsUSD),
				EarningsRUNE: util.IntStr(earningsRUNE),
				Count:        util.IntStr(bucket.Count),
			})

			idx++
		}

		current.TotalEarningsRune = util.IntStr(totalEarningsRune)
		current.TotalEarningsUSD = util.IntStr(totalEarningsUSD)
		current.Count = util.IntStr(totalCount)
	}

	return intervals, nil
}
