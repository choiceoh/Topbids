package shortid

import (
	"fmt"
	"sync/atomic"
)

var counter atomic.Uint64

// New returns "prefix_NNNN" where NNNN is a zero-padded 4-digit counter.
func New(prefix string) string {
	n := counter.Add(1) - 1
	return fmt.Sprintf("%s_%04d", prefix, n%10000)
}
