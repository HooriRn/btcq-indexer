package main

import (
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util/midlog"

	_ "gitlab.com/thorchain/midgard/internal/globalinit"
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
