package redis

import (
	"time"

	"github.com/google/uuid"
	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	redisstream "github.com/imtaco/audio-rtc-exp/internal/stream/redis"

	"github.com/redis/go-redis/v9"
)

func NewPeer[T any](
	redisClient *redis.Client,
	streamOut string,
	streamIn string,
	consumerGroupName string,
	logger *log.Logger,
) (jsonrpc.Peer[T], error) {
	stream, err := newStream[T](
		redisClient,
		streamOut,
		streamIn,
		consumerGroupName,
		logger,
	)
	if err != nil {
		return nil, err
	}
	// TODO: init value of T ?
	return jsonrpc.NewPeer(stream, new(T), logger), nil
}

func NewConn[T any](
	handler jsonrpc.Handler[T],
	redisClient *redis.Client,
	streamOut string,
	streamIn string,
	consumerGroupName string,
	logger *log.Logger,
) (jsonrpc.Conn[T], error) {
	stream, err := newStream[T](
		redisClient,
		streamOut,
		streamIn,
		consumerGroupName,
		logger,
	)
	if err != nil {
		return nil, err
	}
	// TODO: init value of T ?
	return handler.NewConn(stream, new(T)), nil
}

func newStream[T any](
	redisClient *redis.Client,
	streamOut string,
	streamIn string,
	consumerGroupName string,
	logger *log.Logger,
) (jsonrpc.ObjectStream, error) {
	if logger == nil {
		panic("logger cannot be nil")
	}

	var producer redisstream.Producer
	var consumer redisstream.Consumer
	var err error

	if streamOut != "" {
		producer, err = redisstream.NewProducer(
			redisClient,
			streamOut,
			logger,
		)
		if err != nil {
			return nil, err

		}
	}
	if streamIn != "" {
		consumerID := uuid.NewString()
		consumer, err = redisstream.NewConsumer(
			redisClient,
			streamIn,
			consumerGroupName,
			consumerID,
			time.Second, // default block time
			logger,
		)
		if err != nil {
			return nil, err
		}
	}

	return &rsStream{
		producer: producer,
		consumer: consumer,
		logger:   logger,
	}, nil
}
