package timeseries

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/util/btcqerr"
	"github.com/btcq/btcq-indexer/openapi/generated/oapigen"
)

func blockIdFrom(strHeight string, strTimestamp string) (db.BlockId, btcqerr.Err) {
	if strHeight != "" && strTimestamp != "" {
		return db.BlockId{}, btcqerr.BadRequest("only one of height or timestamp can be specified, not both")
	}

	var height *int64
	var timestamp *db.Nano
	firstBlock := db.FirstBlock.Get()
	lastBlock := db.LastAggregatedBlock.Get()
	if strTimestamp != "" {
		ts, err := strconv.ParseInt(strTimestamp, 10, 64)
		if err != nil {
			return db.BlockId{}, btcqerr.BadRequestF("error parsing timestamp %s", strTimestamp)
		}
		second := db.Second(ts)
		firstSecond := db.Nano(firstBlock.Timestamp).ToSecond()
		lastSecond := db.Nano(lastBlock.Timestamp).ToSecond()
		if second < firstSecond || lastSecond < second {
			return db.BlockId{}, btcqerr.BadRequestF("no data for timestamp %v, timestamp range is [%v,%v]",
				second, firstSecond, lastSecond)
		}
		nano := second.ToNano() - 1 + 1e9
		timestamp = &nano
	} else if strHeight != "" {
		h, err := strconv.ParseInt(strHeight, 10, 64)
		if err != nil {
			return db.BlockId{}, btcqerr.BadRequestF("error parsing height %s", strHeight)
		}
		height = &h
		if *height < firstBlock.Height || lastBlock.Height < *height {
			return db.BlockId{}, btcqerr.BadRequestF("no data for height %s, height range is [%v,%v]",
				strHeight, firstBlock.Height, lastBlock.Height)
		}
	} else {
		return db.BlockId{
			Height:    lastBlock.Height,
			Timestamp: lastBlock.Timestamp,
		}, nil
	}

	return findBlockId(height, timestamp)
}

func findBlockId(height *int64, timestamp *db.Nano) (db.BlockId, btcqerr.Err) {
	var row *sql.Row
	query := "SELECT bl.height, bl.timestamp FROM block_log bl"
	if timestamp != nil {
		row = db.TheDB.QueryRow(query+" WHERE bl.timestamp <= $1 ORDER BY bl.timestamp DESC LIMIT 1", *timestamp)
	} else {
		row = db.TheDB.QueryRow(query+" WHERE height = $1", *height)
	}

	param := db.BlockId{}
	err := row.Scan(&param.Height, &param.Timestamp)
	if err != nil {
		return param, btcqerr.InternalErrE(err)
	}
	return param, nil
}

func GetBalances(ctx context.Context, address string, height string, timestamp string) (oapigen.BalanceResponse, btcqerr.Err) {
	blockId, merr := blockIdFrom(height, timestamp)

	if merr != nil {
		return oapigen.BalanceResponse{}, merr
	}

	rows, err := db.Query(ctx,
		`SELECT
			c.asset AS asset,
			b.amount_e8 AS amount_e8
		FROM
			btcq_indexer_agg.current_balances AS c,
			LATERAL (
				SELECT amount_e8
				FROM btcq_indexer_agg.balances
				WHERE addr = $1
					AND asset = c.asset
					AND block_timestamp <= $2
				ORDER BY block_timestamp DESC LIMIT 1
			) AS b
		WHERE c.addr = $1
		ORDER BY asset`,
		address,
		blockId.Timestamp,
	)

	if err != nil {
		return oapigen.BalanceResponse{}, btcqerr.InternalErrE(err)
	}

	result := oapigen.BalanceResponse{
		Height: fmt.Sprintf("%d", blockId.Height),
		Date:   fmt.Sprintf("%d", blockId.Timestamp),
		Coins:  oapigen.Coins{},
	}

	for rows.Next() {
		var asset, amount string
		err := rows.Scan(&asset, &amount)
		if err != nil {
			return oapigen.BalanceResponse{}, btcqerr.InternalErrE(err)
		}
		result.Coins = append(result.Coins, oapigen.Coin{
			Amount: amount,
			Asset:  asset,
		})
	}

	return result, nil
}

