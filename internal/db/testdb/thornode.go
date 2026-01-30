package testdb

import (
	"net/http"

	"github.com/jarcoal/httpmock"
	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/midgard/internal/fetch/notinchain"
)

const thorNodeUrl = "http://thornode.com"

// Starts Thornode HTTP mock with some simiple / empty results.
func StartMockThornode() (deactivateCallback func()) {
	notinchain.BaseURL = thorNodeUrl
	httpmock.Activate()

	setInitialThornodeConstants()

	RegisterThornodeNodes([]notinchain.NodeAccount{})
	RegisterThornodeReserve(notinchain.Network{})
	RegisterThornodeVault([]notinchain.AsgardVaults{})

	return func() {
		httpmock.DeactivateAndReset()
	}
}

func RegisterThornodeNodes(nodeAccounts []notinchain.NodeAccount) {
	httpmock.RegisterResponder("GET", thorNodeUrl+"/nodes",
		func(req *http.Request) (*http.Response, error) {
			resp, err := httpmock.NewJsonResponse(200, nodeAccounts)
			if err != nil {
				return httpmock.NewStringResponse(500, ""), nil
			}
			return resp, nil
		},
	)
}

func RegisterThornodeReserve(networkData notinchain.Network) {
	httpmock.RegisterResponder("GET", thorNodeUrl+"/network",
		func(req *http.Request) (*http.Response, error) {
			vaultData := networkData
			resp, err := httpmock.NewJsonResponse(200, vaultData)
			if err != nil {
				return httpmock.NewStringResponse(500, ""), nil
			}
			return resp, nil
		},
	)
}

func RegisterThornodeVault(vaults []notinchain.AsgardVaults) {
	httpmock.RegisterResponder("GET", thorNodeUrl+"/vaults/asgard",
		func(req *http.Request) (*http.Response, error) {
			resp, err := httpmock.NewJsonResponse(200, vaults)
			if err != nil {
				return httpmock.NewStringResponse(500, ""), nil
			}
			return resp, nil
		},
	)
}

// Sets some non 0 values for the constants. To have some meaningful values in tests
// overwrite them with mimir.
// Assumes httpmock.Activate has been called
func setInitialThornodeConstants() {

	constants := notinchain.Constants{Int64Values: map[string]int64{
		"EmissionCurve":      1234,
		"BlocksPerYear":      1234,
		"ChurnInterval":      1234,
		"ChurnRetryInterval": 1234,
		"PoolCycle":          1234,
		"IncentiveCurve":     1234,
		"MinimumBondInRune":  1234,
	}}
	httpmock.RegisterResponder("GET", thorNodeUrl+"/constants",
		func(req *http.Request) (*http.Response, error) {
			resp, err := httpmock.NewJsonResponse(200, constants)
			if err != nil {
				return httpmock.NewStringResponse(500, ""), nil
			}
			return resp, nil
		},
	)
	err := notinchain.LoadConstants()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to read constants")
	}
}
