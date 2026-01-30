package stat_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

const BALANCE_URL = "http://localhost:8080/v2/balance/"

type expectation struct {
	slug string
	json string
}

func TestUnknownKey(t *testing.T) {
	config.Global.EventRecorder.OnTransferEnabled = true

	testdb.CallFail(t, BALANCE_URL+"testAddr1?badkey=123", "badkey")
}

func TestBadTsE2E(t *testing.T) {
	config.Global.EventRecorder.OnTransferEnabled = true

	testdb.CallFail(t, BALANCE_URL+"thorAddr1?timestamp=xxx", "error parsing timestamp xxx")
}

func TestBadHeightE2E(t *testing.T) {
	config.Global.EventRecorder.OnTransferEnabled = true

	testdb.CallFail(t, BALANCE_URL+"thorAddr1?height=xxx", "error parsing height xxx")
}

func TestTooManyParamsE2E(t *testing.T) {
	config.Global.EventRecorder.OnTransferEnabled = true

	testdb.CallFail(t, BALANCE_URL+"thorAddr1?height=1&timestamp="+ts("2022-01-01 00:00:00"), "only one of height or timestamp can be specified, not both")
}

func TestNoDataE2E(t *testing.T) {
	config.Global.EventRecorder.OnTransferEnabled = true

	blocks := testdb.InitTestBlocks(t)

	// writing the first block with height=1 will fully initialize the fakechain
	blocks.NewBlock(t, "2022-01-01 00:00:00",
		testdb.Transfer{
			FromAddr:    "thorAddr3",
			ToAddr:      "thorAddr4",
			AssetAmount: "1 THOR.RUNE",
		},
	)

	testdb.CallFail(t, BALANCE_URL+"thorAddr1?height=0", "no data for height 0, height range is [1,1]")
	testdb.CallFail(t, BALANCE_URL+"thorAddr1?height=2", "no data for height 2, height range is [1,1]")
	testdb.CallFail(t, BALANCE_URL+"thorAddr1?timestamp="+ts("2021-12-31 23:59:59"), "no data for timestamp 1640995199, timestamp range is [1640995200,1640995200]")
	testdb.CallFail(t, BALANCE_URL+"thorAddr1?timestamp="+ts("2022-01-01 00:00:01"), "no data for timestamp 1640995201, timestamp range is [1640995200,1640995200]")
}

func TestZeroBalanceE2E(t *testing.T) {
	config.Global.EventRecorder.OnTransferEnabled = true

	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2022-01-01 00:00:00",
		testdb.Transfer{
			FromAddr:    "thorAddr1",
			ToAddr:      "thorAddr2",
			AssetAmount: "1 THOR.RUNE",
		},
	)

	blocks.NewBlock(t, "2022-01-01 00:00:01",
		testdb.Transfer{
			FromAddr:    "thorAddr2",
			ToAddr:      "thorAddr1",
			AssetAmount: "1 THOR.RUNE",
		},
	)

	checkExpected(t, []expectation{
		{"thorAddr1", `{"height": "2", "date": "1640995201000000000", "coins": [{"amount":"0","asset":"THOR.RUNE"}]}`},
		{"thorAddr2", `{"height": "2", "date": "1640995201000000000", "coins": [{"amount":"0","asset":"THOR.RUNE"}]}`},
	})
}

func TestBalanceE2E(t *testing.T) {
	config.Global.EventRecorder.OnTransferEnabled = true

	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2022-01-01 00:00:01",
		testdb.Transfer{
			FromAddr:    "thorAddr1",
			ToAddr:      "thorAddr2",
			AssetAmount: "1 THOR.RUNE",
		},
	)
	blocks.NewBlock(t, "2022-01-01 00:00:02",
		testdb.Transfer{
			FromAddr:    "thorAddr1",
			ToAddr:      "thorAddr2",
			AssetAmount: "20 THOR.RUNE",
		},
	)
	blocks.NewBlock(t, "2022-01-01 00:00:03",
		testdb.Transfer{
			FromAddr:    "thorAddr2",
			ToAddr:      "thorAddr1",
			AssetAmount: "3 THOR.RUNE",
		},
	)
	blocks.NewBlock(t, "2022-01-01 00:00:04",
		testdb.Transfer{
			FromAddr:    "thorAddr2",
			ToAddr:      "thorAddr1",
			AssetAmount: "300 BTC/BTC",
		},
	)

	checkExpected(t, []expectation{
		{"thorAddr0",
			`{
				"height": "4",
				"date": "1640995204000000000",
				"coins": []
			}`,
		},
		{"thorAddr1",
			`{
				"height": "4",
				"date": "1640995204000000000",
				"coins": [
					{"amount":"300", "asset":"BTC/BTC"},
					{"amount":"-18", "asset":"THOR.RUNE"}
				]
			}`,
		},
		{"thorAddr1?height=3",
			`{
				"height": "3",
				"date": "1640995203000000000",
				"coins": [
					{"amount":"-18", "asset":"THOR.RUNE"}
				]
			}`,
		},
		{"thorAddr1?height=4",
			`{
				"height": "4",
				"date": "1640995204000000000",
				"coins": [
					{"amount":"300", "asset":"BTC/BTC"},
					{"amount":"-18", "asset":"THOR.RUNE"}
				]
			}`,
		},
		{"thorAddr1?timestamp=" + ts("2022-01-01 00:00:01"),
			`{
				"height": "1",
				"date": "1640995201000000000",
				"coins": [
					{"amount":"-1", "asset":"THOR.RUNE"}
				]
			}`,
		},
		{"thorAddr1?timestamp=" + ts("2022-01-01 00:00:04"),
			`{
				"height": "4",
				"date": "1640995204000000000",
				"coins": [
					{"amount":"300", "asset":"BTC/BTC"},
					{"amount":"-18", "asset":"THOR.RUNE"}
				]
			}`,
		},
	})

}

func checkExpected(t *testing.T, expectations []expectation) {
	for i, e := range expectations {
		actualJson := testdb.CallJSON(t, BALANCE_URL+e.slug)
		var expectedBalance oapigen.BalanceResponse
		testdb.MustUnmarshal(t, []byte(e.json), &expectedBalance)
		var actualBalance oapigen.BalanceResponse
		testdb.MustUnmarshal(t, actualJson, &actualBalance)
		require.Equal(t, expectedBalance, actualBalance, fmt.Sprintf("expectation %d failed", i))
	}
}

func ts(date string) string {
	return fmt.Sprintf("%d", db.StrToSec(date))
}
