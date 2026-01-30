package stat_test

import (
	"context"
	"testing"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/internal/timeseries/stat"
)

func TestPoolAddsLookup(t *testing.T) {
	testdb.SetupTestDB(t)
	_, err := stat.PoolAddsLookup(
		context.Background(), "BNB.MATIC-416", db.Window{})
	if err != nil {
		t.Fatal(err)
	}
	// TODO(acsaba): add a events to the database and check that we get at least one value.
}

func TestPoolErratasLookup(t *testing.T) {
	testdb.SetupTestDB(t)
	_, err := stat.PoolErratasLookup(context.Background(), "BNB.BNB", db.Window{})
	if err != nil {
		t.Fatal(err)
	}
	// TODO(acsaba): add a events to the database and check that we get at least one value.
}

func TestPoolFeesLookup(t *testing.T) {
	testdb.SetupTestDB(t)
	_, err := stat.PoolFeesLookup(context.Background(), "BNB.BNB", db.Window{})
	if err != nil {
		t.Fatal(err)
	}
	// TODO(acsaba): add a events to the database and check that we get at least one value.
}

func TestPoolGasLookup(t *testing.T) {
	testdb.SetupTestDB(t)
	_, err := stat.PoolGasLookup(
		context.Background(), "BNB.MATIC-416", db.Window{})
	if err != nil {
		t.Fatal(err)
	}
	// TODO(acsaba): add a events to the database and check that we get at least one value.
}

func TestPoolSlashesLookup(t *testing.T) {
	testdb.SetupTestDB(t)
	_, err := stat.PoolSlashesLookup(context.Background(), "BNB.MATIC-416", db.Window{})
	if err != nil {
		t.Fatal(err)
	}
	// TODO(acsaba): add a events to the database and check that we get at least one value.
}
