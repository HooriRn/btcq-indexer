//go:build linux
// +build linux

package websockets_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/internal/util/jobs"
	"gitlab.com/thorchain/midgard/internal/websockets"
)

type devNull struct{}

func (devNull) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func init() {
	websockets.Logger.SetOutput(devNull{})
}

func recieveSome(t *testing.T, count int) []websockets.Payload {
	ret := []websockets.Payload{}
	for i := 0; i < count; i++ {
		select {
		case payload := <-*websockets.TestChannel:
			ret = append(ret, payload)
		case <-time.After(1000 * time.Millisecond):
			require.Fail(t, "didn't get websoket reply")
		}
	}
	return ret
}

func initWebsocketTest(t *testing.T) {
	channel := make(chan websockets.Payload, 100)
	websockets.TestChannel = &channel
	db.CreateWebsocketChannel()
}

func BlockingWebsockets(t *testing.T) func(ctx context.Context) {
	return func(ctx context.Context) {
		pendingJob, err := websockets.Init(ctx, 10)
		require.Nil(t, err)
		job := pendingJob.Start()
		job.MustWait()
	}
}

func TestWebsockets(t *testing.T) {
	initWebsocketTest(t)

	blocks := testdb.InitTestBlocks(t)
	blocks.NewBlock(t, "2000-01-01 00:00:00",
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 10, RuneAmount: 20},
		testdb.PoolActivate("BTC.BTC"))

	job := jobs.StartForTests(BlockingWebsockets(t))
	defer job.Quit()

	*db.WebsocketNotify <- struct{}{}

	response := recieveSome(t, 1)
	require.Equal(t, "BTC.BTC", response[0].Asset)
	require.Equal(t, "2", response[0].Price)

	blocks.NewBlock(t, "2000-01-01 00:00:01",
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 30, RuneAmount: 0})

	*db.WebsocketNotify <- struct{}{}

	response = recieveSome(t, 1)
	require.Equal(t, "BTC.BTC", response[0].Asset)
	require.Equal(t, "0.5", response[0].Price)
}

func TestWebsocketTwoPools(t *testing.T) {
	initWebsocketTest(t)

	blocks := testdb.InitTestBlocks(t)
	blocks.NewBlock(t, "2000-01-01 00:00:00",
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 10, RuneAmount: 20},
		testdb.PoolActivate("BTC.BTC"),
		testdb.AddLiquidity{Pool: "ETH.ETH", AssetAmount: 10, RuneAmount: 100},
		testdb.PoolActivate("ETH.ETH"),
	)

	job := jobs.StartForTests(BlockingWebsockets(t))
	defer job.Quit()

	*db.WebsocketNotify <- struct{}{}

	response := recieveSome(t, 2)
	require.Contains(t, response, websockets.Payload{"2", "BTC.BTC"})
	require.Contains(t, response, websockets.Payload{"10", "ETH.ETH"})
}
