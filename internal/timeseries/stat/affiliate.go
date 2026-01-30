package stat

import (
	"context"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

type AffiliateItem struct {
	Time      db.Second
	Count     int64
	Volume    int64
	VolumeUSD int64
	THORName  string
}

type AffiliateBucket struct {
	StartTime db.Second
	EndTime   db.Second
	Count     int64
	Volume    int64
	VolumeUSD int64
	Thornames []oapigen.ItemThorname
}

type AffiliateMeta struct {
	Count     int64
	Volume    int64
	VolumeUSD int64
}

var AffiliateAggregate = db.RegisterAggregate(db.NewAggregate("affiliates", "affiliate_fee_events").
	AddJoinQuery("rune_price", "r").
	AddGroupColumn("thorname").
	AddSumlikeExpression("affiliate_volume_in_rune_e8",
		`SUM(_fee_amt_in_rune)::BIGINT`).
	AddSumlikeExpression("affiliate_volume_usd_e8",
		`SUM(_fee_amt_in_rune * r.rune_price_e8 / 1e6)::BIGINT`).
	AddSumlikeExpression("affiliate_count", "COUNT(1)"))

func GetAffiliateFeeBuckets(ctx context.Context, thorname *string, buckets db.Buckets) (
	[]AffiliateItem, error) {

	filters := []string{}
	params := []interface{}{}
	if thorname != nil {
		filters = append(filters, "thorname = $1")
		params = append(params, *thorname)
	}
	q, params := AffiliateAggregate.BucketedQuery(`
			SELECT
				aggregate_timestamp/1000000000 as time,
				thorname,
				SUM(affiliate_count) AS count,
				SUM(affiliate_volume_in_rune_e8) AS volume,
				SUM(affiliate_volume_usd_e8) AS volume_usd
			FROM %s
			GROUP BY time, thorname
			ORDER BY time ASC
		`, buckets, filters, params)

	rows, err := db.Query(ctx, q, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ret := []AffiliateItem{}
	for rows.Next() {
		var bucket AffiliateItem
		err := rows.Scan(&bucket.Time, &bucket.THORName, &bucket.Count, &bucket.Volume, &bucket.VolumeUSD)
		if err != nil {
			return []AffiliateItem{}, err
		}
		ret = append(ret, bucket)
	}
	return ret, rows.Err()
}

func GetTHORNameAffiliate(ctx context.Context, thorname *string, buckets db.Buckets) (oapigen.AffiliateHistoryResponse, error) {
	affiliates, err := GetAffiliateFeeBuckets(ctx, thorname, buckets)
	if err != nil {
		return oapigen.AffiliateHistoryResponse{}, err
	}
	usdPrice, err := USDPriceHistory(ctx, buckets)
	if err != nil {
		return oapigen.AffiliateHistoryResponse{}, err
	}

	return mergeAffiliateGapfill(affiliates, usdPrice), nil
}

func intervalsToResponse(intervals []AffiliateBucket) []oapigen.AffiliateHistoryItem {
	ret := make([]oapigen.AffiliateHistoryItem, len(intervals))
	for i, interval := range intervals {
		ret[i] = oapigen.AffiliateHistoryItem{
			StartTime: util.IntStr(interval.StartTime.ToI()),
			EndTime:   util.IntStr(interval.EndTime.ToI()),
			Count:     util.IntStr(interval.Count),
			Volume:    util.IntStr(interval.Volume),
			VolumeUSD: util.IntStr(interval.VolumeUSD),
			Thornames: interval.Thornames,
		}
	}
	return ret
}

func mergeAffiliateGapfill(affiliates []AffiliateItem,
	denseUSDPrices []USDPriceBucket) oapigen.AffiliateHistoryResponse {
	intervals := make([]AffiliateBucket, len(denseUSDPrices))
	meta := AffiliateMeta{}

	timeAfterLast := denseUSDPrices[len(denseUSDPrices)-1].Window.Until + 1
	affiliates = append(affiliates, AffiliateItem{Time: timeAfterLast})

	idx := 0
	for i, usdPrice := range denseUSDPrices {
		current := &intervals[i]
		current.StartTime = (usdPrice.Window.From)
		current.EndTime = (usdPrice.Window.Until)
		current.Thornames = []oapigen.ItemThorname{}
		for idx < len(affiliates) && affiliates[idx].Time == usdPrice.Window.From {
			affiliate := &affiliates[idx]
			current.Count += affiliate.Count
			current.Volume += affiliate.Volume
			current.VolumeUSD += affiliate.VolumeUSD

			// Fill Thornames
			current.Thornames = append(current.Thornames, oapigen.ItemThorname{
				Thorname:  affiliate.THORName,
				Count:     util.IntStr(affiliate.Count),
				Volume:    util.IntStr(affiliate.Volume),
				VolumeUSD: util.IntStr(affiliate.VolumeUSD),
			})

			// Calculate Meta
			meta.Count += affiliate.Count
			meta.Volume += affiliate.Volume
			meta.VolumeUSD += affiliate.VolumeUSD

			idx++
		}
	}

	return oapigen.AffiliateHistoryResponse{
		Intervals: intervalsToResponse(intervals),
		Meta: oapigen.AffiliateHistoryMeta{
			Count:     util.IntStr(meta.Count),
			Volume:    util.IntStr(meta.Volume),
			VolumeUSD: util.IntStr(meta.VolumeUSD),
			StartTime: util.IntStr(denseUSDPrices[0].Window.From.ToI()),
			EndTime:   util.IntStr(denseUSDPrices[len(denseUSDPrices)-1].Window.Until.ToI()),
		},
	}
}
