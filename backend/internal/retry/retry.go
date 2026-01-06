package retry

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type Retry interface {
	Do(ctx context.Context, operation func() error) error
}

func New(logger *log.Logger, initialInterval, maxInterval, maxElapsedTime time.Duration) Retry {
	return &retryImpl{
		logger:          logger,
		initialInterval: initialInterval,
		maxInterval:     maxInterval,
		maxElapsedTime:  maxElapsedTime,
	}
}

type retryImpl struct {
	logger          *log.Logger
	initialInterval time.Duration
	maxInterval     time.Duration
	maxElapsedTime  time.Duration
}

func (r *retryImpl) Do(ctx context.Context, operation func() error) error {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = r.initialInterval
	b.MaxInterval = r.maxInterval
	b.MaxElapsedTime = r.maxElapsedTime

	attempt := 0
	return backoff.Retry(func() error {
		attempt++
		err := operation()
		if err != nil {
			r.logger.Warn("Retry attempt failed",
				log.Int("attempt", attempt),
				log.Error(err))
		}
		return err
	}, backoff.WithContext(b, ctx))
}
