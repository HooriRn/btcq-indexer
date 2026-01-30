package timeseries_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func TestTHORNamesE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	setupLastBlock := int64(100)
	timeseries.SetLastHeightForTest(setupLastBlock)

	thor1 := "thor1xxxx"
	thor2 := "thor2xxxx"
	thor3 := "thor3xxxx"
	btc1 := "bc1xxxx"
	btc2 := "bc2xxxx"
	eth1 := "0x1xxxx"

	// setup a happy thorname
	blocks.NewBlock(t, "2000-01-01 00:00:00",
		testdb.THORName{
			Name:            "test1",
			Chain:           "THOR",
			Address:         thor1,
			Owner:           thor1,
			RegistrationFee: 10_00000000,
			FundAmount:      1_00000000,
			ExpireHeight:    123456,
		},
		testdb.THORName{
			Name:            "test1",
			Chain:           "BTC",
			Address:         btc1,
			Owner:           thor1,
			RegistrationFee: 0,
			FundAmount:      0,
			ExpireHeight:    123456,
		},
	)
	var lookup oapigen.THORNameDetailsResponse
	body := testdb.CallJSON(t, "http://localhost:8080/v2/thorname/lookup/test1")
	testdb.MustUnmarshal(t, body, &lookup)

	require.Equal(t, 2, len(lookup.Entries))
	require.Equal(t, thor1, lookup.Owner)
	require.Equal(t, "123456", lookup.Expire)
	require.Equal(t, "BTC", lookup.Entries[0].Chain)
	require.Equal(t, btc1, lookup.Entries[0].Address)
	require.Equal(t, "THOR", lookup.Entries[1].Chain)
	require.Equal(t, thor1, lookup.Entries[1].Address)

	// Test chainging ownership of happy thorname
	blocks.NewBlock(t, "2000-01-01 00:02:00",
		testdb.THORName{
			Name:            "test1",
			Chain:           "THOR",
			Address:         thor2,
			Owner:           thor2,
			RegistrationFee: 0,
			FundAmount:      1_00000000,
			ExpireHeight:    1234567,
		},
		testdb.THORName{
			Name:            "test1",
			Chain:           "BTC",
			Address:         btc2,
			Owner:           thor2,
			RegistrationFee: 0,
			FundAmount:      0,
			ExpireHeight:    1234567,
		},
	)
	body = testdb.CallJSON(t, "http://localhost:8080/v2/thorname/lookup/test1")
	testdb.MustUnmarshal(t, body, &lookup)

	require.Equal(t, 2, len(lookup.Entries))
	require.Equal(t, thor2, lookup.Owner)
	require.Equal(t, "1234567", lookup.Expire)
	require.Equal(t, "BTC", lookup.Entries[0].Chain)
	require.Equal(t, btc2, lookup.Entries[0].Address)
	require.Equal(t, "THOR", lookup.Entries[1].Chain)
	require.Equal(t, thor2, lookup.Entries[1].Address)

	// check that an expired thorname doesn't show up
	blocks.NewBlock(t, "2000-01-01 00:03:00",
		testdb.THORName{
			Name:            "expired",
			Chain:           "THOR",
			Address:         thor1,
			Owner:           thor1,
			RegistrationFee: 10_00000000,
			FundAmount:      1_00000000,
			ExpireHeight:    1,
		},
	)
	testdb.CallFail(t, "http://localhost:8080/v2/thorname/lookup/expired", "not found")

	// Test reverse lookup, but first create a thorname, where the user is no longer the owner
	blocks.NewBlock(t, "2000-01-01 00:04:00",
		testdb.THORName{
			Name:            "test2",
			Chain:           "THOR",
			Address:         thor2,
			Owner:           thor2,
			RegistrationFee: 0,
			FundAmount:      1_00000000,
			ExpireHeight:    4000,
		},
	)
	blocks.NewBlock(t, "2000-01-01 00:05:00",
		testdb.THORName{
			Name:            "test2",
			Chain:           "THOR",
			Address:         thor3,
			Owner:           thor3,
			RegistrationFee: 0,
			FundAmount:      1_00000000,
			ExpireHeight:    4000,
		},
	)
	blocks.NewBlock(t, "2000-01-01 00:06:00",
		testdb.THORName{
			Name:            "test3",
			Chain:           "THOR",
			Address:         thor2,
			Owner:           thor2,
			RegistrationFee: 0,
			FundAmount:      1_00000000,
			ExpireHeight:    4000,
		},
	)
	var rlookup oapigen.ReverseTHORNameResponse
	testdb.CallFail(t, "http://localhost:8080/v2/thorname/rlookup/"+thor1, "not found")
	testdb.CallFail(t, "http://localhost:8080/v2/thorname/rlookup/"+btc1, "not found")

	body = testdb.CallJSON(t, "http://localhost:8080/v2/thorname/rlookup/"+thor2)
	testdb.MustUnmarshal(t, body, &rlookup)
	require.Equal(t, 2, len(rlookup))
	require.Equal(t, "test1", rlookup[0])
	require.Equal(t, "test3", rlookup[1])

	//Test renewing the thorname and add new chain and also, changing btc address
	blocks.NewBlock(t, "2000-01-01 00:07:00",
		testdb.THORName{
			Name:            "test1",
			Chain:           "THOR",
			Address:         thor2,
			Owner:           thor2,
			RegistrationFee: 0,
			FundAmount:      1_00000000,
			ExpireHeight:    3,
		},
		testdb.THORName{
			Name:            "test1",
			Chain:           "BTC",
			Address:         btc1,
			Owner:           thor2,
			RegistrationFee: 0,
			FundAmount:      0,
			ExpireHeight:    3,
		},
	)

	blocks.NewBlock(t, "2000-01-01 00:08:00",
		testdb.THORName{
			Name:            "test1",
			Chain:           "ETH",
			Address:         eth1,
			Owner:           thor2,
			RegistrationFee: 0,
			FundAmount:      0,
			ExpireHeight:    3,
		},
	)

	testdb.CallFail(t, "http://localhost:8080/v2/thorname/lookup/test1", "not found")
	testdb.CallFail(t, "http://localhost:8080/v2/thorname/rlookup/"+eth1, "not found")

	blocks.NewBlock(t, "2000-01-01 00:09:00",
		testdb.THORName{
			Name:            "test1",
			Chain:           "THOR",
			Address:         thor2,
			Owner:           thor2,
			RegistrationFee: 0,
			FundAmount:      1_00000000,
			ExpireHeight:    1000000,
		},
	)

	body = testdb.CallJSON(t, "http://localhost:8080/v2/thorname/lookup/test1")
	testdb.MustUnmarshal(t, body, &lookup)

	require.Equal(t, 3, len(lookup.Entries))
	require.Equal(t, thor2, lookup.Owner)
	require.Equal(t, "1000000", lookup.Expire)
	require.Equal(t, "BTC", lookup.Entries[0].Chain)
	require.Equal(t, btc1, lookup.Entries[0].Address)
	require.Equal(t, "ETH", lookup.Entries[1].Chain)
	require.Equal(t, eth1, lookup.Entries[1].Address)
	require.Equal(t, "THOR", lookup.Entries[2].Chain)
	require.Equal(t, thor2, lookup.Entries[2].Address)
}

func TestTHORNamesCaseInsensitive(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	// setup a happy thorname
	blocks.NewBlock(t, "2000-01-01 00:00:00",
		testdb.THORName{
			Name:            "name1",
			Chain:           "ETH",
			Address:         "EthADDR1",
			Owner:           "ThorAddr1",
			RegistrationFee: 10_00000000,
			FundAmount:      1_00000000,
			ExpireHeight:    123456,
		},
		testdb.THORName{
			Name:            "name2",
			Chain:           "BTC",
			Address:         "BTCaddr2",
			Owner:           "ThorAddr2",
			RegistrationFee: 10_00000000,
			FundAmount:      1_00000000,
			ExpireHeight:    123456,
		},
	)

	var rlookup oapigen.ReverseTHORNameResponse

	{
		body := testdb.CallJSON(t, "http://localhost:8080/v2/thorname/rlookup/ethaddr1")
		testdb.MustUnmarshal(t, body, &rlookup)
		require.Equal(t, 1, len(rlookup))
		require.Equal(t, "name1", rlookup[0])
	}

	{
		body := testdb.CallJSON(t, "http://localhost:8080/v2/thorname/rlookup/EthaDDr1")
		testdb.MustUnmarshal(t, body, &rlookup)
		require.Equal(t, 1, len(rlookup))
		require.Equal(t, "name1", rlookup[0])
	}

	{
		body := testdb.CallJSON(t, "http://localhost:8080/v2/thorname/rlookup/BTCaddr2")
		testdb.MustUnmarshal(t, body, &rlookup)
		require.Equal(t, 1, len(rlookup))
		require.Equal(t, "name2", rlookup[0])
	}

	testdb.JSONFailGeneral(t, "http://localhost:8080/v2/thorname/rlookup/bTcAdDr2")

	{
		body := testdb.CallJSON(t, "http://localhost:8080/v2/thorname/lookup/name1")
		var lookup oapigen.THORNameDetailsResponse
		testdb.MustUnmarshal(t, body, &lookup)

		require.Equal(t, 1, len(lookup.Entries))
		require.Equal(t, "ThorAddr1", lookup.Owner)
	}
}

func TestTHORNamesOwner(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2000-01-01 00:00:00",
		testdb.THORName{
			Name:         "name1",
			Chain:        "THOR",
			Address:      "thorTarget",
			Owner:        "thorOwner",
			FundAmount:   1_00000000,
			ExpireHeight: 3,
		},
	)

	var rlookup oapigen.ReverseTHORNameResponse

	// rlookup by owner fails
	testdb.CallFail(t, "http://localhost:8080/v2/thorname/rlookup/thorOwner", "not found")

	body := testdb.CallJSON(t, "http://localhost:8080/v2/thorname/owner/thorOwner")
	testdb.MustUnmarshal(t, body, &rlookup)

	require.Equal(t, "name1", rlookup[0])

	// Add a few blocks, let it expire.
	blocks.NewBlock(t, "2000-01-01 00:00:01")
	blocks.NewBlock(t, "2000-01-01 00:00:02")
	blocks.NewBlock(t, "2000-01-01 00:00:03")
	blocks.NewBlock(t, "2000-01-01 00:00:04")

	testdb.CallFail(t, "http://localhost:8080/v2/thorname/owner/thorOwner", "not found")

	// Reenable ThorName
	blocks.NewBlock(t, "2000-01-01 00:00:05",
		testdb.THORName{
			Name:         "name1",
			Chain:        "THOR",
			Address:      "thorTarget",
			Owner:        "thorOwner",
			FundAmount:   1_00000000,
			ExpireHeight: 100,
		},
	)
	body = testdb.CallJSON(t, "http://localhost:8080/v2/thorname/owner/thorOwner")
	testdb.MustUnmarshal(t, body, &rlookup)
	require.Equal(t, "name1", rlookup[0])

	// Register a differ owner & overwrite the older owner
	blocks.NewBlock(t, "2000-01-01 00:00:06",
		testdb.THORName{
			Name:         "name1",
			Chain:        "THOR",
			Address:      "thorTarget",
			Owner:        "thorDifferentOwner",
			FundAmount:   1_00000000,
			ExpireHeight: 99,
		},
	)

	testdb.CallFail(t, "http://localhost:8080/v2/thorname/owner/thorOwner", "not found")

	body = testdb.CallJSON(t, "http://localhost:8080/v2/thorname/owner/thorDifferentOwner")
	testdb.MustUnmarshal(t, body, &rlookup)
	require.Equal(t, "name1", rlookup[0])

	// thorOnwer has two thornames on its behalf and renewing its THOR (Chain) name with a new address
	blocks.NewBlock(t, "2000-01-01 00:00:07",
		testdb.THORName{
			Name:         "name2",
			Chain:        "BTC",
			Address:      "btcTarget",
			Owner:        "thorOwner",
			FundAmount:   1_00000000,
			ExpireHeight: 99,
		},
		testdb.THORName{
			Name:         "name1",
			Chain:        "THOR",
			Address:      "thorOwner",
			Owner:        "thorOwner",
			FundAmount:   1_00000000,
			ExpireHeight: 100,
		},
		testdb.THORName{
			Name:         "name1",
			Chain:        "BTC",
			Address:      "btcTarget",
			Owner:        "thorOwner",
			FundAmount:   1_00000000,
			ExpireHeight: 101,
		},
	)

	body = testdb.CallJSON(t, "http://localhost:8080/v2/thorname/owner/thorOwner")
	testdb.MustUnmarshal(t, body, &rlookup)
	require.Equal(t, 2, len(rlookup))
	require.Equal(t, "name1", rlookup[0])
	require.Equal(t, "name2", rlookup[1])
}
