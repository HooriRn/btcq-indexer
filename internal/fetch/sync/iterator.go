package sync

import (
	"reflect"

	"gitlab.com/thorchain/midgard/internal/fetch/sync/blockstore"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/chain"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
)

type Iterator struct {
	s           *Sync
	height      int64
	finalHeight int64
	chainIt     *chain.Iterator
	bStoreIt    *blockstore.Iterator
}

func NewIterator(s *Sync, startHeight, finalHeight int64) Iterator {
	if CheckBlockStoreBlocks {
		return newIteratorChecked(s, startHeight, finalHeight)
	}

	var bStoreIt *blockstore.Iterator = nil
	var cIt *chain.Iterator = nil
	if startHeight <= s.blockStore.LastFetchedHeight() {
		obj := s.blockStore.Iterator(startHeight)
		bStoreIt = &obj
	} else {
		obj := s.chainClient.Iterator(startHeight, finalHeight)
		cIt = &obj
	}

	ret := Iterator{
		s:           s,
		height:      startHeight,
		finalHeight: finalHeight,
		chainIt:     cIt,
		bStoreIt:    bStoreIt,
	}
	return ret
}

func (i *Iterator) Next() (*chain.Block, error) {
	if CheckBlockStoreBlocks {
		return i.nextChecked()
	}
	if i.finalHeight < i.height {
		return nil, nil
	}

	if i.bStoreIt != nil {
		ret, err := i.bStoreIt.Next()
		if err == nil && ret != nil {
			if ret.Height != i.height {
				return nil, miderr.InternalErrF(
					"BlockStore height not incremented by one. Actual: %d Expected: %d", ret.Height, i.height)
			}
			i.height++
			return ret, nil
		} else {
			if err != nil {
				logger.ErrorE(err, "Blockstore error, switching over to chain")
			} else {
				logger.Info("Reached blockstore end, switching over to chain")
			}
			i.bStoreIt = nil
			cIt := i.s.chainClient.Iterator(i.height, i.finalHeight)
			i.chainIt = &cIt
		}
	}

	if i.chainIt == nil {
		return nil, miderr.InternalErr("Programming error no iterator present")
	}
	ret, err := i.chainIt.Next()
	if err != nil {
		return nil, err
	}
	if ret != nil {
		if ret.Height != i.height {
			return nil, miderr.InternalErrF(
				"Chain height not incremented by one. Actual: %d Expected: %d", ret.Height, i.height)
		}
		i.height++
		return ret, nil
	}
	return nil, miderr.InternalErr("Programming error, chain returned no result and no error")
}

func newIteratorChecked(s *Sync, startHeight, finalHeight int64) Iterator {
	logger.Info("Debug mode, syncing will be slow. Reading blocks both from BlockStore and Chain ")

	cIt := s.chainClient.Iterator(startHeight, finalHeight)
	bStoreIt := s.blockStore.Iterator(startHeight)
	ret := Iterator{
		s:           s,
		height:      startHeight,
		finalHeight: finalHeight,
		chainIt:     &cIt,
		bStoreIt:    &bStoreIt,
	}
	return ret
}

// Useful for developers to check blockstore.
// Very strict, gives an error if there is a difference between blockstore and chain and stops.
func (i *Iterator) nextChecked() (*chain.Block, error) {
	if i.finalHeight < i.height {
		return nil, nil
	}

	var err error
	var bsBlock *chain.Block
	if i.bStoreIt != nil {
		bsBlock, err = i.bStoreIt.Next()
		if err != nil {
			return nil, err
		}
		if bsBlock == nil {
			logger.Info("Reached blockstore end, reading only chain from now on")
			i.bStoreIt = nil
		}
	}

	if i.chainIt == nil {
		return nil, miderr.InternalErr("Programming error no iterator present")
	}
	chainBlock, err := i.chainIt.Next()
	if err != nil {
		return nil, err
	}

	if bsBlock != nil {
		if reflect.DeepEqual(bsBlock, chainBlock) {
			return nil, miderr.InternalErrF(
				"Blockstore blocks blocks don't match chain blocks at height %d", i.height)
		}
	}

	if chainBlock != nil {
		if chainBlock.Height != i.height {
			return nil, miderr.InternalErrF(
				"Chain height not incremented by one. Actual: %d Expected: %d", chainBlock.Height, i.height)
		}
		i.height++
		return chainBlock, nil
	}
	return nil, miderr.InternalErr("Programming error, chain returned no result and no error")
}

func (i *Iterator) FetchingFrom() string {
	if i.bStoreIt != nil {
		return "blockstore"
	}
	return "chain"
}
