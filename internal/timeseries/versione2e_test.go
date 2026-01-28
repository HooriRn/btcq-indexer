package timeseries_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/btcq/btcq-indexer/internal/db/testdb"
	"github.com/btcq/btcq-indexer/internal/timeseries"
)

func TestVersionE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-09-01 00:10:00",
		testdb.Version{
			Version: "1.107.0",
		},
	)

	version, err := timeseries.ActiveNetworkVersion(context.Background())
	require.NoError(t, err)
	require.Equal(t, "1.107.0", version)
}
