package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/btcq/btcq-indexer/internal/util/btcqerr"
	"github.com/btcq/btcq-indexer/internal/util/btcqlog"
)

func ReadChainID(ctx context.Context) (string, error) {
	var chainId string
	err := TheDB.QueryRow("SELECT value FROM constants WHERE key = $1", chainIdKey).Scan(&chainId)
	if err != nil && err != sql.ErrNoRows {
		btcqlog.FatalE(err, "Failed to read 'chain_id' from constants")
	}

	return chainId, err
}

// GetMigrateUpdates returns SQL to run for schema migrations.
// There are no current migrations for this chain app; add blocks here when needed.
func GetMigrateUpdates(currentDdlHash md5Hash, tag string) (data []byte, err error) {
	return nil, nil
}

// TrimDB deletes all blocks including and after certain height.
// NOTE: You should trim to blocks height % 100 == 0 for now.

func TrimDB(ctx context.Context, heightOrTimestamp int64) {

	height, timestamp, err := QueryTimestampAndHeight(ctx, heightOrTimestamp)
	if err != nil {
		btcqlog.FatalF("Couldn't find height for %d", heightOrTimestamp)
	}

	// Actions & Rune Price Aggregates
	btcqlog.Info("Deleting actions")
	DeleteAfter("btcq_indexer_agg.actions", "block_timestamp", timestamp.ToI())
	DeleteAfter("btcq_indexer_agg.rune_price", "block_timestamp", timestamp.ToI())
	btcqlog.Info("Deleting watermark")
	DeleteWatermark(timestamp.ToI())

	btcqlog.InfoF("Deleting rows including and after height %d , timestamp %d", height, timestamp)
	tables := GetTableColumns(ctx)
	for table, columns := range tables {
		if columns["block_timestamp"] {
			btcqlog.InfoF("%s  deleting by block_timestamp", table)
			DeleteAfter(table, "block_timestamp", timestamp.ToI())
		} else if columns["height"] {
			btcqlog.InfoF("%s deleting by height", table)
			DeleteAfter(table, "height", height)
		} else if table == "constants" {
			btcqlog.InfoF("Skipping table %s", table)
		} else {
			btcqlog.WarnF("talbe %s has no good column", table)
		}
	}
}

func GetLatestHeight(ctx context.Context) (int64, error) {
	q := `
		SELECT height
		FROM block_log
		ORDER BY height DESC
		LIMIT 1
	`
	rows, err := Query(ctx, q)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if !rows.Next() {
		return 0, btcqerr.BadRequestF("No blocks in block_log")
	}

	var height int64
	err = rows.Scan(&height)
	return height, err
}

func QueryTimestampAndHeight(ctx context.Context, id int64) (
	height int64, timestamp Nano, err error) {
	q := `
		SELECT height, timestamp
		FROM block_log
		WHERE height=$1 OR timestamp<=$1
		ORDER BY TIMESTAMP DESC
		LIMIT 1
	`
	rows, err := Query(ctx, q, id)
	if err != nil {
		return
	}
	defer rows.Close()

	if !rows.Next() {
		err = btcqerr.BadRequestF("No such height or timestamp: %d", id)
		return
	}
	err = rows.Scan(&height, &timestamp)
	return
}

func DeleteWatermark(value int64) {
	q := `
	UPDATE 
		btcq_indexer_agg.watermarks 
	SET watermark = $1 
	WHERE materialized_table = 'actions'
	`
	_, err := TheDB.Exec(q, value)
	if err != nil {
		btcqlog.FatalE(err, "update failed")
	}
}

func DeleteAfter(table string, columnName string, value int64) {
	q := fmt.Sprintf("DELETE FROM %s WHERE $1 <= %s", table, columnName)
	_, err := TheDB.Exec(q, value)
	if err != nil {
		btcqlog.FatalE(err, "delete failed")
	}
}

type TableMap map[string]map[string]bool

func GetTableColumns(ctx context.Context) TableMap {
	q := `
	SELECT
		table_name,
		column_name
	FROM information_schema.columns
	WHERE table_schema='btcq_indexer'
	`
	rows, err := Query(ctx, q)
	if err != nil {
		btcqlog.FatalE(err, "Query error")
	}
	defer rows.Close()

	ret := TableMap{}
	for rows.Next() {
		var table, column string
		err := rows.Scan(&table, &column)
		if err != nil {
			btcqlog.FatalE(err, "Query error")
		}
		if _, ok := ret[table]; !ok {
			ret[table] = map[string]bool{}
		}
		ret[table][column] = true
	}
	return ret
}
