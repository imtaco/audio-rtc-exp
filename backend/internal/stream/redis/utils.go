package redis

import (
	"fmt"
	"time"

	"github.com/jonboulle/clockwork"
)

func (sc *consumerImpl) minID(backtime time.Duration) string {
	return minIDWithClock(sc.clock, backtime)
}

func (st *trimerImpl) minID(backtime time.Duration) string {
	return minIDWithClock(st.clock, backtime)
}

// Helper function for testing
func minIDWithClock(clock clockwork.Clock, backtime time.Duration) string {
	cutoffTime := clock.Now().Add(-backtime).UnixMilli()
	return fmt.Sprintf("%d-0", cutoffTime)
}
