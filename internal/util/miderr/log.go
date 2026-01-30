package miderr

import "gitlab.com/thorchain/midgard/internal/util/midlog"

func LogEventParseErrorF(format string, v ...interface{}) {
	midlog.WarnF(format, v...)
}
