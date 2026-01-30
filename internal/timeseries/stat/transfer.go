package stat

import (
	"context"
	"database/sql"

	"gitlab.com/thorchain/midgard/internal/db"
)

type PoolAdds struct {
	AssetE8Total int64
	RuneE8Total  int64
}

// TODO (HooriRn): All of these can be replaced by QueryOneValue
// Maybe we should delete these functions
func PoolAddsLookup(ctx context.Context, pool string, w db.Window) (*PoolAdds, error) {
	const q = `SELECT COALESCE(SUM(asset_e8), 0), COALESCE(SUM(rune_e8), 0)
FROM add_events
WHERE pool = $1 AND block_timestamp >= $2 AND block_timestamp < $3`

	var r PoolAdds
	err := db.TheDB.QueryRow(q, pool, w.From.ToNano(), w.Until.ToNano()).Scan(&r.AssetE8Total, &r.RuneE8Total)
	if err != nil {
		if err == sql.ErrNoRows {
			return &PoolAdds{
				AssetE8Total: 0,
				RuneE8Total:  0,
			}, nil
		}
		return nil, err
	}

	return &r, nil
}

type PoolErratas struct {
	AssetE8Total int64
	RuneE8Total  int64
}

func PoolErratasLookup(ctx context.Context, pool string, w db.Window) (*PoolErratas, error) {
	const q = `SELECT COALESCE(SUM(asset_e8), 0), COALESCE(SUM(rune_e8), 0) FROM errata_events
WHERE asset = $1 AND block_timestamp >= $2 AND block_timestamp < $3`

	var r PoolErratas
	err := db.TheDB.QueryRow(q, pool, w.From.ToNano(), w.Until.ToNano()).Scan(&r.AssetE8Total, &r.RuneE8Total)
	if err != nil {
		if err == sql.ErrNoRows {
			return &PoolErratas{
				AssetE8Total: 0,
				RuneE8Total:  0,
			}, nil
		}
		return nil, err
	}
	return &r, nil
}

type PoolFees struct {
	AssetE8Total    int64
	AssetE8Avg      float64
	PoolDeductTotal int64
}

func PoolFeesLookup(ctx context.Context, pool string, w db.Window) (PoolFees, error) {
	const q = `SELECT COALESCE(SUM(asset_e8), 0), COALESCE(AVG(asset_E8), 0), COALESCE(SUM(pool_deduct), 0) FROM fee_events
WHERE asset = $1 AND block_timestamp >= $2 AND block_timestamp < $3`

	var r PoolFees
	err := db.TheDB.QueryRow(q, pool, w.From.ToNano(), w.Until.ToNano()).Scan(&r.AssetE8Total, &r.AssetE8Avg, &r.PoolDeductTotal)
	if err != nil {
		if err == sql.ErrNoRows {
			return PoolFees{
				AssetE8Total:    0,
				AssetE8Avg:      0,
				PoolDeductTotal: 0,
			}, nil
		}
		return PoolFees{}, err
	}

	return r, nil
}

type PoolGas struct {
	AssetE8Total int64
	RuneE8Total  int64
}

func PoolGasLookup(ctx context.Context, pool string, w db.Window) (*PoolGas, error) {
	const q = `SELECT COALESCE(SUM(asset_e8), 0), COALESCE(SUM(rune_e8), 0)
FROM gas_events
WHERE asset = $1 AND block_timestamp >= $2 AND block_timestamp < $3`

	var r PoolGas
	err := db.TheDB.QueryRow(q, pool, w.From.ToNano(), w.Until.ToNano()).Scan(&r.AssetE8Total, &r.RuneE8Total)
	if err != nil {
		if err == sql.ErrNoRows {
			return &PoolGas{
				AssetE8Total: 0,
				RuneE8Total:  0,
			}, nil
		}
		return nil, err
	}

	return &r, nil
}

type PoolSlashes struct {
	AssetE8Total int64
}

func PoolSlashesLookup(ctx context.Context, pool string, w db.Window) (*PoolSlashes, error) {
	const q = `
		SELECT COALESCE(SUM(asset_e8), 0)
		FROM slash_events
		WHERE pool = $1 AND block_timestamp >= $2 AND block_timestamp < $3`

	var r PoolSlashes
	err := db.TheDB.QueryRow(q, pool, w.From.ToNano(), w.Until.ToNano()).Scan(&r.AssetE8Total)
	if err != nil {
		if err == sql.ErrNoRows {
			return &PoolSlashes{
				AssetE8Total: 0,
			}, nil
		}
		return nil, err
	}

	return &r, nil
}
