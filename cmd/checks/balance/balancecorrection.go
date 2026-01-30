package main

import (
	"fmt"

	"gitlab.com/thorchain/midgard/internal/fetch/record"
)

type BalanceCorrection struct {
	addr            string
	asset           string
	thorAmountE8    int64
	midgardAmountE8 int64
}

func (b BalanceCorrection) fromAddr() string {
	if b.thorAmountE8 < b.midgardAmountE8 {
		return b.addr
	}
	return record.MidgardBalanceCorrectionAddress
}

func (b BalanceCorrection) toAddr() string {
	if b.thorAmountE8 < b.midgardAmountE8 {
		return record.MidgardBalanceCorrectionAddress
	}
	return b.addr
}

func (b BalanceCorrection) absAmountDiffE8() int64 {
	diff := b.thorAmountE8 - b.midgardAmountE8
	if diff < 0 {
		return -diff
	}
	return diff
}

func (b BalanceCorrection) sprint() string {
	return fmt.Sprintf(
		`{"asset": "%v", "fromAddr": "%v", "toAddr": "%v", "amountE8": %v}`,
		b.asset, b.fromAddr(), b.toAddr(), b.absAmountDiffE8())
}

// Mutates the second parameter
func getCorrections(thorBalances map[string]Balance, midgardBalances map[string]Balance) []BalanceCorrection {
	result := []BalanceCorrection{}
	for key, thorBalance := range thorBalances {
		midgardBalance, ok := midgardBalances[key]
		if !ok {
			bc := BalanceCorrection{
				addr:            thorBalance.addr,
				asset:           thorBalance.asset,
				thorAmountE8:    thorBalance.amountE8,
				midgardAmountE8: 0}
			result = append(result, bc)
		} else {
			delete(midgardBalances, thorBalance.key())
			if thorBalance.amountE8 != midgardBalance.amountE8 {
				bc := BalanceCorrection{
					addr:            thorBalance.addr,
					asset:           thorBalance.asset,
					thorAmountE8:    thorBalance.amountE8,
					midgardAmountE8: midgardBalance.amountE8}
				result = append(result, bc)
			}
		}
	}
	for _, b := range midgardBalances {
		if b.amountE8 != 0 && b.addr != record.MidgardBalanceCorrectionAddress {
			bc := BalanceCorrection{
				addr:            b.addr,
				asset:           b.asset,
				thorAmountE8:    0,
				midgardAmountE8: b.amountE8,
			}
			result = append(result, bc)
		}
	}
	return result
}
