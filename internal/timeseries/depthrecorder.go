// Depth recorder fills keeps track of historical depth values and inserts changes
// in the block_pool_depths table.
package timeseries

import (
	"fmt"
	"strings"
	"time"

	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/fetch/record"
	"github.com/btcq/btcq-indexer/internal/util"
)

// MapDiff helps to get differences between snapshots of a map.
type mapDiff struct {
	snapshot map[string]int64
}

// Save the map as the new snapshot.
func (md *mapDiff) save(newMap map[string]int64) {
	md.snapshot = map[string]int64{}
	for k, v := range newMap {
		md.snapshot[k] = v
	}
}

// Check if there is a chage for this pool.
func (md *mapDiff) diffAtKey(pool string, newMap map[string]int64) (hasDiff bool, newValue int64) {
	oldV, hasOld := md.snapshot[pool]
	newV, hasNew := newMap[pool]
	if hasNew {
		return !hasOld || oldV != newV, newV
	} else {
		return hasOld, 0
	}
}

func isTORAnchor(pool string, height int64) bool {
	// ignore synth and derived pools
	assetPool := util.AssetFromString(pool)
	if assetPool.Synth || assetPool.Chain == "THOR" {
		return false
	}

	usdPools := []string{}
	rootChainId := db.RootChain.Get().Name
	// check TORANCHOR mimir if available
	// TODO (HooriRn): add stagenet block height
	if height > 9967469 && rootChainId == "thorchain" {
		mimirTORKey := fmt.Sprintf("TORANCHOR-%s", strings.ToUpper(strings.ReplaceAll(pool, ".", "-")))
		if record.Recorder.CurrentMimirStatus(mimirTORKey) == 1 {
			usdPools = append(usdPools, pool)
		}
	} else {
		usdPools = config.Global.UsdPools
	}

	for _, qbtcPriceInUsd := range usdPools {
		if pool == qbtcPriceInUsd {
			poolStatus := record.Recorder.CurrentPoolStatus(pool)
			if poolStatus != "available" {
				continue
			}

			chainHalted := record.Recorder.CurrentMimirStatus(
				fmt.Sprintf("HALT%sCHAIN", assetPool.Chain))
			tradingHalted := record.Recorder.CurrentMimirStatus(
				fmt.Sprintf("HALT%sTRADING", assetPool.Chain))
			if chainHalted == 1 || tradingHalted == 1 {
				continue
			}

			// new mimir changes
			newChainHalted := record.Recorder.CurrentMimirStatus(
				fmt.Sprintf("10-%s", assetPool.Chain))
			newTradingHalted := record.Recorder.CurrentMimirStatus(
				fmt.Sprintf("11-%s", assetPool.Chain))
			if newChainHalted == 1 || newTradingHalted == 1 {
				continue
			}

			return true
		}
	}

	return false
}

type depthManager struct {
	assetE8DepthSnapshot mapDiff
	qbtcE8DepthSnapshot  mapDiff
	synthE8DepthSnapshot mapDiff
}

var depthRecorder depthManager

// Insert rows in the block_pool_depths for every changed value in the depth maps.
// If there is no change it doesn't write out anything.
// All values will be written out together (assetDepth, qbtcDepth, synthDepth), even if only one of the values
// changed in the pool.
func (sm *depthManager) update(
	timestamp time.Time, assetE8DepthPerPool, qbtcE8DepthPerPool, synthE8DepthPerPool map[string]int64,
	height int64) error {
	blockTimestamp := timestamp.UnixNano()
	// We need to iterate over all 2*n maps: {old,new}{Asset,Qbtc,Synth}.
	// First put all pool names into a set.
	poolNames := map[string]bool{}
	accumulatePoolNames := func(m map[string]int64) {
		for pool := range m {
			poolNames[pool] = true
		}
	}
	accumulatePoolNames(assetE8DepthPerPool)
	accumulatePoolNames(qbtcE8DepthPerPool)
	accumulatePoolNames(synthE8DepthPerPool)
	accumulatePoolNames(sm.assetE8DepthSnapshot.snapshot)
	accumulatePoolNames(sm.qbtcE8DepthSnapshot.snapshot)
	accumulatePoolNames(sm.synthE8DepthSnapshot.snapshot)

	cols := []string{"pool", "asset_e8", "qbtc_e8", "synth_e8", "block_timestamp"}

	var err error
	qbtcPricesInUsd := []float64{}
	for pool := range poolNames {
		assetDiff, assetValue := sm.assetE8DepthSnapshot.diffAtKey(pool, assetE8DepthPerPool)
		qbtcDiff, qbtcValue := sm.qbtcE8DepthSnapshot.diffAtKey(pool, qbtcE8DepthPerPool)
		synthDiff, synthValue := sm.synthE8DepthSnapshot.diffAtKey(pool, synthE8DepthPerPool)
		if assetDiff || qbtcDiff || synthDiff {
			err = db.Inserter.Insert("block_pool_depths", cols, pool, assetValue, qbtcValue, synthValue, blockTimestamp)
			if err != nil {
				break
			}
		}

		// Add TOR anchors
		if isTORAnchor(pool, height) {
			usdPoolRatio := float64(assetE8DepthPerPool[pool]) / float64(qbtcE8DepthPerPool[pool])
			qbtcPricesInUsd = append(qbtcPricesInUsd, usdPoolRatio)
		}

	}
	sm.assetE8DepthSnapshot.save(assetE8DepthPerPool)
	sm.qbtcE8DepthSnapshot.save(qbtcE8DepthPerPool)
	sm.synthE8DepthSnapshot.save(synthE8DepthPerPool)

	// Calculate median of TOR anchors
	// if there is no available anchor pools just use the deepest pool
	var QbtcPriceInTOR float64
	if len(qbtcPricesInUsd) > 0 {
		QbtcPriceInTOR = util.GetMedian(qbtcPricesInUsd)
	} else {
		var maxDepth int64 = -1
		for _, pool := range config.Global.UsdPools {
			if maxDepth > assetE8DepthPerPool[pool] {
				QbtcPriceInTOR = float64(assetE8DepthPerPool[pool]) / float64(qbtcE8DepthPerPool[pool])
				maxDepth = assetE8DepthPerPool[pool]
			}
		}
	}

	err = db.Inserter.Insert("qbtc_price", []string{"qbtc_price_e8", "block_timestamp"}, QbtcPriceInTOR, blockTimestamp)
	if err != nil {
		return err
	}

	if err != nil {
		return fmt.Errorf("error saving depths (timestamp: %d): %w", blockTimestamp, err)
	}

	return nil
}

func ResetDepthManagerForTest() {
	depthRecorder = depthManager{}
}
