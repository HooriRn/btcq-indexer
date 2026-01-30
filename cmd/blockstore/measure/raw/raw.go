// Measures raw zstd decompression speed of blockstore files
package main

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/zstd"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	folder := os.Args[1]
	dirEntry, err := os.ReadDir(folder)
	if err != nil {
		log.Fatal().Err(err).Msgf("Cannot read folder %s", folder)
	}

	min, max, avg, cnt := math.MaxFloat32, 0., 0., 0.
	for _, de := range dirEntry {
		cnt++
		timer := Measure()
		inFile, _, zstd := openInFile(folder, de)
		for {
			b := make([]byte, 2048)
			if _, err := (*zstd).Read(b); err != nil {
				if err == io.EOF {
					break
				}
				log.Fatal().Err(err).Msgf("Error reading file %s", inFile.Name())
			}
		}
		t := float64(timer())
		if err := inFile.Close(); err != nil {
			log.Fatal().Err(err).Msgf("Error reading file %s", inFile.Name())
		}
		fmt.Printf("%f\n", t)
		min, max, avg = math.Min(min, t), math.Max(max, t), avg+t
	}
	avg = avg / cnt
	fmt.Printf("min:%f,max:%f,avg:%f\n", min, max, avg)
}

func openInFile(folder string, dirEntry os.DirEntry) (*os.File, *bufio.Reader, *io.ReadCloser) {
	name := dirEntry.Name()
	inFileName := filepath.Join(folder, name)
	inFile, err := os.Open(inFileName)
	if err != nil {
		log.Fatal().Err(err).Msgf("Unable to open file %s", inFileName)
	}
	zstd := zstd.NewReader(inFile)
	r := bufio.NewReader(zstd)
	return inFile, r, &zstd
}

func Measure() func() (milli int) {
	start := time.Now()
	return func() int {
		return int(time.Since(start).Milliseconds())
	}
}
