// This tool shows various views and queries used by the aggregate mechanisms.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/dbinit"
	_ "gitlab.com/thorchain/midgard/internal/timeseries/stat"
	"gitlab.com/thorchain/midgard/internal/util"
)

const usageString = `Print SQL definitions for a given aggregate
Usage:
$ go run ./cmd/aggregates [flags] config aggregateName
`

var startTime = flag.String("from", "2021-05-03 14:03:05", "Starting time used for example queries")
var endTime = flag.String("to", "2021-07-02 09:57:45", "End time used for example queries")
var interval = flag.String("interval", "day", "Interval used for example queries")

func init() {
	flag.Usage = func() {
		fmt.Print(usageString)
		flag.PrintDefaults()
		fmt.Println("\nDefined aggregates:")
		for _, aggregate := range db.AggregateList() {
			fmt.Println(aggregate)
		}
	}
}

func parseTime(s string) string {
	const format = "2006-01-02 15:04:05"
	t, err := time.Parse(format, s)
	if err != nil {
		log.Panic().Err(err).Msg("Failed to parse timestamp")
	}
	return util.IntStr(t.Unix())
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	flag.Parse()

	if flag.NArg() != 2 {
		fmt.Print("Wrong number of arguments\n\n")
		flag.Usage()
		return
	}

	aggregate := db.GetRegisteredAggregateByName(flag.Arg(1))
	if aggregate == nil {
		fmt.Printf("Unknown aggregate: %s\n\n", flag.Arg(1))
		flag.Usage()
		return
	}

	// We use TimescaleDB to generate buckets in general, so we need a DB connection.
	// This is the only reason we are asking for a config.
	config.ReadGlobalFrom(flag.Arg(0))
	dbinit.Setup()

	// We need to set this to some sensible value, so buckets are not truncated
	db.LastCommittedBlock.Set(1, db.TimeToNano(time.Now()))

	fmt.Print("--\n-- Materialized and plain VIEWs defined in the `midgard_agg` schema:\n--\n")
	aggregate.CreateViews(os.Stdout)

	fmt.Print("--\n-- Basic bucketed query\n--\n")
	urlParams := make(url.Values)
	urlParams.Add("from", parseTime(*startTime))
	urlParams.Add("to", parseTime(*endTime))
	buckets, err := db.BucketsFromQuery(context.Background(), &urlParams)
	if err != nil {
		fmt.Printf("Failed to create buckets: %v\n", err)
		return
	}
	q, params := aggregate.BucketedQuery("%s", buckets, nil, nil)
	fmt.Println(q)
	fmt.Printf("-- params: %v\n", params)

	fmt.Print("\n--\n-- Bucketed query with an interval\n--\n")
	urlParams = make(url.Values)
	urlParams.Add("from", parseTime(*startTime))
	urlParams.Add("to", parseTime(*endTime))
	urlParams.Add("interval", *interval)
	buckets, err = db.BucketsFromQuery(context.Background(), &urlParams)
	if err != nil {
		fmt.Printf("Failed to create buckets: %v\n", err)
		return
	}
	q, params = aggregate.BucketedQuery("%s", buckets, nil, nil)
	fmt.Println(q)
	fmt.Printf("-- params: %v\n", params)
}
