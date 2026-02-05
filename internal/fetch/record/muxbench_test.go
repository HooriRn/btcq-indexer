package record_test

import (
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/btcq/btcq-indexer/internal/db/testdb"
	"github.com/btcq/btcq-indexer/internal/fetch/record"
)

type FakeDemux struct {
	reuse struct {
		record.Bond
		record.Errata
		record.Fee
		record.Gas
		record.Pool
		record.Reserve
		record.Rewards
		record.SetIPAddress
		record.SetNodeKeys
		record.SetVersion
		record.Slash
		record.Stake
		record.Transfer
		record.Withdraw
		record.ValidatorRequestLeave
		record.PoolBalanceChange
		record.THORNameChange
		record.SlashPoints
	}
}

var GlobalFakeDemux FakeDemux

func (d *FakeDemux) processDemux(event abci.Event) int64 {
	attrs := event.Attributes

	switch event.Type {
	case "transfer":
		if err := d.reuse.Transfer.LoadTendermint(attrs); err != nil {
			panic(err)
		}
		return d.reuse.Transfer.AmountE8
	default:
		panic("unknown event type")
	}
}

// Note: this presents a worse picture than it should, because without the
// Demux.reuse the `LoadTendermint` functions would not need to clear the structures they are
// filling in.
func processDirect(event abci.Event) int64 {
	attrs := event.Attributes

	switch event.Type {
	case "transfer":
		var x record.Transfer
		if err := x.LoadTendermint(attrs); err != nil {
			panic(err)
		}
		return x.AmountE8
	default:
		panic("unknown event type")
	}
}

var total int64

var events = []abci.Event{
	testdb.Transfer{
		FromAddr:    "thorAddr2",
		ToAddr:      "thorAddr1",
		AssetAmount: "1 THOR.RUNE",
	}.ToTendermint(),
}

func BenchmarkLoadDemux(b *testing.B) {
	d := &GlobalFakeDemux

	for i := 0; i < b.N; i++ {
		for _, event := range events {
			total += d.processDemux(event)
		}
	}
}

func BenchmarkLoadDirect(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, event := range events {
			total += processDirect(event)
		}
	}
}
