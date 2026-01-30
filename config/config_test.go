package config_test

import (
	"testing"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
)

func TestMustLoadConfigFile(t *testing.T) {
	testdb.HideTestLogs(t)

	var c config.Config
	config.MustLoadConfigFiles("config.json", &c)
	config.LogAndcheckUrls(&c)
}
