package record_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v4"
	pgxstd "github.com/jackc/pgx/v4/stdlib"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
)

func intToBytes(n int64) []byte {
	return []byte(base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(int(n)))))
}

func clearTable() {
	_, _ = db.TheDB.Exec("DELETE FROM swap_events")
}

func insertOne(t *testing.T, n int64) {
	e := record.Swap{
		Tx:             intToBytes(n),
		Chain:          []byte("chain"),
		FromAddr:       intToBytes(n),
		ToAddr:         intToBytes(n),
		FromAsset:      []byte("BNB.BNB"),
		FromE8:         n,
		ToAsset:        []byte("THOR.RUNE"),
		ToE8:           n,
		Memo:           intToBytes(n),
		Pool:           []byte("BNB.BNB"),
		ToE8Min:        n,
		SwapSlipBP:     n,
		LiqFeeE8:       n,
		LiqFeeInRuneE8: n,
	}
	height := n
	var direction db.SwapDirection = db.AssetToRune

	const q = `INSERT INTO swap_events (
		tx, chain, from_addr, to_addr, from_asset, from_E8, to_asset, to_E8, memo, pool,
		to_E8_min, swap_slip_BP, liq_fee_E8, liq_fee_in_rune_E8,
		_direction,
		event_id, block_timestamp)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`
	result, err := db.TheDB.Exec(
		q, e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.FromAsset, e.FromE8, e.ToAsset, e.ToE8, e.Memo,
		e.Pool, e.ToE8Min, e.SwapSlipBP, e.LiqFeeE8, e.LiqFeeInRuneE8, direction, height, height)
	if err != nil {
		t.Error("failed to insert:", err)
		return
	}
	k, err := result.RowsAffected()
	if err != nil {
		t.Error("failed to insert2: ", err)
		return
	}
	if k != 1 {
		t.Error("not one insert:", k)
	}
}

func valueStringIterator(argNum int) func() string {
	argCount := 0
	// return a string like ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) but from argcount
	return func() string {
		bb := bytes.Buffer{}
		bb.WriteString(" (")
		for k := 1; k <= argNum; k++ {
			argCount++

			if k != 1 {
				bb.WriteRune(',')
			}
			bb.WriteRune('$')
			bb.WriteString(strconv.Itoa(argCount))
		}
		bb.WriteString(")")
		return bb.String()
	}
}

func insertBatch(t *testing.T, from, to int64) {
	length := int(to - from)
	argNum := 17
	valueStrs := make([]string, 0, length)
	valueArgs := make([]interface{}, 0, argNum*length)
	insertIt := valueStringIterator(argNum)
	for n := from; n < to; n++ {
		e := record.Swap{
			Tx:             intToBytes(n),
			Chain:          []byte("chain"),
			FromAddr:       intToBytes(n),
			ToAddr:         intToBytes(n),
			FromAsset:      []byte("BNB.BNB"),
			FromE8:         n,
			ToAsset:        []byte("THOR.RUNE"),
			ToE8:           n,
			Memo:           intToBytes(n),
			Pool:           []byte("BNB.BNB"),
			ToE8Min:        n,
			SwapSlipBP:     n,
			LiqFeeE8:       n,
			LiqFeeInRuneE8: n,
		}
		var direction db.SwapDirection = db.AssetToRune
		height := n
		valueStrs = append(valueStrs, insertIt())
		valueArgs = append(valueArgs, e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.FromAsset, e.FromE8,
			e.ToAsset, e.ToE8, e.Memo, e.Pool, e.ToE8Min, e.SwapSlipBP,
			e.LiqFeeE8, e.LiqFeeInRuneE8, direction, height, height)
	}
	q := fmt.Sprintf(
		`INSERT INTO swap_events (
			tx, chain, from_addr, to_addr, from_asset, from_E8, to_asset, to_E8, memo, pool,
			to_E8_min, swap_slip_BP, liq_fee_E8, liq_fee_in_rune_E8, _direction,
			event_id, block_timestamp)
	VALUES %s`, strings.Join(valueStrs, ","))

	result, err := db.TheDB.Exec(q, valueArgs...)
	if err != nil {
		t.Error("failed to insert:", err)
		return
	}
	k, err := result.RowsAffected()
	if err != nil {
		t.Error("failed to insert2: ", err)
		return
	}
	if int(k) != length {
		t.Error("not one insert:", k)
	}
}

func copyFromBatch(t *testing.T, from, to int64) {
	length := int(to - from)
	rows := make([][]interface{}, 0, length)
	for n := from; n < to; n++ {
		e := record.Swap{
			Tx:             intToBytes(n),
			Chain:          []byte("chain"),
			FromAddr:       intToBytes(n),
			ToAddr:         intToBytes(n),
			FromAsset:      []byte("BNB.BNB"),
			FromE8:         n,
			ToAsset:        []byte("THOR.RUNE"),
			ToE8:           n,
			Memo:           intToBytes(n),
			Pool:           []byte("BNB.BNB"),
			ToE8Min:        n,
			SwapSlipBP:     n,
			LiqFeeE8:       n,
			LiqFeeInRuneE8: n,
		}
		var direction db.SwapDirection = db.AssetToRune
		height := n
		rows = append(rows, []interface{}{e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.FromAsset,
			e.FromE8, e.ToAsset, e.ToE8, e.Memo, e.Pool, e.ToE8Min, e.SwapSlipBP, e.LiqFeeE8,
			e.LiqFeeInRuneE8, direction, height, height})
	}

	conn, err := db.TheDB.Conn(context.Background())
	if err != nil {
		t.Error("failed to get a connection: ", err)
		return
	}

	err = conn.Raw(func(rawConn interface{}) (err error) {
		pxgConn := rawConn.(*pgxstd.Conn).Conn()
		k, err := pxgConn.CopyFrom(context.Background(), pgx.Identifier{"swap_events"},
			[]string{"tx", "chain", "from_addr", "to_addr", "from_asset", "from_e8", "to_asset",
				"to_e8", "memo", "pool", "to_e8_min", "swap_slip_bp", "liq_fee_e8",
				"liq_fee_in_rune_e8", "_direction", "event_id", "block_timestamp"},
			pgx.CopyFromRows(rows))
		if err != nil {
			t.Error("CopyFrom failed: ", err)
			return
		}

		if int(k) != length {
			t.Error("Wrong number of rows inserted: ", k)
		}

		return
	})
	if err != nil {
		t.Error("failed to execute a raw pgx operation: ", err)
	}
}

func batchInserterBatch(t *testing.T, from, to int64) {
	err := db.Inserter.StartBlock()
	if err != nil {
		t.Error("Failed to StartBlock: ", err)
		return
	}

	for n := from; n < to; n++ {
		e := record.Swap{
			Tx:             intToBytes(n),
			Chain:          []byte("chain"),
			FromAddr:       intToBytes(n),
			ToAddr:         intToBytes(n),
			FromAsset:      []byte("BNB.BNB"),
			FromE8:         n,
			ToAsset:        []byte("THOR.RUNE"),
			ToE8:           n,
			Memo:           intToBytes(n),
			Pool:           []byte("BNB.BNB"),
			ToE8Min:        n,
			SwapSlipBP:     n,
			LiqFeeE8:       n,
			LiqFeeInRuneE8: n,
		}
		height := n
		var direction db.SwapDirection = db.AssetToRune
		cols := []string{"tx", "chain", "from_addr", "to_addr", "from_asset", "from_e8", "to_asset",
			"to_e8", "memo", "pool", "to_e8_min", "swap_slip_bp", "liq_fee_e8",
			"liq_fee_in_rune_e8", "_direction", "event_id", "block_timestamp"}
		err = db.Inserter.Insert("swap_events", cols,
			e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.FromAsset, e.FromE8, e.ToAsset, e.ToE8, e.Memo,
			e.Pool, e.ToE8Min, e.SwapSlipBP, e.LiqFeeE8, e.LiqFeeInRuneE8, direction,
			height, height)
		if err != nil {
			t.Error("Failed to insert: ", err)
			return
		}
	}

	err = db.Inserter.EndBlock()
	if err != nil {
		t.Error("Failed to EndBlock: ", err)
		return
	}

	err = db.Inserter.Flush()
	if err != nil {
		t.Error("Failed to EndBlock: ", err)
		return
	}
}

func TestInsertOne(t *testing.T) {
	testdb.SetupTestDB(t)
	clearTable()
	insertOne(t, 0)
}

func TestInsertBatch(t *testing.T) {
	testdb.SetupTestDB(t)
	clearTable()
	insertBatch(t, 0, 3000)
}

func TestInsertCopyFrom(t *testing.T) {
	testdb.SetupTestDB(t)
	clearTable()
	copyFromBatch(t, 0, 3000)
}

func TestInsertBatchInserter(t *testing.T) {
	testdb.SetupTestDB(t)
	clearTable()
	batchInserterBatch(t, 0, 3000)
}

func BenchmarkInsertOne(b *testing.B) {
	testdb.SetupTestDB(nil)
	clearTable()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		insertOne(nil, int64(i))
	}
}

// Max batch size we can use is ~3800 because there is a 64k limit on the
// sql argumentum size and we have 17 args per insert.
//
// The improvement is 73x:
//
// $ go test -run=NONE -bench Insert -v -p 1 ./internal/fetch/record...
// goos: linux
// goarch: amd64
// pkg: gitlab.com/thorchain/midgard/internal/timeseries
// BenchmarkInsertOne
// BenchmarkInsertOne-8                 682           1634591 ns/op
// BenchmarkInsertBatch
// BenchmarkInsertBatch-8             58502             22192 ns/op
// PASS
//
// Updated with CopyFrom and BatchInserter, the results look as follows:
// goos: linux
// goarch: amd64
// pkg: gitlab.com/thorchain/midgard/internal/fetch/record
// cpu: Intel(R) Core(TM) i7-3770 CPU @ 3.40GHz
// BenchmarkInsertOne
// BenchmarkInsertOne-8                         607           2076099 ns/op
// BenchmarkInsertBatch
// BenchmarkInsertBatch-8                     32160             34306 ns/op
// BenchmarkInsertCopyFrom
// BenchmarkInsertCopyFrom-8                  89013             15208 ns/op
// BenchmarkInsertBatchInserter
// BenchmarkInsertBatchInserter-8             99566             13642 ns/op
func BenchmarkInsertBatch(b *testing.B) {
	testdb.SetupTestDB(nil)
	clearTable()
	b.ResetTimer()
	batchSize := 3000
	for i := 0; i < b.N; i += batchSize {
		to := i + batchSize
		if b.N < to {
			to = b.N
		}
		insertBatch(nil, int64(i), int64(to))
	}
}

func BenchmarkInsertCopyFrom(b *testing.B) {
	testdb.SetupTestDB(nil)
	clearTable()
	b.ResetTimer()
	batchSize := 3000
	for i := 0; i < b.N; i += batchSize {
		to := i + batchSize
		if b.N < to {
			to = b.N
		}
		copyFromBatch(nil, int64(i), int64(to))
	}
}

func BenchmarkInsertBatchInserter(b *testing.B) {
	testdb.SetupTestDB(nil)
	clearTable()
	b.ResetTimer()
	batchSize := 3000
	for i := 0; i < b.N; i += batchSize {
		to := i + batchSize
		if b.N < to {
			to = b.N
		}
		batchInserterBatch(nil, int64(i), int64(to))
	}
}
