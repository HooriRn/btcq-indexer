package main

import (
	"time"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/blockstore"
	"gitlab.com/thorchain/midgard/internal/util/jobs"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

func main() {
	midlog.LogCommandLine()
	config.ReadGlobal()

	mainContext := jobs.InitSignals()

	midlog.InfoF("BlockStore: local directory: %s", config.Global.BlockStore.Local)

	fragmentClient, err := NewClient(mainContext)
	if err != nil {
		midlog.FatalE(err, "Error during chain client initialization")
	}

	status, err := fragmentClient.RefreshStatus()
	if err != nil {
		midlog.FatalE(err, "Error during fetching chain status")
	}

	db.InitializeChainVarsFromThorNodeStatus(status)

	blockStore := blockstore.NewBlockStore(
		mainContext,
		config.Global.BlockStore,
		db.RootChain.Get().Name)

	// BlockStore creation may take some time to copy remote blockstore to local.
	// If it was cancelled, we don't create anything else.
	jobs.StopIfCanceled()

	startHeight, err := blockStore.FistFetchedHeight()
	if err != nil {
		midlog.FatalE(err, "Error during getting the first height from blockstore")
	}
	var endHeight int64 = blockStore.LastFetchedHeight()

	itb := blockStore.Iterator(startHeight)
	itc := fragmentClient.Iterator(startHeight, endHeight)

	midlog.InfoF("BlockStore: start fragmenting from %d to %d", startHeight, endHeight)

	finishedNormally := false

	currentHeight := startHeight
	fragmentJob := jobs.Start("Fragment", func() {
		defer blockStore.Close()
		for {
			if mainContext.Err() != nil {
				midlog.InfoF("BlockStore: write shutdown")
				return
			}
			bBlock, err := itb.Next()
			if err != nil {
				midlog.WarnF("BlockStore: error while opening at height %d : %v", currentHeight, err)
				return
			}
			if bBlock == nil {
				midlog.Info("BlockStore: Reached Blockstore last block")
				jobs.InitiateShutdown()
				finishedNormally = true
				return
			}
			if bBlock.PureBlock != nil {
				currentHeight++
				itc = fragmentClient.Iterator(currentHeight, endHeight)
				if currentHeight%1000 == 0 {
					percentGlobal := 100 * float64(currentHeight) / float64(endHeight)
					midlog.InfoF(
						"BlockStore: block %d is already filled [%.2f%%]",
						currentHeight, percentGlobal)
				}
				continue
			}

			cBlock, err := itc.Next()
			if err != nil {
				midlog.WarnF("BlockStore: error while fetching at height %d : %v", currentHeight, err)
				db.SleepWithContext(mainContext, 7*time.Second)
				itc = fragmentClient.Iterator(currentHeight, endHeight)
				itb = blockStore.Iterator(currentHeight)
				continue
			}
			if bBlock.Height != cBlock.Height {
				midlog.ErrorEF(
					err,
					"BlockStore: height not incremented by one. Expected (Blockstore): %d Actual (Thornode): %d",
					bBlock.Height, cBlock.Height)
				return
			}
			if cBlock == nil && bBlock != nil {
				midlog.Error("BlockStore: Reached ThorNode last block while blockstore")
				return
			}

			// fill the missing data
			block := bBlock
			block.PureBlock = cBlock.PureBlock

			blockStore.DumpBlock(block, false)

			if currentHeight%1000 == 0 {
				percentGlobal := 100 * float64(block.Height) / float64(endHeight)
				midlog.InfoF(
					"BlockStore: filled block with height %d [%.2f%%]",
					currentHeight, percentGlobal)
			}
			currentHeight++
		}
	})

	jobs.WaitUntilSignal()

	jobs.ShutdownWait(&fragmentJob)

	if !finishedNormally {
		jobs.LogSignalAndStop()
	}
}
