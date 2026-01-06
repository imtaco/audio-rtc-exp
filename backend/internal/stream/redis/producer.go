package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type Producer interface {
	Add(ctx context.Context, values map[string]interface{}) (string, error)
	AddWithID(ctx context.Context, id string, values map[string]interface{}) error
}

type producerImpl struct {
	client *redis.Client
	stream string
	logger *log.Logger
}

func NewProducer(
	client *redis.Client,
	stream string,
	logger *log.Logger,
) (Producer, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if stream == "" {
		return nil, fmt.Errorf("stream name is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	return &producerImpl{
		client: client,
		stream: stream,
		logger: logger,
	}, nil
}

func (sp *producerImpl) Add(ctx context.Context, values map[string]interface{}) (string, error) {
	id, err := sp.client.XAdd(ctx, &redis.XAddArgs{
		Stream: sp.stream,
		Values: values,
	}).Result()

	if err != nil {
		return "", fmt.Errorf("failed to add message to stream: %w", err)
	}

	sp.logger.Debug("Added message to stream",
		log.String("stream", sp.stream),
		log.String("id", id))

	return id, nil
}

func (sp *producerImpl) AddWithID(ctx context.Context, id string, values map[string]interface{}) error {
	err := sp.client.XAdd(ctx, &redis.XAddArgs{
		Stream: sp.stream,
		ID:     id,
		Values: values,
	}).Err()

	if err != nil {
		return fmt.Errorf("failed to add message with ID to stream: %w", err)
	}

	sp.logger.Debug("Added message with ID to stream",
		log.String("stream", sp.stream),
		log.String("id", id))

	return nil
}
