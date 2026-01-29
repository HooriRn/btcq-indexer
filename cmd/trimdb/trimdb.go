package main

// Deletes all blocks including and after certain height.

// NOTE: You should trim to blocksheight % 100 == 0 for now.
// Since the block_log.agg_state is cleared for 99% of the blocks, this means
// Midgard can only restart from heights divisible by 100. This is planed to be fixed
// when Midgard can load depths from block_pool_depths on startup.

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/api"
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/db/dbinit"
	"github.com/btcq/btcq-indexer/internal/util/btcqlog"
)

func main() {
	btcqlog.LogCommandLine()

	// TODO(huginn): enforce this
	btcqlog.Warn("If Midgard is running, stop it and rerun this tool!")

	if len(os.Args) != 3 {
		btcqlog.FatalF("Provide 2 arguments, %d provided\nUsage: $ trimdb config heightOrTimestamp",
			len(os.Args)-1)
	}

	config.ReadGlobalFrom(os.Args[1])
	ctx := context.Background()

	dbinit.Setup()

	idStr := os.Args[2]
	heightOrTimestamp, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		btcqlog.FatalF("Couldn't parse height or timestamp: %s", idStr)
	}
	height, timestamp, err := api.TimestampAndHeight(ctx, heightOrTimestamp)
	if err != nil {
		btcqlog.FatalF("Couldn't find height for %d", heightOrTimestamp)
	}

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

func DeleteWatermark(value int64) {
	_, err := db.TheDB.Exec("UPDATE btcq_indexer_agg.watermarks SET watermark = $1", value)
	if err != nil {
		btcqlog.FatalE(err, "delete failed")
	}
}

func DeleteAfter(table string, columnName string, value int64) {
	q := fmt.Sprintf("DELETE FROM %s WHERE $1 <= %s", table, columnName)
	_, err := db.TheDB.Exec(q, value)
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
	rows, err := db.Query(ctx, q)
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
