package db_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/internal/db"
)

// To run with fuzzing, run:
// go test -v -fuzz=Fuzz -fuzztime=30s ./internal/db/eventid_test.go
func FuzzInvolution(f *testing.F) {
	cases := []int64{2220000000003, 2221000040004, 2227000050005, 2229000000009}
	for _, c := range cases {
		f.Add(c)
	}
	f.Fuzz(func(t *testing.T, eid int64) {
		require.Equal(t, eid, db.ParseEventId(eid).AsBigint())
	})
}

func TestEventId(t *testing.T) {
	cases := map[int64]db.EventId{
		2220300000003: {222, db.BeginBlockEvents, 0, 300000003},
		2221000040004: {222, db.TxsResults, 4, 4},
		2228000050005: {222, db.TxsResults, 700005, 5},
		2229700000007: {222, db.EndBlockEvents, 999999, 700000007},
	}
	for eid, event_id := range cases {
		require.Equal(t, event_id, db.ParseEventId(eid))
		require.Equal(t, eid, event_id.AsBigint())
	}
}
