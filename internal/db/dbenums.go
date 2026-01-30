package db

type SwapDirection int8

// Do not change these constantss. SQL Queries may assume this value dirrectly.
const (
	RuneToAsset   SwapDirection = 0
	AssetToRune   SwapDirection = 1
	RuneToSynth   SwapDirection = 2
	SynthToRune   SwapDirection = 3
	RuneToDerived SwapDirection = 4
	DerivedToRune SwapDirection = 5
	RuneToTrade   SwapDirection = 6
	TradeToRune   SwapDirection = 7
	RuneToSecure  SwapDirection = 8
	SecureToRune  SwapDirection = 9
)
