package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db/dbinit"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

type Balance struct {
	addr     string
	asset    string
	amountE8 int64
}

func (b Balance) key() string {
	return strings.ToUpper(b.asset + "_" + b.addr)
}

type commandLineArguments struct {
	dbConfigPath    string
	thorGenesisPath string
}

func main() {
	args := parseCommandLineArguments()

	setupDB(args.dbConfigPath)

	thorBalances, height, timestamp := readThorBalances(args.thorGenesisPath)
	midgardBalances := readMidgardBalancesAt(timestamp)
	corrections := getCorrections(thorBalances, midgardBalances)

	printCorrections(height, timestamp, corrections)
}

func parseCommandLineArguments() commandLineArguments {
	midlog.LogCommandLine()
	if len(os.Args) != 3 {
		printUsage()
	}
	return commandLineArguments{
		dbConfigPath:    os.Args[1],
		thorGenesisPath: os.Args[2],
	}
}

func printUsage() {
	fmt.Println("Usage: postgres_config_json genesis_json")
	os.Exit(1)
}

func setupDB(dbConfigPath string) {
	config.MustLoadConfigFiles(dbConfigPath, &config.Global)
	dbinit.Setup()
}

func printCorrections(height int64, timestamp int64, corrections []BalanceCorrection) {
	fmt.Print("{")
	fmt.Printf(`"info": {"height": %v, "timestamp": %v}`, height, timestamp)
	var printedCorrections []string
	for _, c := range corrections {
		printedCorrections = append(printedCorrections, c.sprint())
	}
	sort.Strings(printedCorrections)
	fmt.Print(", \"corrections\" : [\n")
	delim := false
	for _, pc := range printedCorrections {
		if delim {
			fmt.Print(",\n")
		}
		fmt.Print(pc)
		delim = true
	}
	fmt.Print(`]`)
	fmt.Print("}")
}
