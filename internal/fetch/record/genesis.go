package record

import (
	"strconv"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

func LoadGenesis() {
	if !db.ConfigHasGenesis() {
		return
	}

	defer db.DestroyGenesis()

	genesisTime := db.GenesisData.GenesisTime
	genesisBlockHeight := db.GenesisData.GetGenesisHeight()
	recordBlockHeight := genesisBlockHeight - 1

	lastCommitted := db.LastCommittedBlock.Get().Height

	if db.GenesisExits() {
		midlog.Info("The Genesis is already written.")
		// This means block log was empty and first block of genesis hasn't been fetched.
		if lastCommitted == 0 {
			midlog.Info("The Genesis is written but there was no block fetch.")
			db.LastCommittedBlock.Set(recordBlockHeight, db.TimeToNano(db.GenesisData.GenesisTime.Add(-1)))
		}
		return
	}

	if genesisBlockHeight < lastCommitted {
		midlog.Info("The Genesis was already written in a previous run.")
		return
	}

	// Initialize metadata
	var m = Metadata{
		BlockHeight:    recordBlockHeight,
		BlockTimestamp: genesisTime,
		EventId: db.EventId{
			BlockHeight: recordBlockHeight,
		},
	}

	BeginBlockEventsTotal.Add(uint64(0))
	m.EventId.Location = db.BeginBlockEvents
	m.EventId.EventIndex = 1

	err := db.Inserter.StartBlock()
	if err != nil {
		midlog.FatalE(err, "Failed to StartBlock")
		return
	}

	// Add Genesis KV to the records
	recordGenPools(&m)
	recordGenSupplies(&m)
	recordGenTransfers(&m)
	recordGenLPs(&m)
	recordGenTHORNames(&m)
	recordGenNodes(&m)
	recordGenLoans(&m)
	recordGenMimirs(&m)

	// Set genesis constant to db
	setGenesisConstant(genesisBlockHeight)

	err = db.Inserter.EndBlock()
	if err != nil {
		midlog.FatalE(err, "Failed to EndBlock")
		deleteDBIfFail()
		return
	}

	err = db.Inserter.Flush()
	if err != nil {
		midlog.FatalE(err, "Failed to Flush")
		deleteDBIfFail()
		return
	}

	db.LastCommittedBlock.Set(recordBlockHeight, db.TimeToNano(db.GenesisData.GenesisTime.Add(-1)))
}

func setGenesisConstant(genesisBlockHeight int64) {
	_, err := db.TheDB.Exec(`INSERT INTO constants (key, value) VALUES ('genesis', $1)
			ON CONFLICT (key) DO UPDATE SET value = $1`, []byte(strconv.FormatInt(genesisBlockHeight, 10)))
	if err != nil {
		midlog.FatalE(err, "Failed to Insert into constants")
		return
	}
}

func deleteDBIfFail() {
	_, err := db.TheDB.Exec(`DELETE FROM constants WHERE key = 'ddl_hash'`)
	if err != nil {
		midlog.FatalE(err, "Failed to delete ddl hash.")
		return
	}
}
