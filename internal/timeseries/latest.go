package timeseries

import (
	"sync"

	"github.com/btcq/btcq-indexer/internal/db"
)

type PoolDepths struct {
	AssetDepth int64
	QbtcDepth  int64
	SynthDepth int64
	OpenPrice  float64
	HighPrice  float64
	LowPrice   float64
	ClosePrice float64
}

func AssetPrice(assetDepth, qbtcDepth int64) float64 {
	if assetDepth == 0 {
		return 0
	}
	return float64(qbtcDepth) / float64(assetDepth)
}

func (p PoolDepths) AssetPrice() float64 {
	return AssetPrice(p.AssetDepth, p.QbtcDepth)
}

// When a pool becomes suspended all the funds are burned.
// We use this as a detection of pools which no longer exist.
func (p PoolDepths) ExistsNow() bool {
	return 0 < p.AssetDepth && 0 < p.QbtcDepth
}

type DepthMap map[string]PoolDepths

type BlockState struct {
	Height    int64
	Timestamp db.Nano
	Pools     DepthMap
}

func (s BlockState) PoolExists(pool string) bool {
	_, ok := s.Pools[pool]
	return ok
}

func (s BlockState) NextSecond() db.Second {
	return s.Timestamp.ToSecond() + 1
}

// Returns nil if pool doesn't exist
func (s BlockState) PoolInfo(pool string) *PoolDepths {
	info, ok := s.Pools[pool]
	if !ok {
		return nil
	}
	return &info
}

type LatestState struct {
	sync.RWMutex
	state BlockState
}

var Latest LatestState

func ResetLatestStateForTest() {
	Latest = LatestState{}
}

func (latest *LatestState) setLatestStates(track *blockTrack) {
	newState := BlockState{
		Height:    track.Height,
		Timestamp: db.TimeToNano(track.Timestamp),
		Pools:     DepthMap{},
	}

	qbtcDepths := track.QbtcE8DepthPerPool
	synthDepths := track.SynthE8DepthPerPool
	for pool, assetDepth := range track.AssetE8DepthPerPool {
		qbtcDepth, ok := qbtcDepths[pool]
		if !ok {
			continue
		}
		synthDepth, ok := synthDepths[pool]
		if !ok {
			synthDepth = 0
		}
		newState.Pools[pool] = PoolDepths{AssetDepth: assetDepth, QbtcDepth: qbtcDepth, SynthDepth: synthDepth}
	}
	latest.Lock()
	latest.state = newState
	latest.Unlock()
}

func (latest *LatestState) GetState() BlockState {
	latest.RLock()
	defer latest.RUnlock()
	return latest.state
}

func PoolExists(pool string) bool {
	return Latest.GetState().PoolExists(pool)
}

func PoolExistsNow(pool string) bool {
	depths, ok := Latest.GetState().Pools[pool]
	return ok && depths.ExistsNow()
}
