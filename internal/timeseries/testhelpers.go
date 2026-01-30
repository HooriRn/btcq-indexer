// Exports functionality for tests only.
// Because the filename has _test, it won't be included in production builds.

package timeseries

import (
	"time"

	"gitlab.com/thorchain/midgard/internal/db"
)

func copyMap(m map[string]int64) map[string]int64 {
	m2 := map[string]int64{}
	for i, v := range m {
		m2[i] = v
	}
	return m2
}

func copyOfLastTrack() (ret *blockTrack) {
	height := int64(1)
	t := time.Unix(0, 0)
	hash := []byte("hash0")
	assetDepth := map[string]int64{}
	runeDepth := map[string]int64{}
	synthDepth := map[string]int64{}
	interfacePtr := lastBlockTrack.Load()
	if interfacePtr != nil {
		oldTrack := interfacePtr.(*blockTrack)
		height = oldTrack.Height
		t = oldTrack.Timestamp
		hash = oldTrack.Hash
		assetDepth = copyMap(oldTrack.aggTrack.AssetE8DepthPerPool)
		runeDepth = copyMap(oldTrack.aggTrack.RuneE8DepthPerPool)
		synthDepth = copyMap(oldTrack.aggTrack.SynthE8DepthPerPool)
	}
	return &blockTrack{
		Height:    height,
		Timestamp: t,
		Hash:      hash,
		aggTrack: aggTrack{
			AssetE8DepthPerPool: assetDepth,
			RuneE8DepthPerPool:  runeDepth,
			SynthE8DepthPerPool: synthDepth,
		},
	}
}

// Often current height or timestamp is read from the last track, this function helps
// to set them for tests.
func SetLastTimeForTest(timestamp db.Second) {
	trackPtr := copyOfLastTrack()
	trackPtr.Timestamp = timestamp.ToTime()
	setLastBlock(trackPtr)
}

func SetLastHeightForTest(height int64) {
	trackPtr := copyOfLastTrack()
	trackPtr.Height = height
	setLastBlock(trackPtr)
}

type Depth struct {
	Pool       string
	AssetDepth int64
	RuneDepth  int64
	SynthDepth int64
}

func SetDepthsForTest(depths []Depth) {
	resetAggTrack()
	trackPtr := copyOfLastTrack()
	for _, depth := range depths {
		trackPtr.aggTrack.AssetE8DepthPerPool[depth.Pool] = depth.AssetDepth
		trackPtr.aggTrack.RuneE8DepthPerPool[depth.Pool] = depth.RuneDepth
		trackPtr.aggTrack.SynthE8DepthPerPool[depth.Pool] = depth.SynthDepth
	}
	setLastBlock(trackPtr)
}

func resetAggTrack() {
	trackPtr := copyOfLastTrack()
	trackPtr.aggTrack = aggTrack{
		AssetE8DepthPerPool: make(map[string]int64),
		RuneE8DepthPerPool:  make(map[string]int64),
		SynthE8DepthPerPool: make(map[string]int64),
	}
	setLastBlock(trackPtr)
}
