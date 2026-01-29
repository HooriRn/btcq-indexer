// Tool for dumping to a json structure the blocks received from ThorNode.
//
// The Output path is configured with the "block_store_folder" configuration parameter
// Each output file contains exactly blocksPerChunk number of block events (block chunk)
// sent from ThorNode
// Partially fetched block chunks are stored in a temporary file.
//
// Each block chunk file is named after the last contained block height (padded with zeros to 12 width)
//
// The tool is restartable, and will resume the dump from the last successfully fetched block
// chunk (unfinished block chunks are discarded)
package main

import (
	"time"

	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/fetch/sync/blockstore"
	"github.com/btcq/btcq-indexer/internal/fetch/sync/chain"
	"github.com/btcq/btcq-indexer/internal/util/jobs"
	"github.com/btcq/btcq-indexer/internal/util/btcqlog"
)

func main() {
	btcqlog.LogCommandLine()
	config.ReadGlobal()

	mainContext := jobs.InitSignals()

	btcqlog.InfoF("BlockStore: local directory: %s", config.Global.BlockStore.Local)

	chainClient, err := chain.NewClient(mainContext)
	if err != nil {
		btcqlog.FatalE(err, "Error during chain client initialization")
	}

	status, err := chainClient.RefreshStatus()
	if err != nil {
		btcqlog.FatalE(err, "Error during fetching chain status")
	}

	db.InitializeChainVarsFromThorNodeStatus(status)

	forkHeight := db.CurrentChain.Get().HardForkHeight

	blockStore := blockstore.NewBlockStore(
		mainContext,
		config.Global.BlockStore,
		db.RootChain.Get().Name)

	// BlockStore creation may take some time to copy remote blockstore to local.
	// If it was cancelled, we don't create anything else.
	jobs.StopIfCanceled()

	startHeight := blockStore.LastFetchedHeight() + 1
	if startHeight < status.SyncInfo.EarliestBlockHeight {
		btcqlog.FatalF(
			"Cannot continue dump, startHeight[%d] < status.SyncInfo.EarliestBlockHeight[%d]",
			startHeight, status.SyncInfo.EarliestBlockHeight)
	}
	endHeight := status.SyncInfo.LatestBlockHeight
	if config.Global.BlockStore.DownloadFullChunksOnly {
		if forkHeight != 0 && forkHeight <= endHeight {
			endHeight = forkHeight
		} else {
			endHeight = endHeight - endHeight%config.Global.BlockStore.BlocksPerChunk
		}
		if endHeight < startHeight {
			btcqlog.Info("No new full chunks, exiting")
			return
		}
	}

	it := chainClient.Iterator(startHeight, endHeight)

	btcqlog.InfoF("BlockStore: start fetching from %d to %d", startHeight, endHeight)

	finishedNormally := false

	currentHeight := startHeight
	blockStoreJob := jobs.Start("BlockStore", func() {
		defer blockStore.Close()
		for {
			if mainContext.Err() != nil {
				btcqlog.InfoF("BlockStore: write shutdown")
				return
			}
			block, err := it.Next()
			if err != nil {
				btcqlog.WarnF("BlockStore: error while fetching at height %d : %v", currentHeight, err)
				db.SleepWithContext(mainContext, 7*time.Second)
				it = chainClient.Iterator(currentHeight, endHeight)
				continue
			}
			if block == nil {
				btcqlog.Info("BlockStore: Reached ThorNode last block")
				jobs.InitiateShutdown()
				finishedNormally = true
				return
			}
			if block.Height != currentHeight {
				btcqlog.ErrorEF(
					err,
					"BlockStore: height not incremented by one. Expected: %d Actual: %d",
					currentHeight, block.Height)
				return
			}

			forceFinalizeChunk := forkHeight != 0 && block.Height == forkHeight
			blockStore.DumpBlock(block, forceFinalizeChunk)

			if forceFinalizeChunk {
				btcqlog.Info("BlockStore: Reached fork height")
				jobs.InitiateShutdown()
				finishedNormally = true
				return
			}

			if currentHeight%1000 == 0 {
				percentGlobal := 100 * float64(block.Height) / float64(endHeight)
				percentCurrentRun := 100 * float64(block.Height-startHeight) / float64(endHeight-startHeight)
				btcqlog.InfoF(
					"BlockStore: fetched block with height %d [%.2f%% ; %.2f%%]",
					block.Height, percentGlobal, percentCurrentRun)
			}
			currentHeight++
		}
	})

	jobs.WaitUntilSignal()

	jobs.ShutdownWait(&blockStoreJob)

	if !finishedNormally {
		jobs.LogSignalAndStop()
	}

}
