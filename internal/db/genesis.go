package db

import (
	"encoding/json"
	"os"
	"strconv"
	"time"

	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/util"
	"github.com/btcq/btcq-indexer/internal/util/btcqlog"
)

var (
	GenesisInfo GenesisInfoType
	GenesisData GenesisType
)

// Genesis Type - Maybe it should be separate file?
type GenesisInfoType struct {
	Height int64
	Hash   string
}

func (s GenesisInfoType) set(height int64, hash string) {
	GenesisInfo = GenesisInfoType{
		Height: height,
		Hash:   hash,
	}
}

func (s GenesisInfoType) Get() GenesisInfoType {
	if !ConfigHasGenesis() {
		return GenesisInfoType{}
	}
	return GenesisInfo
}

type JsonMap map[string]interface{}

// This genesis type is custom made from THORNode:
// https://gitlab.com/thorchain/thornode/-/blob/95ece18f92e363381aa0d09a9df779b4d63318f5/x/thorchain/genesis.pb.go#L132
// initial_height can be string (THORChain export) or number (e.g. qbtc genesis).
type GenesisType struct {
	GenesisTime   time.Time     `json:"genesis_time"`
	ChainID       string        `json:"chain_id"`
	InitialHeight flexibleHeight `json:"initial_height"`
	AppState      AppState      `json:"app_state,omitempty"`
}

// flexibleHeight unmarshals from JSON string or number (e.g. "1" or 1).
type flexibleHeight string

func (h *flexibleHeight) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*h = flexibleHeight(s)
		return nil
	}
	var n int64
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*h = flexibleHeight(strconv.FormatInt(n, 10))
	return nil
}

type AppState struct {
	Auth JsonMap `json:"auth"`
	Bank Bank    `json:"bank"`
}

type Bank struct {
	Balances []Balance `json:"balances"`
	Supplies []Coin    `json:"supply"`
}

type Balance struct {
	Address string `json:"address"`
	Coins   []Coin `json:"coins"`
}

type Coin struct {
	Amount int64  `json:"amount,string"`
	Denom  string `json:"denom"`
}

func GenesisExits() bool {
	return readDbConstant("genesis") != nil
}

func (g *GenesisType) GetGenesisHeight() (height int64) {
	return config.Global.Genesis.InitialBlockHeight
}

func DestroyGenesis() {
	GenesisData = GenesisType{}
}

func ConfigHasGenesis() bool {
	return config.Global.Genesis.Local != ""
}

func ReadDBGenesisHeight() int64 {
	value := readDbConstant("genesis")
	if value == nil {
		return 0
	}
	return util.MustParseInt64(string(value))
}

// TODO(HooriRn): add lp units sanity check
func InitGenesis() {
	if !ConfigHasGenesis() {
		return
	}

	genesisFile, err := os.ReadFile(config.Global.Genesis.Local)
	if err != nil {
		btcqlog.Fatal("Can't read genesis file!")
	}

	var genesisData GenesisType
	err = json.Unmarshal(genesisFile, &genesisData)
	if err != nil {
		btcqlog.ErrorE(err, "Can't unmarshal genesis file! seems the file is not right.")
	}

	genesisChainId := genesisData.ChainID
	genesisRootId := GetRootFromChainIdName(genesisChainId)
	rootFromThorNodeSatusChainId := RootChain.Get().Name
	if rootFromThorNodeSatusChainId != genesisRootId {
		btcqlog.WarnF("Genesis file chain id mismatch: root chain: %s, genesis: %s",
			RootChain.Get().Name, genesisChainId)
	}

	height := genesisData.GetGenesisHeight()
	if height <= 0 {
		btcqlog.Fatal("Genesis block height should be more than zero.")
	}

	dbHeight := ReadDBGenesisHeight()
	if dbHeight > 0 && dbHeight != height {
		btcqlog.Fatal("The DB current genesis height is not the same as the file, Please nukedb first.")
	}

	if config.Global.Genesis.InitialBlockHash == "" {
		btcqlog.Fatal("There is no hash in genesis config! Please add genesis block hash to config.")
	}
	GenesisInfo.set(height, config.Global.Genesis.InitialBlockHash)

	GenesisData = genesisData
}
