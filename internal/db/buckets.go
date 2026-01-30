package db

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
)

// Window specifies the applicable time period.
type Window struct {
	From  Second // lower bound [inclusive]
	Until Second // upper bound [exclusive]
}

type Interval int

const (
	Min5 Interval = iota
	Hour
	Day
	Week
	Month
	Quarter
	Year
	UndefinedInterval
)

type IntervalDescription struct {
	interval Interval
	// Name used in JSON API and in aggregate views
	name string
	// This is used for the sql `date_trunc` function.	`date_trunc` does not accept '5 minute'
	// as a parameter, instead we round every timestamp to the nearest 5min
	// with (timestamp / 300) * 300.
	// TODO(huginn): remove when all aggregates are updated
	dateTruncName string
	// Can this interval be used for TimescaleDB continuous aggregating?
	// The interval have to be exact number of seconds (and also start exactly at epoch, which
	// makes 'week' inexact)
	exact bool
	// Lower bound on duration in seconds.
	// Used for gapfill, to make sure every interval has a value.
	// For `exact` intervals the minDuration and maxDuration should be equal.
	minDuration Second
	// Upper bound on duration in seconds.
	// Used for extending bounds at least to the next occurrence.
	maxDuration Second
}

var intervals = [...]IntervalDescription{
	{Min5, "5min", "minute", true, 60 * 5, 60 * 5},
	{Hour, "hour", "hour", true, 60 * 60, 60 * 60},
	{Day, "day", "day", true, 60 * 60 * 24, 60 * 60 * 24},
	{Week, "week", "week", false, 60 * 60 * 24 * 7, 60 * 60 * 24 * 7},
	{Month, "month", "month", false, 60 * 60 * 24 * 28, 60 * 60 * 24 * 31},
	{Quarter, "quarter", "quarter", false, 60 * 60 * 24 * 28 * 3, 60 * 60 * 24 * 31 * 3},
	{Year, "year", "year", false, 60 * 60 * 24 * 28 * 12, 60 * 60 * 24 * 31 * 12},
}

// Convenience maps for the `intervals`
var (
	intervalMap              map[Interval]IntervalDescription
	intervalFromJSONParamMap map[string]Interval
)

// Initialize the above convenience maps
func init() {
	intervalMap = make(map[Interval]IntervalDescription)
	intervalFromJSONParamMap = make(map[string]Interval)
	for _, i := range intervals {
		intervalMap[i.interval] = i
		intervalFromJSONParamMap[i.name] = i.interval
	}
}

type Seconds []Second

// Bucketing has two modes:
// a) If interval is given then all timestamps are rounded to the interval boundaries.
//
//	Timestamps contains count+1 timestamps, so the last timestamp should be the endTime
//	of the last bucket.
//
// b) If interval is nill, then it's an exact search with from..to parameters.
//
//	In this case there are exactly two Timestamps.
type Buckets struct {
	Timestamps Seconds
	interval   *Interval
}

func OneIntervalBuckets(from, to Second) Buckets {
	return Buckets{Timestamps: Seconds{from, to}}
}

func AllHistoryBuckets() Buckets {
	return Buckets{Timestamps: Seconds{FirstBlock.Get().Timestamp.ToSecond(), NowSecond()}}
}

func (b Buckets) Start() Second {
	return b.Timestamps[0]
}

func (b Buckets) End() Second {
	return b.Timestamps[len(b.Timestamps)-1]
}

func (b Buckets) Count() int {
	return len(b.Timestamps) - 1
}

func (b Buckets) Bucket(idx int) (startTime, endTime Second) {
	return b.Timestamps[idx], b.Timestamps[idx+1]
}

func (b Buckets) BucketWindow(idx int) Window {
	return Window{b.Timestamps[idx], b.Timestamps[idx+1]}
}

func (b Buckets) Window() Window {
	return Window{b.Start(), b.End()}
}

func (b Buckets) OneInterval() bool {
	return b.interval == nil
}

const (
	maxIntervalCount   = 400
	cutoffWindowLength = 2 * maxIntervalCount
)

// Returns all the buckets for the window, so other queries don't have to care about gapfill functionality.
func generateTimestamps(ctx context.Context, interval Interval, w Window) (Seconds, miderr.Err) {
	// We use an SQL query to use the date_trunc of sql.
	// It's not important which table we select we just need a timestamp type and we use WHERE 1=0
	// in order not to actually select any data.
	// We could consider writing an sql function instead or programming dategeneration in go.

	intervalParams := intervalMap[interval]
	if intervalParams.maxDuration*cutoffWindowLength < (w.Until - w.From) {
		return nil, miderr.BadRequestF(
			"Too wide range requested, max allowed intervals (%d).\n%s",
			maxIntervalCount, usage)
	}

	q := `
		WITH gapfill AS (
			SELECT
				time_bucket_gapfill($1::BIGINT, block_timestamp, $2::BIGINT, $3::BIGINT) as bucket
			FROM block_pool_depths
			WHERE 1=0
			GROUP BY bucket)
		SELECT
			EXTRACT(EPOCH FROM
				date_trunc($4, to_timestamp(bucket/300*300)))::BIGINT as truncated
		FROM gapfill
		GROUP BY truncated
		ORDER BY truncated ASC
	`

	// Widen from and until a bit, to make sure we don't loose any at edges.
	untilParam := w.Until + intervalParams.maxDuration + intervalParams.minDuration
	fromParam := w.From - intervalParams.maxDuration - intervalParams.minDuration
	rows, err := Query(ctx, q, intervalParams.minDuration,
		fromParam, untilParam,
		intervalParams.dateTruncName)
	if err != nil {
		return nil, miderr.InternalErrE(err)
	}
	defer rows.Close()

	timestamps := []Second{}
	for rows.Next() {
		var timestamp Second
		err := rows.Scan(&timestamp)
		if err != nil {
			return nil, miderr.InternalErrE(err)
		}
		timestamps = append(timestamps, timestamp)
	}

	// Leave exactly one timestamp bigger than Until
	lastIdx := len(timestamps) - 1
	for ; 0 < lastIdx && w.Until <= timestamps[lastIdx-1]; lastIdx-- {
	}
	firstIdx := 0
	for ; firstIdx < len(timestamps) && timestamps[firstIdx+1] <= w.From; firstIdx++ {
	}
	ret := timestamps[firstIdx : lastIdx+1]

	if len(ret) < 2 {
		// We need at least 2 elements to have an [from, to) interval.
		return nil, miderr.BadRequestF(
			"No interval requested. Use count or a wider from/to range.\n%s", usage)
	}
	return ret, nil
}

// TODO(acsaba): Migrate graphql to use GenerateBuckets.
func BucketsFromWindow(ctx context.Context, window Window, interval Interval) (ret Buckets, merr miderr.Err) {
	ret.interval = &interval
	ret.Timestamps, merr = generateTimestamps(ctx, *ret.interval, window)
	if merr != nil {
		return
	}
	if maxIntervalCount < ret.Count() {
		return Buckets{}, miderr.BadRequestF("Too wide range requested: %d, max allowed intervals (%d).\n%s",
			ret.Count(), maxIntervalCount, usage)
	}
	return
}

const usage = `Usage:

With interval parameter you get a series of buckets:
- Interval possible values: 5min, hour, day, week, month, quarter, year.
- count: optional int, (1..100)
- from/to: optional int, unix second.

Possible configurations with interval:
- ?interval=day&count=10                       - last 10 days.
- ?interval=day&count=10&to=1608825600         - last 10 days before to.
- ?interval=day&count=10&from=1606780800       - next 10 days after from.
- ?interval=day&from=1606780800&to=1608825600  - days between from and to, returns only the first 100.
- ?interval=year                               - same as interval=year&from=start_of_chain&to=now

Without interval you get only one interval:
- ?from=1606780842&to=1608825642               - only meta for this interval
- ?from=1606780842                             - until now
- ?to=1608825642                               - since start of chain
- no parameters                                - since start of chain until now
`

// Trim timestamps to [firstblock, lastblock)
// - Leaves at least two timestamps.
// - Leave max one timestamp after the last block (note: that won't be a start time)
// - Leave max one timestamp before the first block.
func restrictBuckets(firstBlock, lastBlock Second, buckets *Buckets) {
	firstok, lastok := 0, len(buckets.Timestamps)-1

	for 1 < lastok && lastBlock < buckets.Timestamps[lastok-1] {
		lastok--
	}
	for firstok < lastok-1 && buckets.Timestamps[firstok+1] < firstBlock {
		firstok++
	}
	buckets.Timestamps = buckets.Timestamps[firstok : lastok+1]
}

func generateBucketsWithInterval(ctx context.Context, from, to *Second, count *int64, interval Interval) (ret Buckets, merr miderr.Err) {
	firstSecond := FirstBlock.Get().Timestamp.ToSecond()
	nowSecond := NowSecond()

	if count == nil {
		if from == nil {
			from = &firstSecond
		}
		if to == nil {
			to = &nowSecond
		}
		ret.interval = &interval
		ret.Timestamps, merr = generateTimestamps(ctx, *ret.interval, Window{From: *from, Until: *to})
		if merr != nil {
			return
		}
		if maxIntervalCount < ret.Count() {
			ret.Timestamps = ret.Timestamps[:maxIntervalCount+1]
		}
		restrictBuckets(firstSecond, nowSecond, &ret)
		return ret, nil
	}
	// count != nil

	if *count < 1 || maxIntervalCount < *count {
		return Buckets{}, miderr.BadRequestF("Count out of range: %d, allowed [1..%d].\n%s",
			*count, maxIntervalCount, usage)
	}
	requestedCountInt := (int)(*count)
	if from != nil && to != nil {
		return Buckets{}, miderr.BadRequestF(
			"Count and from and to was specified. Specify max 2 of them.\n%s", usage)
	}
	if from == nil && to == nil {
		to = &nowSecond
	}
	ret.interval = &interval
	if to != nil {
		// to & count was given
		window := Window{From: *to - Second(*count)*intervalMap[interval].maxDuration, Until: *to}
		ret.Timestamps, merr = generateTimestamps(ctx, *ret.interval, window)
		if merr != nil {
			return
		}
		restrictBuckets(firstSecond, nowSecond, &ret)
		if requestedCountInt < ret.Count() {
			// We might have more intervals then requested, trim the beginning.
			ret.Timestamps = ret.Timestamps[ret.Count()-(requestedCountInt):]
		}
		return
	} else {
		// from & count was given
		window := Window{From: *from, Until: *from + Second(*count)*intervalMap[interval].maxDuration}
		ret.Timestamps, merr = generateTimestamps(ctx, *ret.interval, window)
		if merr != nil {
			return
		}
		restrictBuckets(firstSecond, nowSecond, &ret)
		if requestedCountInt < ret.Count() {
			// We might have more intervals then requested, trim the end.
			ret.Timestamps = ret.Timestamps[:requestedCountInt+1]
		}
		return
	}
}

// No interval was provided, we do a single From..To query
func generateBucketsOnlyMeta(ctx context.Context, fromP, toP *Second, count *int64) (ret Buckets, merr miderr.Err) {
	if count != nil {
		return Buckets{}, miderr.BadRequestF(
			"count was provided but no interval parameter.\n%s", usage)
	}
	if toP == nil {
		now := NowSecond()
		toP = &now
	}
	if fromP == nil {
		fromV := FirstBlock.Get().Timestamp.ToSecond()
		fromP = &fromV
	}
	return OneIntervalBuckets(*fromP, *toP), nil
}

func optionalIntParam(urlParams *url.Values, name string) (*int64, miderr.Err) {
	input := util.ConsumeUrlParam(urlParams, name)
	if input == "" {
		return nil, nil
	}
	i, err := strconv.ParseInt(input, 10, 64)
	if err != nil {
		return nil, miderr.BadRequestF(
			"Parameter '%s' is not integer: %s\n%s", name, input, usage)
	}
	return &i, nil
}

func optionalSecParam(urlParams *url.Values, name string) (*Second, miderr.Err) {
	intp, merr := optionalIntParam(urlParams, name)
	return (*Second)(intp), merr
}

func BucketsFromQuery(ctx context.Context, urlParams *url.Values) (Buckets, miderr.Err) {
	from, merr := optionalSecParam(urlParams, "from")
	if merr != nil {
		return Buckets{}, merr
	}
	to, merr := optionalSecParam(urlParams, "to")
	if merr != nil {
		return Buckets{}, merr
	}
	count, merr := optionalIntParam(urlParams, "count")
	if merr != nil {
		return Buckets{}, merr
	}

	intervalStr := util.ConsumeUrlParam(urlParams, "interval")
	if intervalStr == "" {
		return generateBucketsOnlyMeta(ctx, from, to, count)
	}
	interval, ok := intervalFromJSONParamMap[strings.ToLower(intervalStr)]
	if !ok {
		return Buckets{}, miderr.BadRequestF(
			"Invalid interval '(%s)', accepted values: 5min, hour, day, week, month, quarter, year.\n%s",
			intervalStr, usage)
	}

	return generateBucketsWithInterval(ctx, from, to, count, interval)
}

// Select field that truncates the value considering the buckets.Interval
// Result is date in seconds.
func SelectTruncatedTimestamp(targetColumn string, buckets Buckets) string {
	if buckets.OneInterval() {
		return fmt.Sprintf(`(%d)::BIGINT`, buckets.Start())
	} else {
		return fmt.Sprintf(
			`EXTRACT(EPOCH FROM (date_trunc('%s', to_timestamp(%s/1000000000/300*300))))::BIGINT`,
			intervalMap[*buckets.interval].dateTruncName, targetColumn)
	}
}

func (b Buckets) AggregateName() string {
	return intervalMap[*b.interval].name
}
