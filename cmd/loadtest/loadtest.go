package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/dbinit"
)

const (
	Mode    ModeEnum = ModeHTTPFetch
	N                = 1000
	Threads          = 7
)

const URL = "http://localhost:8080/v2/history/depths/ETH.ETH?interval=day"

// const URL = "http://localhost:8080/v2/history/swaps?interval=day"
// const URL = "http://localhost:8080/v2/history/liquidity_changes?interval=day"
// const URL = "http://localhost:8080/v2/history/tvl?interval=day"
// const URL = "http://localhost:8080/v2/actions"
// const URL = "http://localhost:8080/v2/pools"
// const URL = "http://localhost:8080/v2/pool/BTC.BTC"
// const URL = "http://localhost:8080/v2/pool/BTC.BTC/stats"
// const URL = "http://localhost:8080/v2/members"
// const URL = "http://localhost:8080/v2/member/thor10jhw68ctam2vu4htxp06cyadu2jscpz02ukg38"
// const URL = "http://localhost:8080/v2/member/bnb13plxuczc6fvvnd48hahlfnd87zldd7k40hl8e5"

type ModeEnum int

const (
	ModeHTTPFetch ModeEnum = iota
	ModeDBFetch
)

type SingleSummary struct {
	id    int
	ok    bool
	milli int
}

func Measure() func() (milli int) {
	start := time.Now()
	return func() int {
		return int(time.Since(start).Milliseconds())
	}
}

func measureHTTP(result *SingleSummary) {
	result.ok = false
	timer := Measure()
	resp, err := http.Get(URL)
	if err != nil {
		log.Debug().Str("mode", "http").Int("req", result.id).Err(err).Msg("Failed to get")
		return
	}
	if resp.StatusCode != 200 {
		log.Debug().Str("mode", "http").Int("req", result.id).Str("status", resp.Status).Msg("Returned an error")
		return
	}
	defer resp.Body.Close()
	buf := new(strings.Builder)
	n, err := io.Copy(buf, resp.Body)
	if err != nil || n < 10 {
		log.Debug().Str("mode", "http").Int("req", result.id).Err(err).Msg("Failed to read")
		return
	}
	result.milli = timer()
	result.ok = true
	log.Debug().Str("mode", "http").Int("req", result.id).Int("time_ms", result.milli).Msg("OK")
}

const (
	ColumnNumber = 4
	Query        = `
SELECT
        pool,
        last(asset_e8, block_timestamp) as asset_e8,
        last(rune_e8, block_timestamp) as rune_e8,
        EXTRACT(EPOCH FROM (date_trunc('day', to_timestamp(block_timestamp/1000000000/300*300))))::BIGINT AS truncated
FROM block_pool_depths
WHERE pool = ANY($1) AND $2 <= block_timestamp AND block_timestamp < $3
GROUP BY truncated, pool
ORDER BY truncated ASC;`
)

var QArgs = []interface{}{
	[]string{"ETH.ETH", "BNB.BUSD-BAF BNB.USDT-DC8", "ETH.USDT-0X62E273709DA575835C7F6AEF4A31140CA5B1D190"},
	db.Nano(1618012800000000000),
	db.Nano(1619654400000000000),
}

func measureDB(result *SingleSummary) {
	ctx := context.Background()
	result.ok = false

	timer := Measure()
	lastHeightRows, err := db.Query(ctx, Query, QArgs...)
	if err != nil {
		log.Debug().Str("mode", "db").Int("req", result.id).Err(err).Msg("Failed query")
		return
	}
	defer lastHeightRows.Close()

	data := make([]string, ColumnNumber)
	results := []interface{}{}
	for i := range data {
		results = append(results, &data[i])
	}
	if lastHeightRows.Next() {
		err := lastHeightRows.Scan(results...)
		if err != nil {
			log.Debug().Str("mode", "db").Int("req", result.id).Err(err).Msg("Failed read")
			return
		}
	}
	result.milli = timer()
	result.ok = true
	log.Debug().Str("mode", "db").Int("req", result.id).Int("time_ms", result.milli).Msg("OK")
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	config.ReadGlobal()

	var measureFunc func(*SingleSummary)
	switch Mode {
	case ModeHTTPFetch:
		log.Debug().Str("URL", URL).Int("times", N).Int("threads", Threads).Msg(
			"HTTP loadtest: ")
		measureFunc = measureHTTP
	case ModeDBFetch:
		measureFunc = measureDB
		dbinit.Setup()
	}

	var summaries []SingleSummary
	for i := 0; i < N; i++ {
		summaries = append(summaries, SingleSummary{id: i})
	}

	done := make(chan struct{}, Threads)
	for i := 0; i < Threads; i++ {
		done <- struct{}{}
	}
	for i := range summaries {
		ilocal := i
		<-done
		go func() {
			measureFunc(&summaries[ilocal])
			done <- struct{}{}
		}()
	}
	for i := 0; i < Threads; i++ {
		<-done
	}

	var sum float64
	failNum := 0
	for _, v := range summaries {
		if v.ok {
			sum += float64(v.milli)
		} else {
			failNum++
		}
	}
	success := 100 * float64(failNum) / float64(N)
	avg := sum / float64(N)
	qps := Threads * 1000 / avg
	log.Info().Msgf(
		"Failures: %.2f%%, Average response time: %.2f milli, Average qps: %.2f",
		success, avg, qps)
}
