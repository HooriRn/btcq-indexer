package main

import (
	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/db"
	"github.com/btcq/btcq-indexer/internal/util/midlog"

	_ "github.com/btcq/btcq-indexer/internal/globalinit"
)

func main() {
	midlog.LogCommandLine()
	config.ReadGlobal()

	db.SetupWithoutUpdate()

	midlog.Warn("Destroying database by removing the ddl hash")
	_, err := db.TheDB.Exec(`DELETE FROM constants WHERE key = 'ddl_hash'`)
	if err != nil {
		midlog.FatalE(err, "Failed to delete ddl hash.")
	}
	midlog.Info("Done. Next midgard run will reload the DB schema.")
}
