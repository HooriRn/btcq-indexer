package timeseries

import (
	"context"

	"github.com/lib/pq"
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/fetch/record"
	"github.com/btcq/btcq-indexer/internal/util"
	"github.com/btcq/btcq-indexer/openapi/generated/oapigen"
)

// GetMemberIds returns the ids of all known members.
//
// The id of a member is defined as their qbtc address if they are participating with their qbtc
// address, or as their asset address otherwise (for members with asset address only.)
//
// Member ids present in multiple pools will be only returned once.
func GetMemberIds(ctx context.Context, pool *string) (addrs []string, err error) {
	poolFilter := ""
	qargs := []interface{}{}
	if pool != nil {
		poolFilter = "pool = $1"
		qargs = append(qargs, pool)
	}

	q := "SELECT DISTINCT member_id FROM btcq_indexer_agg.members " + db.Where(poolFilter)

	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var member string
		err := rows.Scan(&member)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, member)
	}

	return addrs, nil
}

// TODO(HooriRn): this struct might not be needed since the graphql depracation. (delete-graphql)
// Info of a member in a specific pool.
type MemberPool struct {
	Pool           string
	QbtcAddress    string
	AssetAddress   string
	LiquidityUnits int64
	AssetDeposit   int64
	QbtcDeposit    int64
	QbtcAdded      int64
	AssetAdded     int64
	QbtcPending    int64
	AssetPending   int64
	DateFirstAdded int64
	DateLastAdded  int64
	QbtcWithdrawn  int64
	AssetWithdrawn int64
}

func (memberPool MemberPool) toOapigen() oapigen.MemberPool {
	return oapigen.MemberPool{
		Pool:           memberPool.Pool,
		QbtcAddress:    memberPool.QbtcAddress,
		AssetAddress:   memberPool.AssetAddress,
		LiquidityUnits: util.IntStr(memberPool.LiquidityUnits),
		QbtcDeposit:    util.IntStr(memberPool.QbtcDeposit),
		AssetDeposit:   util.IntStr(memberPool.AssetDeposit),
		QbtcAdded:      util.IntStr(memberPool.QbtcAdded),
		AssetAdded:     util.IntStr(memberPool.AssetAdded),
		QbtcWithdrawn:  util.IntStr(memberPool.QbtcWithdrawn),
		AssetWithdrawn: util.IntStr(memberPool.AssetWithdrawn),
		QbtcPending:    util.IntStr(memberPool.QbtcPending),
		AssetPending:   util.IntStr(memberPool.AssetPending),
		DateFirstAdded: util.IntStr(memberPool.DateFirstAdded),
		DateLastAdded:  util.IntStr(memberPool.DateLastAdded),
	}
}

func (memberPool MemberPool) toSavers() oapigen.SaverPool {
	return oapigen.SaverPool{
		Pool:           util.ConvertSynthPoolToNative(memberPool.Pool),
		AssetAddress:   memberPool.AssetAddress,
		AssetAdded:     util.IntStr(memberPool.AssetAdded),
		AssetDeposit:   util.IntStr(memberPool.AssetDeposit),
		SaverUnits:     util.IntStr(memberPool.LiquidityUnits),
		AssetWithdrawn: util.IntStr(memberPool.AssetWithdrawn),
		DateFirstAdded: util.IntStr(memberPool.DateFirstAdded),
		DateLastAdded:  util.IntStr(memberPool.DateLastAdded),
	}
}

// Pools data associated with a single member
type MemberPools []MemberPool

func (memberPools MemberPools) ToOapigen() []oapigen.MemberPool {
	ret := make([]oapigen.MemberPool, len(memberPools))
	for i, memberPool := range memberPools {
		ret[i] = memberPool.toOapigen()
	}

	return ret
}

func (memberPools MemberPools) ToSavers(poolRedeemValueMap map[string]int64) []oapigen.SaverPool {
	ret := make([]oapigen.SaverPool, len(memberPools))
	for i, memberPool := range memberPools {
		ret[i] = memberPool.toSavers()
		ret[i].AssetRedeem = util.IntStr(poolRedeemValueMap[memberPool.Pool])
	}

	return ret
}

type MemberPoolType int

const (
	RegularAndSaverPools MemberPoolType = iota // regular and synth pools too
	RegularPools                               // regular (non-synth) pools e.g. 'BTC.BTC'
	SaverPools                                 // LPs of synth pools e.g. 'BTC/BTC'
)

func PoolBasedOfType(poolName string, poolType MemberPoolType) bool {
	if poolType == RegularAndSaverPools {
		return true
	}
	poolCoinType := record.GetCoinType([]byte(poolName))
	if poolCoinType == record.AssetSynth && poolType == SaverPools {
		return true
	}
	if poolCoinType == record.AssetNative && poolType == RegularPools {
		return true
	}
	return false
}

func GetMemberPools(ctx context.Context, address []string, poolType MemberPoolType) (MemberPools, error) {
	q := `
		SELECT
			pool,
			COALESCE(qbtc_addr, ''),
			COALESCE(asset_addr, ''),
			lp_units_total,
			asset_e8_deposit,
			qbtc_e8_deposit,
			added_qbtc_e8_total,
			added_asset_e8_total,
			withdrawn_qbtc_e8_total,
			withdrawn_asset_e8_total,
			pending_qbtc_e8_total,
			pending_asset_e8_total,
			COALESCE(first_added_timestamp / 1000000000, 0),
			COALESCE(last_added_timestamp / 1000000000, 0)
		FROM btcq_indexer_agg.members
		WHERE member_id = ANY($1) OR asset_addr = ANY($1)
		ORDER BY pool
	`

	rows, err := db.Query(ctx, q, pq.Array(address))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results MemberPools
	for rows.Next() {
		var entry MemberPool
		err := rows.Scan(
			&entry.Pool,
			&entry.QbtcAddress,
			&entry.AssetAddress,
			&entry.LiquidityUnits,
			&entry.AssetDeposit,
			&entry.QbtcDeposit,
			&entry.QbtcAdded,
			&entry.AssetAdded,
			&entry.QbtcWithdrawn,
			&entry.AssetWithdrawn,
			&entry.QbtcPending,
			&entry.AssetPending,
			&entry.DateFirstAdded,
			&entry.DateLastAdded,
		)
		if err != nil {
			return nil, err
		}
		if PoolBasedOfType(entry.Pool, poolType) {
			results = append(results, entry)
		}
	}
	return results, nil
}
