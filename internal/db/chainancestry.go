package db

import (
	"sync/atomic"
	"unsafe"

	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/midgard/config"
)

var wellKnownChainInfos = []config.ForkInfo{
	// Mainnet
	{
		ChainId:             "thorchain",
		EarliestBlockHash:   "7D37DEF6E1BE23C912092069325C4A51E66B9EF7DDBDE004FF730CFABC0307B1",
		EarliestBlockHeight: 1,
		HardForkHeight:      4786559,
	},
	{
		ChainId:             "thorchain-mainnet-v1",
		ParentChainId:       "thorchain",
		EarliestBlockHash:   "9B86543A5CF5E26E3CE93C8349B2EABE5E238DFFC9EBE8EC6207FE7178FF27AC",
		EarliestBlockHeight: 4786560,
		HardForkHeight:      17562000,
	},
	{
		ChainId:             "thorchain-1",
		ParentChainId:       "thorchain-mainnet-v1",
		EarliestBlockHash:   "0B3C8F9E3EA7E9B1C10CAC5217ED771E0540671EFB9C5315BF01167266BCBEDF",
		EarliestBlockHeight: 17562001,
	},

	// Stagenet
	{
		ChainId:             "thorchain-stagenet-2",
		EarliestBlockHash:   "6F872F4DBF9D52AAE14F2296941C3A90F07D312634EC192AABAF02643047B82E",
		EarliestBlockHeight: 12501,
	},

	// Devnet
	{
		ChainId:             "dev-1",
		EarliestBlockHash:   "9E8E7219CD54CB46386C1DEF6C7D25C25DE990CB0C140F9B705DE2DB231BA795",
		EarliestBlockHeight: 1,
	},
}

var mergedChainMap unsafe.Pointer

func CombinedForkInfoMap() *map[string]config.ForkInfo {
	merged := (*map[string]config.ForkInfo)(atomic.LoadPointer(&mergedChainMap))
	if merged != nil {
		return merged
	}

	m := make(map[string]config.ForkInfo)
	infos := []config.ForkInfo{}
	infos = append(infos, wellKnownChainInfos...)
	infos = append(infos, config.Global.ThorChain.ForkInfos...)

	for _, fi := range infos {
		if fi.ParentChainId != "" {
			parent, ok := m[fi.ParentChainId]
			if !ok {
				log.Fatal().Msgf("Chain '%s' has parent '%s', but it's not defined",
					fi.ChainId, fi.ParentChainId)
			}
			if parent.HardForkHeight == 0 {
				log.Fatal().Msgf("Chain '%s' is a parent of '%s', but has no HardForkHeight defined",
					fi.ParentChainId, fi.ChainId)
			}
			if fi.EarliestBlockHeight == 0 {
				fi.EarliestBlockHeight = parent.HardForkHeight + 1
			}
			if fi.EarliestBlockHeight != parent.HardForkHeight+1 {
				log.Fatal().Msgf("Height discontinuity: %s ends at %d, %s starts at %d",
					fi.ParentChainId, parent.HardForkHeight, fi.ChainId, fi.EarliestBlockHeight)
			}
		} else {
			if fi.EarliestBlockHeight == 0 {
				fi.EarliestBlockHeight = 1
			}
		}
		if fi.HardForkHeight != 0 && fi.HardForkHeight < fi.EarliestBlockHeight {
			log.Fatal().Msgf(
				"Invalid ForkInfo for '%s': HardForkHeight[=%d] < EarliestBlockHeight[=%d]",
				fi.ChainId, fi.HardForkHeight, fi.EarliestBlockHeight)
		}

		m[fi.ChainId] = fi
	}

	atomic.StorePointer(&mergedChainMap, unsafe.Pointer(&m))
	return &m
}

func mergeAdditionalInfo(chainId *FullyQualifiedChainId, info config.ForkInfo) {
	if info.EarliestBlockHash != "" {
		chainId.StartHash = info.EarliestBlockHash
	}
	if info.EarliestBlockHeight != 0 {
		chainId.StartHeight = info.EarliestBlockHeight
	}
	if info.HardForkHeight != 0 {
		chainId.HardForkHeight = info.HardForkHeight
	}
}

func recursiveFindRoot(target config.ForkInfo) config.ForkInfo {
	m := *CombinedForkInfoMap()
	for {
		parent, ok := m[target.ParentChainId]
		if !ok {
			break
		}
		target = parent
	}
	return target
}

func EnrichAndGetRoot(chainId *FullyQualifiedChainId) FullyQualifiedChainId {
	m := *CombinedForkInfoMap()
	target, ok := m[chainId.Name]
	if !ok {
		if chainId.StartHeight != 1 {
			log.Fatal().Msgf(
				`Chain '%s' does not start at 1, yet it doesn't have a ForkInfo definition
				If you intend to start a truncated chain, add a ForkInfo definition for it without
				specifying a parent`, chainId.Name)
		}
		return *chainId
	}

	mergeAdditionalInfo(chainId, target)
	if chainId.HardForkHeight != 0 && chainId.HardForkHeight < chainId.StartHeight {
		log.Fatal().Msgf(
			"Merging in ForkInfo resulted in invalid data for chain '%s': HardForkHeight[=%d] < StartHeight[=%d]",
			chainId.Name, chainId.HardForkHeight, chainId.StartHeight)
	}

	target = recursiveFindRoot(target)

	if target.ChainId == chainId.Name {
		return *chainId
	}
	return FullyQualifiedChainId{
		Name:           target.ChainId,
		StartHash:      target.EarliestBlockHash,
		StartHeight:    target.EarliestBlockHeight,
		HardForkHeight: target.HardForkHeight,
	}
}

func GetRootFromChainIdName(chainIdName string) string {
	m := *CombinedForkInfoMap()
	target, ok := m[chainIdName]
	if !ok {
		log.Warn().Msg("There is no chain for this chainId. Might be the root chain itself.")
		return chainIdName
	}

	target = recursiveFindRoot(target)

	return target.ChainId
}
