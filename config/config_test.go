package config_test

import (
	"testing"

	"github.com/btcq/btcq-indexer/config"
	"github.com/btcq/btcq-indexer/internal/db/testdb"
)

func TestMustLoadConfigFile(t *testing.T) {
	testdb.HideTestLogs(t)

	var c config.Config
	config.MustLoadConfigFiles("config.json", &c)
	config.LogAndcheckUrls(&c)
}
