package blockstore

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DataDog/zstd"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/chain"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

//TODO(freki): replace midloglog.Fatal()-s to midlog.Warn()-s on write path

var logger = midlog.LoggerForModule("blockstore")

type BlockStore struct {
	cfg               config.BlockStore
	chainId           string
	currentFile       *os.File
	blockWriter       io.WriteCloser
	writeCursorHeight int64
	lastFetchedHeight int64
}

// If chainId != "" then blocks until missing chunks are downloaded from remote repository to local
// folder. During the download the hashes of the remote chunks are checked.
//
// TODO(freki): Make sure that public functions return sane results for null.
// TODO(freki): Log if blockstore is created or not and what the latest height there is.
// TODO(freki): Read acceptable hash values for this specific chainId
func NewBlockStore(ctx context.Context, cfg config.BlockStore, chainId string) *BlockStore {
	if len(cfg.Local) == 0 {
		logger.Info("Not started, local folder not configured")
		return nil
	}
	b := &BlockStore{cfg: cfg, chainId: chainId}
	b.cleanUp()
	if b.chainId != "" {
		b.updateFromRemote(ctx)
	}
	b.lastFetchedHeight = b.findLastFetchedHeight()
	b.writeCursorHeight = b.lastFetchedHeight + 1
	return b
}

func (b *BlockStore) FistFetchedHeight() (int64, error) {
	if b == nil {
		return 0, miderr.InternalErr("No blockstore struct!")
	}
	chunks, err := b.getLocalChunks()
	if err != nil {
		return 0, err
	}
	h, err := (chunks[0]).toHeight()
	if err != nil {
		return 0, err
	}
	return (h - b.cfg.BlocksPerChunk + 1), nil
}

func (b *BlockStore) LastFetchedHeight() int64 {
	if b == nil {
		return 0
	}
	return b.lastFetchedHeight
}

func (b *BlockStore) HasHeight(height int64) bool {
	return height <= b.lastFetchedHeight
}

func (b *BlockStore) SingleBlock(height int64) (*chain.Block, error) {
	it := b.Iterator(height)
	res, err := it.Next()
	if err != nil {
		return nil, err
	}
	if err := it.cleanupCurrentChunk(); err != nil {
		return nil, err
	}
	return res, nil
}

func (b *BlockStore) DumpBlock(block *chain.Block, forceFinalizeChunk bool) {
	if b.currentFile == nil {
		err := b.startNewFile()
		if err != nil {
			logger.FatalEF(err,
				"Couldn't create temporary file. Did you create the folder %s ?",
				b.cfg.Local)
		}

		// TODO(freki): if compressionlevel == 0 keep original writer
		b.blockWriter = zstd.NewWriterLevel(b.currentFile, b.cfg.CompressionLevel)
	}

	err := writeBlockAsGobLine(block, b.blockWriter)
	if err != nil {
		logger.FatalEF(err, "Error writing to %s, block height %d",
			b.currentFile.Name(),
			block.Height)
	}

	b.writeCursorHeight = block.Height
	if block.Height%b.cfg.BlocksPerChunk == 0 || forceFinalizeChunk {
		logger.InfoF("Creating dump file for height %d", b.writeCursorHeight)

		err = b.finalizeChunk(chunkName(b.writeCursorHeight, withoutExtension))
		if err != nil {
			logger.FatalE(err, "Error creating file")
		}
	}
}

func (b *BlockStore) Close() {
	err := b.finalizeChunk(chunkName(b.writeCursorHeight, "."+currentChunkName))
	if err != nil {
		logger.ErrorE(err, "Error closing")
	}
}

func (b *BlockStore) Iterator(startHeight int64) Iterator {
	return Iterator{blockStore: b, nextHeight: startHeight}
}

func (b *BlockStore) findLastFetchedHeight() int64 {
	chunks, err := b.getLocalChunks()
	if err != nil || len(chunks) == 0 {
		if db.ConfigHasGenesis() {
			h := db.GenesisData.GetGenesisHeight()
			return h - h%config.Global.BlockStore.BlocksPerChunk
		}
		return 0
	}
	height := chunks[len(chunks)-1].height
	logger.InfoF("Last fetched height in blockstore: %d", height)
	return height
}

func (b *BlockStore) getLocalDirEntries() ([]os.DirEntry, error) {
	folder := b.cfg.Local
	dirEntries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	return dirEntries, nil
}

func (b *BlockStore) getLocalChunks() ([]*chunk, error) {
	dirEntries, err := b.getLocalDirEntries()
	if err != nil {
		logger.FatalE(err, "Error opening blockstore local folder, make sure folder exists")
	}
	var chunks []*chunk
	for _, de := range dirEntries {
		r, err := NewChunk(de.Name())
		if err != nil {
			continue
		}
		chunks = append(chunks, r)
	}
	return chunks, nil
}

func (b *BlockStore) getLocalChunkNames() (map[string]bool, error) {
	localChunks, err := b.getLocalChunks()
	if err != nil {
		return nil, err
	}
	res := map[string]bool{}
	if localChunks == nil {
		return res, nil
	}
	for _, r := range localChunks {
		res[r.name] = true
	}
	return res, nil
}

func (b *BlockStore) startNewFile() error {
	path := b.localPath(currentChunkName)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	b.currentFile = file
	return nil
}

func (b *BlockStore) finalizeChunk(chunkName string) error {
	chunkPath := b.localPath(chunkName)
	if b.currentFile == nil {
		return nil
	}
	if err := b.blockWriter.Close(); err != nil {
		return miderr.InternalErrF("BlockStore: error closing block writer: %v", err)
	}
	if _, err := os.Stat(chunkPath); err == nil {
		midlog.InfoF("BlockStore: error renaming temporary file to already existing: %s", chunkPath)
		if err = os.Remove(chunkPath); err != nil {
			return miderr.InternalErrF("Blockstore: error deleting the existing file for replace: %s (%v)", chunkPath, err)
		}
	}
	oldName := b.currentFile.Name()
	if b.blockWriter != b.currentFile {
		if err := b.currentFile.Close(); err != nil {
			return miderr.InternalErrF("BlockStore: error closing %s (%v)", oldName, err)
		}
	}
	if err := os.Rename(oldName, chunkPath); err != nil {
		return miderr.InternalErrF("BlockStore: error renaming %s (%v)", oldName, err)
	}
	b.currentFile = nil
	return nil
}

func (b *BlockStore) findChunkPathForHeight(h int64) (string, error) {
	chunks, err := b.getLocalChunks()
	if err != nil || len(chunks) == 0 {
		return "", err
	}
	lo, hi := 0, len(chunks)-1
	if chunks[hi].height < h {
		return "", io.EOF
	}
	for lo < hi {
		mid := lo + (hi-lo)/2
		if chunks[mid].height < h {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return b.localPath(chunks[lo].name), nil
}

func (b *BlockStore) cleanUp() {
	dirEntries, err := b.getLocalDirEntries()
	if err != nil {
		logger.FatalE(err, "Error listing directory")
	}
	for _, de := range dirEntries {
		chunkName := de.Name()
		if isTemporaryChunk(chunkName) {
			path := b.localPath(chunkName)
			logger.InfoF("cleanup, removing %s", path)
			if err := os.Remove(path); err != nil {
				logger.FatalEF(err, "Error cleaning up chunk  %s", path)
			}
		}
	}
	b.currentFile = nil
	b.blockWriter = nil
}

func (b *BlockStore) readChunkHashes() []*chunk {
	chunks := []*chunk{}
	logger.DebugF("Reading chunk hashes from %s", b.getChunkHashesPath())
	f, err := os.Open(b.getChunkHashesPath())
	if err != nil {
		logger.ErrorEF(err, "Error reading chunk hashes from: %s", b.getChunkHashesPath())
		return chunks
	}
	defer f.Close()
	r := bufio.NewReader(f)
	seen := map[string]bool{}
	for {
		bytes, err := r.ReadBytes('\n')
		if err != nil {
			if err == io.EOF && len(bytes) == 0 {
				break
			}
			logger.ErrorE(err, "Error reading chunk hashes")
			break
		}
		entry := string(bytes)
		fields := strings.Fields(entry)
		if len(fields) != 2 {
			logger.ErrorF("Invalid hash entry %s", entry)
			break
		}
		chunk, err := NewChunk(fields[1])
		if err != nil {
			logger.ErrorEF(err, "Error parsing %s", entry)
			break
		}
		chunk.hash = fields[0]
		if len(chunk.hash) == 0 {
			logger.ErrorEF(err, "Invalid hash entry %s", entry)
			break
		}
		if seen[chunk.name] {
			continue
		}
		seen[chunk.name] = true
		chunks = append(chunks, chunk)
	}
	if l := len(chunks); l > 0 {
		logger.DebugF("Last found chunk hash %v", chunks[l-1])
	} else {
		logger.Warn("No chunk hashes found")
	}
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].name < chunks[j].name
	})
	return chunks
}

func (b *BlockStore) getChunkHashesPath() string {
	if b.cfg.ChunkHashesPath != "" {
		return b.cfg.ChunkHashesPath
	}
	return "./resources/hashes/" + b.chainId
}

func (b *BlockStore) localPath(name string) string {
	return filepath.Join(b.cfg.Local, name)
}

func (b *BlockStore) remotePath(name string) string {
	return b.cfg.Remote + name
}
