package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"

	"github.com/DataDog/zstd"
	tmjson "github.com/tendermint/tendermint/libs/json"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/blockstore"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/chain"
)

func main() {
	if len(os.Args) == 1 {
		printUsage()
	}
	if os.Args[1] == "-f" {
		printChunkFiles()
	} else if os.Args[1] == "-h" {
		printHeights()
	} else {
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Usage: -f [file ...]")
	fmt.Println("Usage: -h [blockstore folder] [height ...]")
	os.Exit(1)
}

func printChunkFiles() {
	if len(os.Args) < 3 {
		printUsage()
	}
	for _, fn := range os.Args[2:] {
		f, err := os.Open(fn)
		if err != nil {
			log.Fatal(err)
		}
		printChunkFile(bufio.NewReader(zstd.NewReader(f)))
	}
}

func printHeights() {
	if len(os.Args) < 4 {
		printUsage()
	}
	bs := blockstore.NewBlockStore(context.Background(), config.BlockStore{Local: os.Args[2]}, "")
	for _, h := range heights() {
		b, err := bs.SingleBlock(h)
		if err != nil {
			log.Fatal(err)
		}
		printBlock(b)
	}
}

func heights() []int64 {
	heights := []int64{}
	for _, h := range os.Args[3:] {
		height, err := strconv.ParseInt(h, 10, 64)
		if err != nil {
			log.Fatal(err)
		}
		heights = append(heights, height)
	}
	return heights
}

func printChunkFile(rd *bufio.Reader) {
	for {
		bytes := nextLine(rd)
		if bytes == nil {
			break
		}
		block, err := blockstore.GobLineToBlock(bytes)
		if err != nil {
			log.Fatal(err)
		}
		printBlock(block)
	}
}

func printBlock(b *chain.Block) {
	bytes, err := tmjson.Marshal(b)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(bytes)
	os.Stdout.Write([]byte{'\n'})
}

func nextLine(rd *bufio.Reader) []byte {
	bytes, err := rd.ReadBytes('\n')
	if err != nil {
		if err == io.EOF {
			if len(bytes) == 0 {
				return nil
			}
			log.Println("premature end")
		}
		log.Fatal(err)
	}
	return bytes
}
