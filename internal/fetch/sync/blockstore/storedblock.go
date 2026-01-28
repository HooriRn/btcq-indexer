package blockstore

import (
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	cmtbytes "github.com/cometbft/cometbft/libs/bytes"
	cmtversion "github.com/cometbft/cometbft/proto/tendermint/version"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	comettypes "github.com/cometbft/cometbft/types"
	tendermintabci "github.com/tendermint/tendermint/abci/types"
	tmbytes "github.com/tendermint/tendermint/libs/bytes"
	tmversion "github.com/tendermint/tendermint/proto/tendermint/version"
	tendermintcoretypes "github.com/tendermint/tendermint/rpc/core/types"
	tenderminttypes "github.com/tendermint/tendermint/types"

	"github.com/btcq/btcq-indexer/internal/fetch/sync/chain"
)

type storedBlock struct {
	// Block mirrors chain.Block, but using the tendermint libraries for backward
	// compatibility. It is also a field within the serialized struct for backward
	// compatibility.
	Block struct {
		Height    int64                                   `json:"height"`
		Time      time.Time                               `json:"time"`
		Hash      []byte                                  `json:"hash"`
		Results   *tendermintcoretypes.ResultBlockResults `json:"results"`
		PureBlock *tendermintcoretypes.ResultBlock        `json:"pure_block"`
	}
}

func blockToStored(block *chain.Block) (*storedBlock, error) {
	sBlock := storedBlock{}
	sBlock.Block.Height = block.Height
	sBlock.Block.Time = block.Time
	sBlock.Block.Hash = block.Hash
	sBlock.Block.Results = cometToTendermintBlockResults(block.Results)
	sBlock.Block.PureBlock = cometToTendermintBlock(block.PureBlock)
	return &sBlock, nil
}

func storedToBlock(sBlock *storedBlock) (*chain.Block, error) {
	block := &chain.Block{
		Height:    sBlock.Block.Height,
		Time:      sBlock.Block.Time,
		Hash:      sBlock.Block.Hash,
		Results:   tendermintToCometBlockResults(sBlock.Block.Results),
		PureBlock: tendermintToCometBlock(sBlock.Block.PureBlock),
	}
	return block, nil
}

////////////////////////////////////////////////////////////////////////////////////////
// Block Conversions
////////////////////////////////////////////////////////////////////////////////////////

func tendermintToCometBlock(tblock *tendermintcoretypes.ResultBlock) *coretypes.ResultBlock {
	block := &coretypes.ResultBlock{
		BlockID: comettypes.BlockID{
			Hash: cmtbytes.HexBytes(tblock.BlockID.Hash),
			PartSetHeader: comettypes.PartSetHeader{
				Total: tblock.BlockID.PartSetHeader.Total,
				Hash:  cmtbytes.HexBytes(tblock.BlockID.PartSetHeader.Hash),
			},
		},
		Block: &comettypes.Block{
			Header: comettypes.Header{
				Version: cmtversion.Consensus{
					Block: tblock.Block.Header.Version.Block,
					App:   tblock.Block.Header.Version.App,
				},
				ChainID: tblock.Block.Header.ChainID,
				Height:  tblock.Block.Header.Height,
				Time:    tblock.Block.Header.Time,
				LastBlockID: comettypes.BlockID{
					Hash: cmtbytes.HexBytes(tblock.Block.Header.LastBlockID.Hash),
					PartSetHeader: comettypes.PartSetHeader{
						Total: tblock.Block.Header.LastBlockID.PartSetHeader.Total,
						Hash:  cmtbytes.HexBytes(tblock.Block.Header.LastBlockID.PartSetHeader.Hash),
					},
				},
				LastCommitHash:     cmtbytes.HexBytes(tblock.Block.Header.LastCommitHash),
				DataHash:           cmtbytes.HexBytes(tblock.Block.Header.DataHash),
				ValidatorsHash:     cmtbytes.HexBytes(tblock.Block.Header.ValidatorsHash),
				NextValidatorsHash: cmtbytes.HexBytes(tblock.Block.Header.NextValidatorsHash),
				ConsensusHash:      cmtbytes.HexBytes(tblock.Block.Header.ConsensusHash),
				AppHash:            cmtbytes.HexBytes(tblock.Block.Header.AppHash),
				LastResultsHash:    cmtbytes.HexBytes(tblock.Block.Header.LastResultsHash),
				EvidenceHash:       cmtbytes.HexBytes(tblock.Block.Header.EvidenceHash),
				ProposerAddress:    comettypes.Address(tblock.Block.Header.ProposerAddress),
			},
			LastCommit: &comettypes.Commit{
				Height: tblock.Block.LastCommit.Height,
				Round:  tblock.Block.LastCommit.Round,
				BlockID: comettypes.BlockID{
					Hash: cmtbytes.HexBytes(tblock.Block.LastCommit.BlockID.Hash),
					PartSetHeader: comettypes.PartSetHeader{
						Total: tblock.Block.LastCommit.BlockID.PartSetHeader.Total,
						Hash:  cmtbytes.HexBytes(tblock.Block.LastCommit.BlockID.PartSetHeader.Hash),
					},
				},
			},
		},
	}

	for _, tx := range tblock.Block.Data.Txs {
		block.Block.Data.Txs = append(block.Block.Data.Txs, comettypes.Tx(tx))
	}

	for _, sig := range tblock.Block.LastCommit.Signatures {
		block.Block.LastCommit.Signatures = append(block.Block.LastCommit.Signatures, comettypes.CommitSig{
			BlockIDFlag:      comettypes.BlockIDFlag(sig.BlockIDFlag),
			ValidatorAddress: comettypes.Address(sig.ValidatorAddress),
			Timestamp:        sig.Timestamp,
			Signature:        sig.Signature,
		})
	}

	return block
}

func cometToTendermintBlock(cblock *coretypes.ResultBlock) *tendermintcoretypes.ResultBlock {
	tblock := &tendermintcoretypes.ResultBlock{
		BlockID: tenderminttypes.BlockID{
			Hash: tmbytes.HexBytes(cblock.BlockID.Hash),
			PartSetHeader: tenderminttypes.PartSetHeader{
				Total: cblock.BlockID.PartSetHeader.Total,
				Hash:  tmbytes.HexBytes(cblock.BlockID.PartSetHeader.Hash),
			},
		},
		Block: &tenderminttypes.Block{
			Header: tenderminttypes.Header{
				Version: tmversion.Consensus{
					Block: cblock.Block.Header.Version.Block,
					App:   cblock.Block.Header.Version.App,
				},
				ChainID: cblock.Block.Header.ChainID,
				Height:  cblock.Block.Header.Height,
				Time:    cblock.Block.Header.Time,
				LastBlockID: tenderminttypes.BlockID{
					Hash: tmbytes.HexBytes(cblock.Block.Header.LastBlockID.Hash),
					PartSetHeader: tenderminttypes.PartSetHeader{
						Total: cblock.Block.Header.LastBlockID.PartSetHeader.Total,
						Hash:  tmbytes.HexBytes(cblock.Block.Header.LastBlockID.PartSetHeader.Hash),
					},
				},
				LastCommitHash:     tmbytes.HexBytes(cblock.Block.Header.LastCommitHash),
				DataHash:           tmbytes.HexBytes(cblock.Block.Header.DataHash),
				ValidatorsHash:     tmbytes.HexBytes(cblock.Block.Header.ValidatorsHash),
				NextValidatorsHash: tmbytes.HexBytes(cblock.Block.Header.NextValidatorsHash),
				ConsensusHash:      tmbytes.HexBytes(cblock.Block.Header.ConsensusHash),
				AppHash:            tmbytes.HexBytes(cblock.Block.Header.AppHash),
				LastResultsHash:    tmbytes.HexBytes(cblock.Block.Header.LastResultsHash),
				EvidenceHash:       tmbytes.HexBytes(cblock.Block.Header.EvidenceHash),
				ProposerAddress:    tenderminttypes.Address(cblock.Block.Header.ProposerAddress),
			},
			LastCommit: &tenderminttypes.Commit{
				Height: cblock.Block.LastCommit.Height,
				Round:  cblock.Block.LastCommit.Round,
				BlockID: tenderminttypes.BlockID{
					Hash: tmbytes.HexBytes(cblock.Block.LastCommit.BlockID.Hash),
					PartSetHeader: tenderminttypes.PartSetHeader{
						Total: cblock.Block.LastCommit.BlockID.PartSetHeader.Total,
						Hash:  tmbytes.HexBytes(cblock.Block.LastCommit.BlockID.PartSetHeader.Hash),
					},
				},
			},
		},
	}

	for _, tx := range cblock.Block.Data.Txs {
		tblock.Block.Data.Txs = append(tblock.Block.Data.Txs, tenderminttypes.Tx(tx))
	}

	for _, sig := range cblock.Block.LastCommit.Signatures {
		tblock.Block.LastCommit.Signatures = append(tblock.Block.LastCommit.Signatures, tenderminttypes.CommitSig{
			BlockIDFlag:      tenderminttypes.BlockIDFlag(sig.BlockIDFlag),
			ValidatorAddress: tenderminttypes.Address(sig.ValidatorAddress),
			Timestamp:        sig.Timestamp,
			Signature:        sig.Signature,
		})
	}

	return tblock
}

////////////////////////////////////////////////////////////////////////////////////////
// Block Result Conversions
////////////////////////////////////////////////////////////////////////////////////////

func tendermintToCometBlockResults(tbr *tendermintcoretypes.ResultBlockResults) *coretypes.ResultBlockResults {
	txrs := make([]*abci.ExecTxResult, len(tbr.TxsResults))
	for i, tx := range tbr.TxsResults {
		txr := &abci.ExecTxResult{
			Code:      tx.Code,
			Data:      tx.Data,
			Log:       tx.Log,
			Info:      tx.Info,
			GasWanted: tx.GasWanted,
			GasUsed:   tx.GasUsed,
			Codespace: tx.Codespace,
		}
		events := make([]abci.Event, len(tx.Events))
		for i, event := range tx.Events {
			events[i] = tendermintToCometEvent(event, "")
		}
		txr.Events = events
		txrs[i] = txr
	}

	var finalizeBlockEvents []abci.Event
	for _, event := range tbr.BeginBlockEvents {
		finalizeBlockEvents = append(finalizeBlockEvents, tendermintToCometEvent(event, "BeginBlock"))
	}
	for _, event := range tbr.EndBlockEvents {
		finalizeBlockEvents = append(finalizeBlockEvents, tendermintToCometEvent(event, "EndBlock"))
	}

	return &coretypes.ResultBlockResults{
		Height:              tbr.Height,
		TxsResults:          txrs,
		FinalizeBlockEvents: finalizeBlockEvents,
	}
}

func tendermintToCometEvent(te tendermintabci.Event, mode string) abci.Event {
	ce := abci.Event{
		Type: te.Type,
	}
	var attrs []abci.EventAttribute
	for _, a := range te.Attributes {
		if a.Key != nil {
			attrs = append(attrs, abci.EventAttribute{
				Key:   string(a.Key),
				Value: string(a.Value),
			})
		}
	}
	if mode != "" {
		attrs = append(attrs, abci.EventAttribute{
			Key:   "mode",
			Value: mode,
		})
	}
	ce.Attributes = attrs

	return ce
}

func cometToTendermintBlockResults(cbr *coretypes.ResultBlockResults) *tendermintcoretypes.ResultBlockResults {
	tbr := &tendermintcoretypes.ResultBlockResults{
		Height: cbr.Height,
	}

	for _, tx := range cbr.TxsResults {
		txResult := &tendermintabci.ResponseDeliverTx{
			Code:      tx.Code,
			Data:      tx.Data,
			Log:       tx.Log,
			Info:      tx.Info,
			GasWanted: tx.GasWanted,
			GasUsed:   tx.GasUsed,
			Codespace: tx.Codespace,
		}

		events := make([]tendermintabci.Event, len(tx.Events))
		for i, event := range tx.Events {
			events[i] = cometToTendermintEvent(event)
		}
		txResult.Events = events

		tbr.TxsResults = append(tbr.TxsResults, txResult)
	}

	beginBlockEvents := []tendermintabci.Event{}
	endBlockEvents := []tendermintabci.Event{}

	for _, event := range cbr.FinalizeBlockEvents {
		mode := getEventMode(event)
		if mode == "BeginBlock" {
			beginBlockEvents = append(beginBlockEvents, cometToTendermintEvent(event))
		} else if mode == "EndBlock" {
			endBlockEvents = append(endBlockEvents, cometToTendermintEvent(event))
		} else {
			// Default to BeginBlock for events without mode
			// Also to be noted tendermint's migration events used to be in BeginBlock
			beginBlockEvents = append(beginBlockEvents, cometToTendermintEvent(event))
		}
	}

	tbr.BeginBlockEvents = beginBlockEvents
	tbr.EndBlockEvents = endBlockEvents

	return tbr
}

func cometToTendermintEvent(ce abci.Event) tendermintabci.Event {
	te := tendermintabci.Event{
		Type: ce.Type,
	}
	attrs := make([]tendermintabci.EventAttribute, len(ce.Attributes))
	for i, a := range ce.Attributes {
		if a.Key == "mode" {
			continue
		}
		attrs[i] = tendermintabci.EventAttribute{
			Key:   []byte(a.Key),
			Value: []byte(a.Value),
		}
	}
	te.Attributes = attrs

	return te
}

func getEventMode(event abci.Event) string {
	for _, attr := range event.Attributes {
		if attr.Key == "mode" {
			return attr.Value
		}
	}
	return ""
}
