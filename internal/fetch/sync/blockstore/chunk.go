package blockstore

import (
	"fmt"
	"regexp"
	"strconv"

	"gitlab.com/thorchain/midgard/internal/util/miderr"
)

type chunk struct {
	name   string
	height int64
	hash   string
}

const tmpChunkPattern = "^tmp$|^[0-9]{12}\\.tmp$"
const fullChunkFormat = "%012d"
const currentChunkName = "tmp"
const withoutExtension = ""

func NewChunk(name string) (*chunk, error) {
	c := chunk{name: name}
	mh, err := c.maxHeight()
	c.height = mh
	return &c, err
}

func (c chunk) maxHeight() (int64, error) {
	height, err := c.toHeight()
	if err != nil {
		return 0, miderr.InternalErrF("BlockStore: cannot convert chunk %v", c)
	}
	return height, nil
}

func (c chunk) toHeight() (int64, error) {
	return strconv.ParseInt(c.name, 10, 64)
}

func isTemporaryChunk(name string) bool {
	m, err := regexp.MatchString(tmpChunkPattern, name)
	return err == nil && m
}

func chunkName(height int64, ext string) string {
	return fmt.Sprintf("/"+fullChunkFormat, height) + ext
}
