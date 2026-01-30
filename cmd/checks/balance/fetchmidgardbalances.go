package main

import (
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

func readMidgardBalancesAt(timestamp int64) map[string]Balance {
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
				midgard_agg.balances
			WHERE
				block_timestamp <= $1
			) AS x
		WHERE
			row_number = 1`,
		timestamp)
	if err != nil {
		midlog.FatalE(err, "Error querying midgard balances")
	}
	defer rows.Close()
	balances := map[string]Balance{}
	for rows.Next() {
		b := Balance{}
		err := rows.Scan(&b.addr, &b.asset, &b.amountE8)
		if err != nil {
			midlog.FatalE(err, "Error reading account balances")
		}
		balances[b.key()] = b
	}
	return balances
}
