// Depth recorder fills keeps track of historical depth values and inserts changes
// in the block_pool_depths table.
package timeseries

import (
	"fmt"
	"strings"
	"time"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
	"gitlab.com/thorchain/midgard/internal/util"
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

	for _, runePriceInUsd := range usdPools {
		if pool == runePriceInUsd {
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
	runeE8DepthSnapshot  mapDiff
	synthE8DepthSnapshot mapDiff
}

var depthRecorder depthManager

// Insert rows in the block_pool_depths for every changed value in the depth maps.
// If there is no change it doesn't write out anything.
// All values will be written out together (assetDepth, runeDepth, synthDepth), even if only one of the values
// changed in the pool.
func (sm *depthManager) update(
	timestamp time.Time, assetE8DepthPerPool, runeE8DepthPerPool, synthE8DepthPerPool map[string]int64,
	height int64) error {
	blockTimestamp := timestamp.UnixNano()
	// We need to iterate over all 2*n maps: {old,new}{Asset,Rune,Synth}.
	// First put all pool names into a set.
	poolNames := map[string]bool{}
	accumulatePoolNames := func(m map[string]int64) {
		for pool := range m {
			poolNames[pool] = true
		}
	}
	accumulatePoolNames(assetE8DepthPerPool)
	accumulatePoolNames(runeE8DepthPerPool)
	accumulatePoolNames(synthE8DepthPerPool)
	accumulatePoolNames(sm.assetE8DepthSnapshot.snapshot)
	accumulatePoolNames(sm.runeE8DepthSnapshot.snapshot)
	accumulatePoolNames(sm.synthE8DepthSnapshot.snapshot)

	cols := []string{"pool", "asset_e8", "rune_e8", "synth_e8", "block_timestamp"}

	var err error
	runePricesInUsd := []float64{}
	for pool := range poolNames {
		assetDiff, assetValue := sm.assetE8DepthSnapshot.diffAtKey(pool, assetE8DepthPerPool)
		runeDiff, runeValue := sm.runeE8DepthSnapshot.diffAtKey(pool, runeE8DepthPerPool)
		synthDiff, synthValue := sm.synthE8DepthSnapshot.diffAtKey(pool, synthE8DepthPerPool)
		if assetDiff || runeDiff || synthDiff {
			err = db.Inserter.Insert("block_pool_depths", cols, pool, assetValue, runeValue, synthValue, blockTimestamp)
			if err != nil {
				break
			}
		}

		// Add TOR anchors
		if isTORAnchor(pool, height) {
			usdPoolRatio := float64(assetE8DepthPerPool[pool]) / float64(runeE8DepthPerPool[pool])
			runePricesInUsd = append(runePricesInUsd, usdPoolRatio)
		}

	}
	sm.assetE8DepthSnapshot.save(assetE8DepthPerPool)
	sm.runeE8DepthSnapshot.save(runeE8DepthPerPool)
	sm.synthE8DepthSnapshot.save(synthE8DepthPerPool)

	// Calculate median of TOR anchors
	// if there is no available anchor pools just use the deepest pool
	var RunePriceInTOR float64
	if len(runePricesInUsd) > 0 {
		RunePriceInTOR = util.GetMedian(runePricesInUsd)
	} else {
		var maxDepth int64 = -1
		for _, pool := range config.Global.UsdPools {
			if maxDepth > assetE8DepthPerPool[pool] {
				RunePriceInTOR = float64(assetE8DepthPerPool[pool]) / float64(runeE8DepthPerPool[pool])
				maxDepth = assetE8DepthPerPool[pool]
			}
		}
	}

	err = db.Inserter.Insert("rune_price", []string{"rune_price_e8", "block_timestamp"}, RunePriceInTOR, blockTimestamp)
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
