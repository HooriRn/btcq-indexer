package btcqerr

import "github.com/btcq/btcq-indexer/internal/util/btcqlog"

func LogEventParseErrorF(format string, v ...interface{}) {
	btcqlog.WarnF(format, v...)
}
