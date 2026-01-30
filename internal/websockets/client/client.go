package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"net"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"gitlab.com/thorchain/midgard/internal/websockets"
)

var (
	ip          = flag.String("ip", "127.0.0.1", "server IP")
	connections = flag.Int("conn", 1, "number of websocket connections")
	Logger      = websockets.NewLogger()
)

var disconnect = &websockets.Instruction{
	Message: "Disconnect",
	Assets:  []string{"BTC.BTC"},
}

var connect = &websockets.Instruction{
	Message: "Connect",
	Assets:  []string{"BTC.BTC"},
}

func disconnectConnection(conn net.Conn) {
	marshalled, err := json.Marshal(disconnect)
	if err != nil {
		Logger.Warnf("error marshalling %v", err)
	} else {
		err := wsutil.WriteClientMessage(conn, ws.OpText, marshalled)
		if err != nil {
			Logger.Warnf("error %v", err)
		}
	}

	conn.Close()
}

func main() {
	flag.Usage = func() {
		if _, err := io.WriteString(os.Stderr, `Websockets client generator
Example usage: ./client -ip=172.17.0.1 -conn=10
`); err != nil {
			Logger.Warnf("%v", err)
		}
		flag.PrintDefaults()
	}
	flag.Parse()

	u := url.URL{Scheme: "ws", Host: *ip + ":8080", Path: "/v2/websocket"}
	Logger.Infof("Connecting to %s", u.String())
	var conns []net.Conn
	tts := time.Second
	if *connections > 100 {
		tts = time.Millisecond * 5
	}

	ctx := context.Background()

	for i := 0; i < *connections; i++ {
		c, _, _, err := ws.Dial(ctx, u.String())
		if err != nil {
			Logger.Warn("Failed to connect", i, err)
			break
		}
		conns = append(conns, c)
	}

	if len(conns) == 0 {
		Logger.Info("no valid connections, get out of here")
		return
	}

	Logger.Infof("Finished initializing %d connections", len(conns))

	// Send the connect to assetPool message
	for i := 0; i < len(conns); i++ {
		time.Sleep(tts)
		conn := conns[i]
		marshalled, err := json.Marshal(connect)

		if err != nil {
			Logger.Warnf("error marshalling %v", err)
		} else {
			err := wsutil.WriteClientMessage(conn, ws.OpText, marshalled)
			if err != nil {
				Logger.Warnf("error %v", err)
			}
		}
	}

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())
	go read(ctx, conns)

	for {
		select {
		case <-signalChannel:
			cancel()
			Logger.Info("someone or something hit ctrl c, canceling input...")
			Logger.Info("sleep for 3 secs, have hit cancel, give the routines a few ticks to settle up, then quit input")
			for _, conn := range conns {
				// Disconnect everything.
				disconnectConnection(conn)
				time.Sleep(1 * time.Second)
			}
			time.Sleep(1 * time.Second)
			return
		}
	}
}

func read(ctx context.Context, conns []net.Conn) {
	finished := false

	go func() {
		select {
		case <-ctx.Done():
			Logger.Info("stop reading")
			finished = true
			return
		}
	}()

	for {
		if finished {
			return
		}
		for _, conn := range conns {
			msg, err := wsutil.ReadServerText(conn)
			if err != nil {
				Logger.Warnf("Error reading client data %v", err)
			}

			p := &websockets.Payload{}
			if err := json.Unmarshal(msg, p); err != nil {
				Logger.Warnf("error unmarshalling %v", err)
			}

			Logger.Infof("Symbol %s", p.Asset)
			Logger.Infof("Price %s", p.Price)
		}
	}
}
