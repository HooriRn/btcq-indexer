package api

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"

	"gitlab.com/thorchain/midgard/internal/fetch/notinchain"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func wrapBonderDetails(nodeURL string) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		bonderAddress := ps[0].Value
		nodeAcc, err := notinchain.NodeAccountsLookup()
		var totalBonded int64 = 0
		if err != nil {
			log.Error().Err(err).Msg("failed to get node accounts")
			http.Error(w, "failed to get node accounts", http.StatusInternalServerError)
			return
		}

		bonderDetails := oapigen.BonderDetailsResponse{
			Address:     bonderAddress,
			Nodes:       []oapigen.BonderNode{},
			TotalBonded: "0",
		}

		for _, node := range nodeAcc {
			providers := node.BondProviders.Providers
			for _, provider := range providers {
				if provider.Address == bonderAddress {
					totalBonded += util.MustParseInt64(provider.Bond)
					bonderDetails.Nodes = append(bonderDetails.Nodes, oapigen.BonderNode{
						Address: node.NodeAddr,
						Bond:    provider.Bond,
						Status:  node.Status,
					})
				}
			}
		}

		bonderDetails.TotalBonded = util.IntStr(totalBonded)
		respJSON(w, bonderDetails)
	}
}
