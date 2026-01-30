/*
Compares some blocks stored in the blockstore against the blocks fetched from a remote thornode
*/
package main

import (
	"context"
	"fmt"
	"math/rand"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/fetch/sync"
)

func main() {
	config.ReadGlobal()

	mainContext, mainCancel := context.WithCancel(context.Background())

	sync.CheckBlockStoreBlocks = true
	sync.InitGlobalSync(mainContext)

	for i := 0; i < 100; i++ {
		height := rand.Int63n(sync.GlobalSync.BlockStoreHeight())
		_, err := sync.GlobalSync.FetchSingle(height)
		fmt.Printf("Checking height %d\n", height)
		if err != nil {
			fmt.Printf("Error at height %d\n", height)
			mainCancel()
			panic("mismatch")
		}
	}
}
