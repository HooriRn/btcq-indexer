package db

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/hex"
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

func GetMigrateUpdates(currentDdlHash md5Hash, tag string) (data []byte, err error) {
	currentDdlHashString := hex.EncodeToString(currentDdlHash[:])

	// Get the current chain id
	chainId, err := ReadChainID(context.Background())
	if err != nil {
		// Might be test environment without THORNode
		return nil, nil
	}

	if chainId != "thorchain" {
		btcqlog.Info("Skipping migration for non-mainnet")
		return nil, nil
	}

	latestHeight, err := GetLatestHeight(context.Background())
	if err != nil {
		btcqlog.FatalE(err, "Couldn't get latest height")
	}

	// v2.32.1 migration from v2.32.2
	if currentDdlHashString == "7da33d35fbcdb6f9ad375e199ba722e8" {

		// Trim migration for v2.32.2
		if latestHeight > 20996000 {
			btcqlog.Info("Trimming DB to height 20996001")
			TrimDB(context.Background(), 20996001)
		}

		data = []byte("UPDATE constants SET value = E'\\\\x741ae065783c76df8218e3a75298d633' WHERE key = 'ddl_hash';")
		currentDdlHashString = "741ae065783c76df8218e3a75298d633"
		latestHeight = 20996000
	}

	// v2.32.2 migration from v2.32.3
	if currentDdlHashString == "741ae065783c76df8218e3a75298d633" {
		// Trim migration for v2.32.3
		if latestHeight > 20996000 {
			btcqlog.Info("Trimming DB to height 20996001")
			TrimDB(context.Background(), 20996001)
		}

		data = []byte("UPDATE constants SET value = E'\\\\x38cc2cea31c31512f4d02ee13e36a91e' WHERE key = 'ddl_hash';")
		currentDdlHashString = "38cc2cea31c31512f4d02ee13e36a91e"
		latestHeight = 20996000
	}

	if currentDdlHashString == "38cc2cea31c31512f4d02ee13e36a91e" {
		// Trim migration
		if latestHeight > 21595000 {
			btcqlog.Info("Trimming DB to height 21595001")
			TrimDB(context.Background(), 21595001)
		}

		data = []byte("UPDATE constants SET value = E'\\\\xc6a7c41748d2d0cbee3f2337fc9f21d6' WHERE key = 'ddl_hash';")
		currentDdlHashString = "c6a7c41748d2d0cbee3f2337fc9f21d6"
		latestHeight = 21595000
	}

	// Aggregate update for v2.32.9
	if currentDdlHashString == "20d72bbaa9cade9fdfc90e432391edfd" {

		// Trim for THOR.NAMI
		if latestHeight > 22624800 {
			btcqlog.Info("Trimming DB to height 22624001")
			TrimDB(context.Background(), 22624001)
		}

		sql := `
			CREATE OR REPLACE VIEW btcq_indexer_agg.thorname_last_owner AS
			WITH owner_changes AS (
					SELECT 
						name, 
						owner, 
						block_timestamp,
						LAG(owner) OVER (PARTITION BY name ORDER BY block_timestamp) AS previous_owner
					FROM thorname_change_events
				)
				SELECT DISTINCT ON (name)
					name,
					block_timestamp
				FROM owner_changes
				WHERE owner <> previous_owner OR previous_owner IS NULL
				ORDER BY name, block_timestamp DESC;
		`

		hash := []byte("UPDATE constants SET value = E'\\\\x763034e303c4e588d46f6fb0fa68d598' WHERE key = 'aggregates_ddl_hash';")

		data = append(data, []byte(sql)...)
		data = append(data, hash...)
	}

	return data, nil
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
	WHERE materialized_table = 'actions' OR materialized_table = 'rune_price'
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
