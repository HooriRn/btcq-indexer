package timeseries

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func blockIdFrom(strHeight string, strTimestamp string) (db.BlockId, miderr.Err) {
	if strHeight != "" && strTimestamp != "" {
		return db.BlockId{}, miderr.BadRequest("only one of height or timestamp can be specified, not both")
	}

	var height *int64
	var timestamp *db.Nano
	firstBlock := db.FirstBlock.Get()
	lastBlock := db.LastAggregatedBlock.Get()
	if strTimestamp != "" {
		ts, err := strconv.ParseInt(strTimestamp, 10, 64)
		if err != nil {
			return db.BlockId{}, miderr.BadRequestF("error parsing timestamp %s", strTimestamp)
		}
		second := db.Second(ts)
		firstSecond := db.Nano(firstBlock.Timestamp).ToSecond()
		lastSecond := db.Nano(lastBlock.Timestamp).ToSecond()
		if second < firstSecond || lastSecond < second {
			return db.BlockId{}, miderr.BadRequestF("no data for timestamp %v, timestamp range is [%v,%v]",
				second, firstSecond, lastSecond)
		}
		nano := second.ToNano() - 1 + 1e9
		timestamp = &nano
	} else if strHeight != "" {
		h, err := strconv.ParseInt(strHeight, 10, 64)
		if err != nil {
			return db.BlockId{}, miderr.BadRequestF("error parsing height %s", strHeight)
		}
		height = &h
		if *height < firstBlock.Height || lastBlock.Height < *height {
			return db.BlockId{}, miderr.BadRequestF("no data for height %s, height range is [%v,%v]",
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

func findBlockId(height *int64, timestamp *db.Nano) (db.BlockId, miderr.Err) {
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
		return param, miderr.InternalErrE(err)
	}
	return param, nil
}

func GetBalances(ctx context.Context, address string, height string, timestamp string) (oapigen.BalanceResponse, miderr.Err) {
	blockId, merr := blockIdFrom(height, timestamp)

	if merr != nil {
		return oapigen.BalanceResponse{}, merr
	}

	rows, err := db.Query(ctx,
		`SELECT
			c.asset AS asset,
			b.amount_e8 AS amount_e8
		FROM
			midgard_agg.current_balances AS c,
			LATERAL (
				SELECT amount_e8
				FROM midgard_agg.balances
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
		return oapigen.BalanceResponse{}, miderr.InternalErrE(err)
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
			return oapigen.BalanceResponse{}, miderr.InternalErrE(err)
		}
		result.Coins = append(result.Coins, oapigen.Coin{
			Amount: amount,
			Asset:  asset,
		})
	}

	return result, nil
}

func GetTopHolders(ctx context.Context, asset string, limit int64) (oapigen.HolderResponse, miderr.Err) {
	rows, err := db.Query(ctx,
		`SELECT
			c.addr AS address,
			c.asset AS asset,
			c.amount_e8 AS amount_e8
		FROM
			midgard_agg.current_balances AS c
		WHERE c.asset = $1 AND c.amount_e8 > 0
		ORDER BY amount_e8 desc
		limit $2`,
		asset,
		limit,
	)

	if err != nil {
		return oapigen.HolderResponse{}, miderr.InternalErrE(err)
	}

	result := oapigen.HolderResponse{}

	for rows.Next() {
		var address, queryAsset, amount string
		err := rows.Scan(&address, &queryAsset, &amount)
		if err != nil {
			return oapigen.HolderResponse{}, miderr.InternalErrE(err)
		}

		// Create a holder with the address and amount
		coins := oapigen.Coins{
			oapigen.Coin{
				Amount: amount,
				Asset:  queryAsset,
			},
		}
		result = append(result, oapigen.Holder{
			Address: address,
			Coins:   coins,
		})
	}

	return result, nil
}
