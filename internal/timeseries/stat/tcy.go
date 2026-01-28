package stat

import (
	"context"

	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/util"
	"github.com/btcq/btcq-indexer/openapi/generated/oapigen"
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
				AVG(rune_e8::DOUBLE PRECISION / asset_e8::DOUBLE PRECISION),
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

func GetTCYDistribution(ctx context.Context, address string, period db.Buckets) (oapigen.TCYDistribution, error) {
	addressFilter := ""
	qargs := []interface{}{}
	if address != "" {
		addressFilter = "rune_address = $1"
		qargs = []interface{}{address}
	}

	q := `
		SELECT
			SUM(t.rune_amt),
			t.rune_address,
			t.block_timestamp,
			AVG(r.rune_price_e8 * 1e8)::BIGINT as rune_price_e8
		FROM tcy_distribution_events t
		JOIN rune_price r
			ON t.block_timestamp = r.block_timestamp
		` + db.Where(addressFilter) + `
		GROUP BY t.rune_address, t.block_timestamp
	`

	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		return oapigen.TCYDistribution{}, err
	}
	defer rows.Close()

	var distributionItems []oapigen.TCYDistributionItem
	var total int64
	var lastMonthEarnings int64 = 0
	for rows.Next() {
		var runeAmt int64
		var blockTimestamp int64
		var runePrice int64
		err := rows.Scan(&runeAmt, &address, &blockTimestamp, &runePrice)
		if err != nil {
			return oapigen.TCYDistribution{}, err
		}
		total += runeAmt
		distributionItems = append(distributionItems, oapigen.TCYDistributionItem{
			Amount: util.IntStr(runeAmt),
			Date:   util.IntStr(blockTimestamp / 1e9),
			Price:  util.IntStr(runePrice),
		})
		if period.Start().ToNano().ToI() <= blockTimestamp {
			lastMonthEarnings += runeAmt
		}
	}

	staked, err := getStakedTCY(ctx, address)
	if err != nil {
		return oapigen.TCYDistribution{}, err
	}

	tcyRunePrice, err := getTCYPriceBucket(ctx, period)
	if err != nil {
		return oapigen.TCYDistribution{}, err
	}

	// If the first distribution is before the start of the period, we need to adjust the bucket
	// to ensure we calculate APR correctly.
	if len(distributionItems) > 0 {
		firstTime := util.MustParseInt64(distributionItems[0].Date)
		if firstTime > period.Start().ToI() {
			period = db.Buckets{Timestamps: db.Seconds{db.Second(firstTime), period.End()}}
		}
	}

	periodsPerYear := db.GetPPYFromBuckets(period)
	var apr float64
	if staked > 0 {
		apr = float64(lastMonthEarnings) / (float64(staked) * tcyRunePrice) * periodsPerYear
	}

	ret := oapigen.TCYDistribution{
		Total:         util.IntStr(total),
		Address:       address,
		Distributions: distributionItems,
		Apr:           util.FloatStr(apr),
	}

	return ret, nil
}
