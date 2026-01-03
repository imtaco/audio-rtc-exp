package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultPingTimeout = 3 * time.Second
)

func Ping(client *redis.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPingTimeout)
	defer cancel()
	return client.Ping(ctx).Err()
}
