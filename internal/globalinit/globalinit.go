// The list of packages which have side effects
// In order to create a new runnable it should be enough to import this package to have
// all the init functions called.
package globalinit

import (
	_ "github.com/btcq/btcq-indexer/internal/timeseries"
	_ "github.com/btcq/btcq-indexer/internal/timeseries/stat"
)
