package redis

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/redis/go-redis/v9"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

// RedisForever wraps go-redis client with automatic retry using exponential backoff.
// All operations retry forever until successful or context is cancelled.
type Forever interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	Del(ctx context.Context, key string) error
	HSet(ctx context.Context, key string, values ...any) error
	HGet(ctx context.Context, key string, field string) (string, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HDel(ctx context.Context, key string, fields ...string) error
	EvalSha(ctx context.Context, sha1 string, keys []string, args ...any) (any, error)
}

type redisForeverImpl struct {
	client          redis.UniversalClient
	logger          *log.Logger
	initialInterval time.Duration
	maxInterval     time.Duration
}

// NewForever creates a new Redis utility with forever backoff retry logic.
// initialInterval: starting backoff interval (e.g., 100ms)
// maxInterval: maximum backoff interval (e.g., 10s)
func NewForever(
	client redis.UniversalClient,
	initialInterval time.Duration,
	maxInterval time.Duration,
	logger *log.Logger,
) Forever {
	if client == nil {
		panic("redis client is required")
	}
	if logger == nil {
		panic("logger is required")
	}
	if initialInterval <= 0 {
		initialInterval = 100 * time.Millisecond
	}
	if maxInterval <= 0 {
		maxInterval = 10 * time.Second
	}

	return &redisForeverImpl{
		client:          client,
		logger:          logger,
		initialInterval: initialInterval,
		maxInterval:     maxInterval,
	}
}

// newForeverBackoff creates a new exponential backoff that retries forever.
func (r *redisForeverImpl) newForeverBackoff() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = r.initialInterval
	b.MaxInterval = r.maxInterval
	b.MaxElapsedTime = 0 // 0 means forever
	return b
}

// retryWithBackoff tries operation once first, only creates backoff object if first attempt fails.
// This optimizes for the common case where Redis operations succeed on first try.
func (r *redisForeverImpl) retryWithBackoff(ctx context.Context, operation func() error, operationName string) error {
	// Fast path: try once without backoff overhead
	err := operation()
	if err == nil {
		return nil
	}

	// First attempt failed, log and enter retry loop with backoff
	r.logger.Warn("Redis operation failed, entering retry mode",
		log.String("operation", operationName),
		log.Error(err))

	b := r.newForeverBackoff()
	attempt := 1 // First attempt already done

	return backoff.Retry(func() error {
		select {
		case <-ctx.Done():
			return backoff.Permanent(ctx.Err())
		default:
		}

		attempt++
		err := operation()
		if err != nil {
			r.logger.Warn("Redis operation retry failed",
				log.String("operation", operationName),
				log.Int("attempt", attempt),
				log.Error(err))
			return err
		}

		r.logger.Info("Redis operation recovered",
			log.String("operation", operationName),
			log.Int("total_attempts", attempt))
		return nil
	}, backoff.WithContext(b, ctx))
}

func (r *redisForeverImpl) Get(ctx context.Context, key string) (string, error) {
	var result string
	err := r.retryWithBackoff(ctx, func() error {
		val, err := r.client.Get(ctx, key).Result()
		if err != nil {
			return err
		}
		result = val
		return nil
	}, "Get")
	return result, err
}

func (r *redisForeverImpl) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	return r.retryWithBackoff(ctx, func() error {
		return r.client.Set(ctx, key, value, expiration).Err()
	}, "Set")
}

func (r *redisForeverImpl) Del(ctx context.Context, key string) error {
	return r.retryWithBackoff(ctx, func() error {
		return r.client.Del(ctx, key).Err()
	}, "Del")
}

func (r *redisForeverImpl) HSet(ctx context.Context, key string, values ...any) error {
	return r.retryWithBackoff(ctx, func() error {
		return r.client.HSet(ctx, key, values...).Err()
	}, "HSet")
}

func (r *redisForeverImpl) HGet(ctx context.Context, key string, field string) (string, error) {
	var result string
	err := r.retryWithBackoff(ctx, func() error {
		val, err := r.client.HGet(ctx, key, field).Result()
		if err != nil {
			return err
		}
		result = val
		return nil
	}, "HGet")
	return result, err
}

func (r *redisForeverImpl) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	var result map[string]string
	err := r.retryWithBackoff(ctx, func() error {
		val, err := r.client.HGetAll(ctx, key).Result()
		if err != nil {
			return err
		}
		result = val
		return nil
	}, "HGetAll")
	return result, err
}

func (r *redisForeverImpl) HDel(ctx context.Context, key string, fields ...string) error {
	return r.retryWithBackoff(ctx, func() error {
		return r.client.HDel(ctx, key, fields...).Err()
	}, "HDel")
}

func (r *redisForeverImpl) EvalSha(ctx context.Context, sha1 string, keys []string, args ...any) (any, error) {
	var result any
	err := r.retryWithBackoff(ctx, func() error {
		val, err := r.client.EvalSha(ctx, sha1, keys, args...).Result()
		if err != nil {
			return err
		}
		result = val
		return nil
	}, "EvalSha")
	return result, err
}
