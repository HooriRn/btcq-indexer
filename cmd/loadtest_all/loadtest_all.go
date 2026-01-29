package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/btcq/btcq-indexer/internal/util/btcqlog"
)

var midgardURL = flag.String("midgard_url", "http://localhost:8080", "Base URL of Midgard to test")

const tries = 3 // Number of times to query each URL

type Endpoint struct {
	path   string
	params []string
}

const (
	days100    = "interval=day&count=100"
	exPool     = "pool=BNB.BNB"
	offset1000 = "offset=1000&limit=50"
)

var noParams = []string{}
var historyWithPool = []string{days100, exPool}
var history = []string{days100}

// All combination of url parameters are going to be tried (including all or no parameters)
var endpoints = []Endpoint{
	{"/v2/history/depths/BNB.BNB", []string{days100}},
	{"/v2/history/swaps", historyWithPool},
	{"/v2/history/earnings", history},
	{"/v2/history/liquidity_changes", historyWithPool},
	{"/v2/history/tvl", history},
	{"/v2/actions", []string{offset1000, "address=someaddr"}},
	{"/v2/pools", noParams},
	{"/v2/pool/BNB.BNB/stats", noParams},
	{"/v2/members", noParams},
	{"/v2/thorchain/inbound_addresses", noParams},
	{"/bad", noParams},
}

type measurement struct {
	ok    bool
	milli int
}

type stats struct {
	median float64
	avg    float64
	max    float64
}

func computeStats(ms []float64) (ret stats) {
	sort.Float64s(ms)
	ret.median = ms[len(ms)/2]
	ret.max = ms[len(ms)-1]
	for _, m := range ms {
		ret.avg += float64(m)
	}
	ret.avg /= float64(len(ms))

	// round to 3 digits
	ret.avg = float64(int(ret.avg*1000)) / 1000
	return
}

func fetchHTTP(url string) (err error) {
	resp, err := http.Get(url)
	if err != nil {
		return
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("status: %v", resp.Status)
	}
	defer resp.Body.Close()
	buf := new(strings.Builder)
	n, err := io.Copy(buf, resp.Body)
	if err != nil {
		return
	}
	if n < 10 {
		return fmt.Errorf("response too short")
	}
	return
}

func measureHTTP(url string) (result measurement) {
	result.ok = true
	start := time.Now()
	err := fetchHTTP(url)
	result.milli = int(time.Since(start).Milliseconds())
	if err != nil {
		result.ok = false
		btcqlog.DebugT(btcqlog.Tags(
			btcqlog.Err(err),
			btcqlog.Str("url", url),
			btcqlog.Int("time_ms", result.milli),
		),
			"Fetch failed")
	}
	return
}

func (ep *Endpoint) measureWithParams(params []string) {
	p := strings.Join(params, "&")
	url := *midgardURL + ep.path
	if len(p) != 0 {
		url += "?" + p
	}

	var measurements []float64
	for i := 0; i < tries; i++ {
		m := measureHTTP(url)
		if !m.ok {
			btcqlog.InfoT(btcqlog.Tags(
				btcqlog.Str("endpoint", ep.path),
				btcqlog.Str("params", p),
				btcqlog.Err(fmt.Errorf("unhealthy")),
			), ".")
			return
		}
		if 10000 < m.milli {
			btcqlog.InfoT(btcqlog.Tags(
				btcqlog.Str("endpoint", ep.path),
				btcqlog.Str("params", p),
				btcqlog.Float64("s", float64(m.milli)/1000),
				btcqlog.Err(fmt.Errorf("too slow")),
			), ".")
		}
		measurements = append(measurements, float64(m.milli)/1000)
	}
	stats := computeStats(measurements)
	btcqlog.InfoTF(btcqlog.Tags(
		btcqlog.Str("endpoint", ep.path),
		btcqlog.Str("params", p),
		btcqlog.Float64("s_median", stats.median),
		btcqlog.Float64("s_max", stats.max),
		btcqlog.Float64("s_avg", stats.avg),
	), "%.2f", stats.avg)
}

func allSubsets(parts []string, closure func([]string)) {
	var f func(int, []string)
	f = func(i int, subset []string) {
		if i < 0 {
			closure(subset)
			return
		}
		f(i-1, subset)
		f(i-1, append(subset, parts[i]))
	}
	f(len(parts)-1, []string{})
}

func (ep *Endpoint) measureAll() {
	allSubsets(ep.params, ep.measureWithParams)
}

func main() {
	flag.Parse()

	btcqlog.InfoT(btcqlog.Str("midgard_url", *midgardURL), "Starting")

	for _, ep := range endpoints {
		ep.measureAll()
	}
}
