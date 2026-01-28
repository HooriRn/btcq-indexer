package timer

import (
	"time"

	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/util/midlog"
)

type milliCounter time.Time

func MilliCounter() milliCounter {
	return milliCounter(time.Now())
}

func (m milliCounter) SecondsElapsed() float32 {
	return float32(time.Since(time.Time(m)).Milliseconds()) / 1000
}

// Useful for debugging, prints running times to the console.
// When called with defer use (note the trailing "()"):
// defer timer.Console("name")()
func Console(name string) func() {
	start := MilliCounter()
	midlog.WarnT(midlog.Str("name", name), "Timer start")
	return func() {
		midlog.WarnT(midlog.Tags(
			midlog.Str("name", name),
			midlog.Float32("duration", start.SecondsElapsed()),
		), "Timer end")
	}
}

func DebugConsole(name string) func() {
	ShowLogs := config.Global.Debug.EnableAggregationTimer
	if !ShowLogs {
		return func() {}
	}

	return Console(name)
}
