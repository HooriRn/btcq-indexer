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

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/api"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/dbinit"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

func main() {
	midlog.LogCommandLine()

	// TODO(huginn): enforce this
	midlog.Warn("If Midgard is running, stop it and rerun this tool!")

	if len(os.Args) != 3 {
		midlog.FatalF("Provide 2 arguments, %d provided\nUsage: $ trimdb config heightOrTimestamp",
			len(os.Args)-1)
	}

	config.ReadGlobalFrom(os.Args[1])
	ctx := context.Background()

	dbinit.Setup()

	idStr := os.Args[2]
	heightOrTimestamp, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		midlog.FatalF("Couldn't parse height or timestamp: %s", idStr)
	}
	height, timestamp, err := api.TimestampAndHeight(ctx, heightOrTimestamp)
	if err != nil {
		midlog.FatalF("Couldn't find height for %d", heightOrTimestamp)
	}

	midlog.Info("Deleting actions")
	DeleteAfter("midgard_agg.actions", "block_timestamp", timestamp.ToI())
	DeleteAfter("midgard_agg.rune_price", "block_timestamp", timestamp.ToI())
	midlog.Info("Deleting watermark")
	DeleteWatermark(timestamp.ToI())

	midlog.InfoF("Deleting rows including and after height %d , timestamp %d", height, timestamp)
	tables := GetTableColumns(ctx)
	for table, columns := range tables {
		if columns["block_timestamp"] {
			midlog.InfoF("%s  deleting by block_timestamp", table)
			DeleteAfter(table, "block_timestamp", timestamp.ToI())
		} else if columns["height"] {
			midlog.InfoF("%s deleting by height", table)
			DeleteAfter(table, "height", height)
		} else if table == "constants" {
			midlog.InfoF("Skipping table %s", table)
		} else {
			midlog.WarnF("talbe %s has no good column", table)
		}
	}
}

func DeleteWatermark(value int64) {
	_, err := db.TheDB.Exec("UPDATE midgard_agg.watermarks SET watermark = $1", value)
	if err != nil {
		midlog.FatalE(err, "delete failed")
	}
}

func DeleteAfter(table string, columnName string, value int64) {
	q := fmt.Sprintf("DELETE FROM %s WHERE $1 <= %s", table, columnName)
	_, err := db.TheDB.Exec(q, value)
	if err != nil {
		midlog.FatalE(err, "delete failed")
	}
}

type TableMap map[string]map[string]bool

func GetTableColumns(ctx context.Context) TableMap {
	q := `
	SELECT
		table_name,
		column_name
	FROM information_schema.columns
	WHERE table_schema='midgard'
	`
	rows, err := db.Query(ctx, q)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	defer rows.Close()

	ret := TableMap{}
	for rows.Next() {
		var table, column string
		err := rows.Scan(&table, &column)
		if err != nil {
			midlog.FatalE(err, "Query error")
		}
		if _, ok := ret[table]; !ok {
			ret[table] = map[string]bool{}
		}
		ret[table][column] = true
	}
	return ret
}
