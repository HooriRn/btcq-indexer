package record_test

import (
	"testing"

	"github.com/btcq/btcq-indexer/internal/db/testdb"
	"github.com/btcq/btcq-indexer/internal/fetch/record"
	abci "github.com/cometbft/cometbft/abci/types"
)

type FakeDemux struct {
	reuse struct {
		record.Rewards
	}
}

var GlobalFakeDemux FakeDemux

func (d *FakeDemux) processDemux(event abci.Event) int64 {
	attrs := event.Attributes

	switch event.Type {
	case "rewards":
		if err := d.reuse.Rewards.LoadTendermint(attrs); err != nil {
			panic(err)
		}
		return int64(len(d.reuse.Rewards.PerPool))
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
	case "rewards":
		var x record.Rewards
		if err := x.LoadTendermint(attrs); err != nil {
			panic(err)
		}
		return int64(len(x.PerPool))
	default:
		panic("unknown event type")
	}
}

var total int64

var events = []abci.Event{
	testdb.Rewards{
		BondE8: 100,
		PerPool: []testdb.Amount{
			{Asset: "THOR.RUNE", E8: 50},
		},
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
