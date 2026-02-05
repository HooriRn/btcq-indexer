package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestReadGenesis_qbtc verifies that a qbtc-style genesis (e.g. tmp/genesis.json)
// unmarshals correctly. qbtc uses "initial_height" as a number; THORChain uses string.
func TestReadGenesis_qbtc(t *testing.T) {
	path := filepath.Join("..", "..", "tmp", "genesis.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("no tmp/genesis.json found: %v", err)
		}
		t.Fatal(err)
	}

	var g GenesisType
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatalf("unmarshal genesis: %v", err)
	}

	if g.ChainID == "" {
		t.Error("chain_id empty")
	}
	if g.InitialHeight == "" {
		t.Error("initial_height empty after unmarshal (qbtc uses number 1)")
	}
	if g.AppState.Bank.Balances == nil {
		t.Error("app_state.bank.balances nil")
	}
}
