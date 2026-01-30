package blockstore

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"io"

	"gitlab.com/thorchain/midgard/internal/fetch/sync/chain"
)

const (
	gobLineFormat    = "height=%012d,gob="
	gobLinePrefixLen = 12 + len("height=,gob=")
)

// Historical note (huginn): We planned to make the serialization part of BlockStore
// do a compulsory sanity-check: deserialize the output before writing it out and do
// a `reflect.DeepEqual` with the provided block.
// Unfortunately, this doesn't work because `DeepEqual` distinguishes between nil and non-nil
// 0-length slices. But `gob` does not distinguish between them.
//
// We have tried to use the `github.com/fxamacker/cbor/v2` CBOR library, which can properly
// distinguish between the different empty slice cases. Unfortunately, it doesn't do that if
// a field is annotated with `json:omitempty`, which is the case for most of the tendermint's
// types.

func writeBlockAsGobLine(block *chain.Block, w io.Writer) error {
	sBlock, err := blockToStored(block)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, gobLineFormat, block.Height)

	b64 := base64.NewEncoder(base64.RawStdEncoding, w)
	encoder := gob.NewEncoder(b64)
	err = encoder.Encode(&sBlock)
	if err != nil {
		return err
	}
	b64.Close()

	_, err = w.Write([]byte{'\n'})
	if err != nil {
		return err
	}

	return nil
}

func gobLineMatchHeight(line []byte, height int64) bool {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, gobLineFormat, height)
	return bytes.Equal(line[:gobLinePrefixLen], buf.Bytes())
}

func GobLineToBlock(line []byte) (*chain.Block, error) {
	buf := bytes.NewBuffer(line[gobLinePrefixLen:])
	u64 := base64.NewDecoder(base64.RawStdEncoding, buf)
	decoder := gob.NewDecoder(u64)

	var sBlock storedBlock
	err := decoder.Decode(&sBlock)
	if err != nil {
		return nil, err
	}

	return storedToBlock(&sBlock)
}
