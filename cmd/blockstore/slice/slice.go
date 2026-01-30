// Regroups a bunch of json blockstore files
// into new slices of blockPerChunk size files
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/DataDog/zstd"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/midgard/config"
)

var interrupted int32 = 0

func interrupt() {
	atomic.StoreInt32(&interrupted, 1)
}

func isInterrupted() bool {
	return atomic.LoadInt32(&interrupted) == 1
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	signals := make(chan os.Signal, 10)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signals
		interrupt()
	}()

	config.ReadGlobalFrom(os.Args[1])

	fromFolder := os.Args[2]
	toFolder := config.Global.BlockStore.Local
	dirEntry, err := os.ReadDir(fromFolder)
	if err != nil {
		log.Fatal().Err(err).Msgf("Cannot read folder %s", fromFolder)
		return
	}

	// TODO(freki): Consider replacing implementation: Create two Blockstore, one for reading
	//     one for writing and pipe it over
	count := int64(0)
	tmpFile, writer := openTmpFile(toFolder)
	for _, de := range dirEntry {
		r, inFile := openInFile(fromFolder, de)
		for {
			if isInterrupted() {
				return
			}
			inBytes, errRead := r.ReadBytes('\n')
			if errRead != nil {
				if errRead == io.EOF && len(inBytes) == 0 {
					break
				}
				log.Fatal().Err(errRead).Msgf("Error reading from %s", inFile.Name())
			}
			if _, err := writer.Write(inBytes); err != nil {
				log.Fatal().Err(err).Msgf("Error writing to %s", tmpFile.Name())
			}
			count++
			if count%config.Global.BlockStore.BlocksPerChunk == 0 {
				writer.Close()
				oldPath := tmpFile.Name()
				newPath := outPath(count, toFolder)
				tmpFile.Close()
				log.Info().Msgf("Creating %s", newPath)
				if os.Rename(oldPath, newPath) != nil {
					log.Fatal().Err(err).Msgf("Error renaming %s to %s", oldPath, newPath)
				}
				tmpFile, writer = openTmpFile(toFolder)
			}
		}
		inFile.Close()
	}
}

func outPath(height int64, outFolder string) string {
	return filepath.Join(outFolder, fmt.Sprintf("%012d", height))
}

func openInFile(folder string, dirEntry os.DirEntry) (*bufio.Reader, *os.File) {
	name := dirEntry.Name()
	inFileName := filepath.Join(folder, name)
	log.Info().Msgf("Opening file %s", inFileName)
	inFile, err := os.Open(inFileName)
	if err != nil {
		log.Fatal().Err(err).Msgf("Unable to open file %s", inFileName)
	}
	r := bufio.NewReader(zstd.NewReader(inFile))
	return r, inFile
}

func openTmpFile(toFolder string) (*os.File, *zstd.Writer) {
	outFileName := filepath.Join(toFolder, "tmp")
	outFile, err := os.Create(outFileName)
	if err != nil {
		log.Fatal().Err(err).Msgf("Unable to create file %s", outFileName)
	}
	writer := zstd.NewWriterLevel(outFile, config.Global.BlockStore.CompressionLevel)
	return outFile, writer
}
