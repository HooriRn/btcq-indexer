package main

import (
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/util/btcqlog"
)

func readBalancesAt(timestamp int64) map[string]Balance {
	rows, err := db.TheDB.Query(
		`SELECT 
			addr,asset,amount_e8
		FROM (
			SELECT 
				row_number() OVER (PARTITION BY addr, asset ORDER BY block_timestamp DESC) as row_number,
				addr,
				asset,
				amount_e8
			FROM
				btcq_indexer_agg.balances
			WHERE
				block_timestamp <= $1
			) AS x
		WHERE
			row_number = 1`,
		timestamp)
	if err != nil {
		btcqlog.FatalE(err, "Error querying indexer balances")
	}
	defer rows.Close()
	balances := map[string]Balance{}
	for rows.Next() {
		b := Balance{}
		err := rows.Scan(&b.addr, &b.asset, &b.amountE8)
		if err != nil {
			btcqlog.FatalE(err, "Error reading account balances")
		}
		balances[b.key()] = b
	}
	return balances
}
