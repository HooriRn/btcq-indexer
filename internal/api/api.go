// Package api provides the HTTP interface.
package api

import (
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/pascaldekloe/metrics"
	"github.com/rs/zerolog/hlog"

	"github.com/btcq/btcq-indexer/internal/decimal"
	"github.com/btcq/btcq-indexer/internal/timeseries/stat"
	"github.com/btcq/btcq-indexer/internal/util/btcqlog"
	"github.com/btcq/btcq-indexer/internal/util/timer"
)

// Handler serves the entire API.
var Handler http.Handler

func addMeasured(router *httprouter.Router, url string, handler httprouter.Handle) {
	reg, err := regexp.Compile("[^a-zA-Z0-9]+")
	if err != nil {
		panic("Bad constant url regex.")
	}
	simplifiedURL := reg.ReplaceAllString(url, "_")
	t := timer.NewTimer("serving" + simplifiedURL)

	router.Handle(
		http.MethodGet, url,
		func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
			m := t.One()
			handler(w, r, ps)
			m()
		})
}

// InitHandler inits API main handler
func InitHandler(nodeURL string) {
	router := httprouter.New()

	Handler = loggerHandler(corsHandler(router))

	// apply some navigation pointers
	router.HandleMethodNotAllowed = true
	router.HandleOPTIONS = true
	router.HandlerFunc(http.MethodGet, "/", serveRoot)

	// Debug endpoints
	router.HandlerFunc(http.MethodGet, "/v2/debug/metrics", metrics.ServeHTTP)
	router.HandlerFunc(http.MethodGet, "/v2/debug/timers", timer.ServeHTTP)
	router.HandlerFunc(http.MethodGet, "/v2/debug/usd", stat.ServeUSDDebug)
	router.HandlerFunc(http.MethodGet, "/v2/debug/decimals", decimal.ServeDecimalsDebug)
	router.Handle(http.MethodGet, "/v2/debug/block/:id", debugBlock)

	// Keep only health and pools endpoints
	addMeasured(router, "/v2/health", jsonHealth)
	addMeasured(router, "/v2/pools", jsonPools)

	router.PanicHandler = panicHandler
}

func panicHandler(w http.ResponseWriter, r *http.Request, err interface{}) {
	logger := btcqlog.LoggerForModule("http")
	zlog := logger.GetZeroLogger()
	zlog.Error().
		Interface("error", err).
		Str("path", r.URL.Path).
		Msg("panic http handler")
	w.WriteHeader(http.StatusInternalServerError)
}

func serveRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain;charset=UTF-8")

	// Discarding errors
	_, _ = io.WriteString(w, `# btcq-indexer

Welcome to the HTTP interface.
`)
}

func corsHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		h.ServeHTTP(w, r)
	})
}

func loggerHandler(h http.Handler) http.Handler {
	logger := btcqlog.LoggerForModule("http")

	// simillar to hlog.NewHandler
	setLoggerInContext := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Create a copy of the logger (including internal context slice)
			// to prevent data race when using UpdateContext.
			l := logger.GetZeroLogger().With().Logger()
			r = r.WithContext(l.WithContext(r.Context()))
			next.ServeHTTP(w, r)
		})
	}

	logSummaryAfter := hlog.AccessHandler(func(r *http.Request, status, size int, duration time.Duration) {
		hlog.FromRequest(r).Debug().
			Str("method", r.Method).
			Str("url", r.URL.String()).
			Int("status", status).
			Int("size", size).
			Dur("duration_ms", duration).
			Msg("Access")
	})

	remoteAddrHandler := hlog.RemoteAddrHandler("ip")
	userAgentHandler := hlog.UserAgentHandler("user_agent")
	refererHandler := hlog.RefererHandler("referer")
	requestIDHandler := hlog.RequestIDHandler("req_id", "X-Request-Id")

	return setLoggerInContext(
		logSummaryAfter(
			remoteAddrHandler(
				userAgentHandler(
					refererHandler(
						requestIDHandler(h))))))
}
