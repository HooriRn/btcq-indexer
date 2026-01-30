package record

const ChainIDStagenet2024 = "thorchain-stagenet-2"

func loadStagenetCorrections(rootChainID string) {
	if rootChainID == ChainIDStagenet2024 {
		undelayedLiquidityFeesHeight = 3359140
	}
}
