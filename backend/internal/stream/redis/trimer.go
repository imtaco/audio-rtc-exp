package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/redis/go-redis/v9"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type Trimer interface {
	TrimByTime(ctx context.Context, maxAge time.Duration) error
	TrimByMaxLen(ctx context.Context, maxLen int64) error
}

func NewTrimer(
	client *redis.Client,
	stream string,
	logger *log.Logger,
) Trimer {
	return &trimerImpl{
		client: client,
		stream: stream,
		logger: logger,
		clock:  clockwork.NewRealClock(),
	}
}

// NOTE: need Reids 8.4+ for ACKED option
type trimerImpl struct {
	client *redis.Client
	stream string
	logger *log.Logger
	clock  clockwork.Clock
}

func (st *trimerImpl) TrimByTime(ctx context.Context, maxAge time.Duration) error {
	minID := st.minID(maxAge)
	v, err := st.client.Do(
		ctx, "XTRIM", st.stream, "MINID", minID, "ACKED",
	).Result()
	if err != nil {
		return fmt.Errorf("failed to trim stream: %w", err)
	}

	trimmed := v.(int64)
	st.logger.Info("Trimmed stream",
		log.String("stream", st.stream),
		log.String("min_id", minID),
		log.Int64("trimmed_count", trimmed))

	return nil

}

func (st *trimerImpl) TrimByMaxLen(ctx context.Context, maxLen int64) error {
	v, err := st.client.Do(
		ctx, "XTRIM", st.stream, "MAXLEN", maxLen, "ACKED",
	).Result()
	if err != nil {
		return fmt.Errorf("failed to trim stream: %w", err)
	}

	trimmed := v.(int64)
	st.logger.Info("Trimmed stream by maxLen",
		log.String("stream", st.stream),
		log.Int64("max_len", maxLen),
		log.Int64("trimmed_count", trimmed))

	return nil
}
