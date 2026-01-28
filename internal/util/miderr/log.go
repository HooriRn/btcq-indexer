package miderr

import "github.com/btcq/btcq-indexer/internal/util/midlog"

func LogEventParseErrorF(format string, v ...interface{}) {
	midlog.WarnF(format, v...)
}
