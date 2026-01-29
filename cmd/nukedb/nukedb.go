package main

import (
	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/util/btcqlog"

	_ "github.com/btcq/btcq-indexer/internal/globalinit"
)

func main() {
	btcqlog.LogCommandLine()
	config.ReadGlobal()

	db.SetupWithoutUpdate()

	btcqlog.Warn("Destroying database by removing the ddl hash")
	_, err := db.TheDB.Exec(`DELETE FROM constants WHERE key = 'ddl_hash'`)
	if err != nil {
		btcqlog.FatalE(err, "Failed to delete ddl hash.")
	}
	btcqlog.Info("Done. Next midgard run will reload the DB schema.")
}
