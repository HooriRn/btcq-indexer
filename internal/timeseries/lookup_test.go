package timeseries

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSynthUnitsBigNum(t *testing.T) {
	synthUnits := CalculateSynthUnits(1878459169, 1909971564, 35168185551)
	require.Equal(t, int64(36368256684), synthUnits)
}

func TestGetSinglePoolSynthUnits(t *testing.T) {
	// 10% of the pool is owned by synth holders
	synthUnits := CalculateSynthUnits(10000, 2000, 90)
	require.Equal(t, int64(10), synthUnits)
}

func TestNextChurnHeight(t *testing.T) {
	require.Equal(t, int64(1100), calculateNextChurnHeight(1042, 1000, 100, 10))
	require.Equal(t, int64(1110), calculateNextChurnHeight(1100, 1000, 100, 10))
	require.Equal(t, int64(1120), calculateNextChurnHeight(1112, 1000, 100, 10))
	require.Equal(t, int64(1120), calculateNextChurnHeight(1113, 1000, 100, 10))
	require.Equal(t, int64(1120), calculateNextChurnHeight(1119, 1000, 100, 10))
	require.Equal(t, int64(1130), calculateNextChurnHeight(1120, 1000, 100, 10))
}
