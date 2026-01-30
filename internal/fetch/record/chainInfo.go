package record

import (
	"strings"
)

// Captures chain info in memory.
type chainInfo struct {
	poolsCurrent map[string]string
	mimirCurrent map[string]int64
}

func newChainInfo() *chainInfo {
	return &chainInfo{
		poolsCurrent: make(map[string]string),
		mimirCurrent: make(map[string]int64),
	}
}

func (t *chainInfo) CurrentPoolStatus(pool string) string {
	if p, ok := t.poolsCurrent[pool]; ok {
		return p
	}
	return ""
}

func (t *chainInfo) CurrentMimirStatus(key string) int64 {
	if p, ok := t.mimirCurrent[key]; ok {
		return p
	}
	return 0
}

// Keeping them in lowercase
func (t *chainInfo) SetPoolStatus(pool, status string) {
	t.poolsCurrent[pool] = strings.ToLower(status)
}

func (t *chainInfo) SetMimirStatus(key string, value int64) {
	t.mimirCurrent[key] = value
}
