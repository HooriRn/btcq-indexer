package db

import (
	"context"
	"strings"
)

// BlockDetail is a single block with full contents (for API single-block response).
type BlockDetail struct {
	Height          int64
	Timestamp       Nano
	Hash            []byte
	FinalizedEvents []byte // JSONB
	Txs             []byte // JSONB
}

// BlockSummary is a block with only counts (for API list response).
type BlockSummary struct {
	Height                 int64
	Timestamp              Nano
	Hash                   []byte
	TxCount                int
	FinalizedEventsCount   int
}

const blockDetailQuery = `
SELECT bl.height, bl.timestamp, bl.hash, b.finalized_events, b.txs
FROM block_log bl
JOIN blocks b ON bl.height = b.height
`

const blocksListQuery = `
SELECT bl.height, bl.timestamp, bl.hash,
  jsonb_array_length(b.finalized_events) AS finalized_events_count,
  jsonb_array_length(b.txs) AS tx_count
FROM block_log bl
JOIN blocks b ON bl.height = b.height
ORDER BY bl.height DESC
LIMIT $1 OFFSET $2
`

// GetBlockByHeight returns one block with full details by height, or nil if not found.
func GetBlockByHeight(ctx context.Context, height int64) (*BlockDetail, error) {
	q := blockDetailQuery + ` WHERE bl.height = $1`
	rows, err := Query(ctx, q, height)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	var d BlockDetail
	err = rows.Scan(&d.Height, &d.Timestamp, &d.Hash, &d.FinalizedEvents, &d.Txs)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// GetBlockByHash returns one block with full details by hash (hex string, case-insensitive), or nil if not found.
func GetBlockByHash(ctx context.Context, hashHex string) (*BlockDetail, error) {
	hashHex = strings.ToLower(strings.TrimSpace(hashHex))
	q := blockDetailQuery + ` WHERE bl.hash = decode($1, 'hex')`
	rows, err := Query(ctx, q, hashHex)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	var d BlockDetail
	err = rows.Scan(&d.Height, &d.Timestamp, &d.Hash, &d.FinalizedEvents, &d.Txs)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// GetBlocksList returns a paginated list of block summaries, newest first.
func GetBlocksList(ctx context.Context, limit, offset int) ([]BlockSummary, error) {
	rows, err := Query(ctx, blocksListQuery, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []BlockSummary
	for rows.Next() {
		var s BlockSummary
		err = rows.Scan(&s.Height, &s.Timestamp, &s.Hash, &s.FinalizedEventsCount, &s.TxCount)
		if err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}
