package stat

import (
	"context"

	"github.com/btcq/btcq-indexer/internal/db"
)

func getStakedTCY(ctx context.Context, address string) (int64, error) {
	qargs := []interface{}{address}

	q := `
		SELECT COALESCE(SUM(amount), 0) AS staked
		FROM (
			SELECT tcy_amt AS amount
			FROM tcy_claim_events
			WHERE rune_address = $1
			
			UNION ALL

			SELECT amount
			FROM tcy_stake_events
			WHERE rune_address = $1
			
			UNION ALL

			SELECT -amount
			FROM tcy_unstake_events
			WHERE rune_address = $1
		) AS combined;
	`

	row := db.TheDB.QueryRow(q, qargs...)
	var stakedTCY int64
	err := row.Scan(&stakedTCY)
	if err != nil {
		return 0, err
	}

	return stakedTCY, nil
}

func getTCYPriceBucket(ctx context.Context, w db.Buckets) (float64, error) {
	queryArguments := []interface{}{w.Start().ToNano(), w.End().ToNano()}

	q := `
		SELECT 
			COALESCE(
				AVG(qbtc_e8::DOUBLE PRECISION / asset_e8::DOUBLE PRECISION),
				0
			) AS average_price
		FROM block_pool_depths
		WHERE $1 <= block_timestamp AND block_timestamp < $2
		AND asset_e8 > 0 AND pool = 'THOR.TCY';
	`

	row := db.TheDB.QueryRow(q, queryArguments...)
	var avgPrice float64
	err := row.Scan(&avgPrice)
	if err != nil {
		return 0, err
	}

	return avgPrice, nil
}

