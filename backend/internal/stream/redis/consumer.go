package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/imtaco/audio-rtc-exp/internal/retry"

	"github.com/redis/go-redis/v9"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

const (
	defaultBlockTime      = 5 * time.Second
	broadcastModeBacktime = 3 * time.Second
)

type Consumer interface {
	Open(ctx context.Context) error
	Close()
	// EnsureConsumerGroup(ctx context.Context) error
	Ack(ctx context.Context, ids ...string) error
	DeleteConsumer(ctx context.Context) error
	Channel() <-chan *Message
}

type consumerImpl struct {
	client        *redis.Client
	consumeOnce   sync.Once
	cancel        context.CancelFunc
	chMsg         chan *Message
	stream        string
	consumerGroup string
	consumerName  string
	blockTime     time.Duration
	lastID        string
	pendingMode   bool
	retry         retry.Retry
	logger        *log.Logger
	clock         clockwork.Clock
}

type Message struct {
	ID     string
	Values map[string]interface{}
	sc     *consumerImpl
	ctx    context.Context
}

func (m *Message) Ack() error {
	return m.sc.Ack(m.ctx, m.ID)
}

func NewConsumer(
	client *redis.Client,
	stream string,
	consumerGroup string,
	consumerName string,
	blockTime time.Duration,
	logger *log.Logger,
) (Consumer, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if stream == "" {
		return nil, fmt.Errorf("stream name is required")
	}
	// consumerGroup is optional now
	if consumerGroup != "" && consumerName == "" {
		return nil, fmt.Errorf("consumer name is required when consumer group is set")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if blockTime == 0 {
		blockTime = defaultBlockTime
	}

	return &consumerImpl{
		client:        client,
		chMsg:         make(chan *Message, 1), // TODO: buffer size configurable ?
		stream:        stream,
		consumerGroup: consumerGroup,
		consumerName:  consumerName,
		blockTime:     blockTime,
		lastID:        "$",
		pendingMode:   false,
		retry:         retry.New(logger, 100*time.Millisecond, 10*time.Second, 0), // 0 = retry forever
		logger:        logger,
		clock:         clockwork.NewRealClock(),
	}, nil
}

func (sc *consumerImpl) useGroup() bool {
	return sc.consumerGroup != ""
}

func (sc *consumerImpl) ensureConsumerGroup(ctx context.Context) error {
	if !sc.useGroup() {
		return nil
	}
	return sc.retry.Do(ctx, func() error {
		err := sc.client.XGroupCreateMkStream(ctx, sc.stream, sc.consumerGroup, "$").Err()
		if err != nil {
			if err.Error() == "BUSYGROUP Consumer Group name already exists" {
				sc.logger.Debug("Consumer group already exists",
					log.String("stream", sc.stream),
					log.String("group", sc.consumerGroup))
				return nil
			}
			return fmt.Errorf("failed to create consumer group: %w", err)
		}

		sc.logger.Info("Created consumer group",
			log.String("stream", sc.stream),
			log.String("group", sc.consumerGroup))
		return nil
	})
}

func (sc *consumerImpl) read(ctx context.Context, count int64) ([]redis.XStream, error) {
	if sc.useGroup() {
		return sc.readWithGroup(ctx, count)
	} else {
		return sc.readWihtoutGroup(ctx, count)
	}
}

func (sc *consumerImpl) readWihtoutGroup(ctx context.Context, count int64) ([]redis.XStream, error) {
	// No consumer group, read from latest (lastID)
	streams, err := sc.client.XRead(ctx, &redis.XReadArgs{
		Streams: []string{sc.stream, sc.lastID},
		Count:   count,
		Block:   sc.blockTime,
	}).Result()

	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read from stream: %w", err)
	}

	if len(streams) > 0 && len(streams[0].Messages) > 0 {
		msgs := streams[0].Messages
		sc.lastID = msgs[len(msgs)-1].ID
	}
	return streams, nil

}

func (sc *consumerImpl) readWithGroup(ctx context.Context, count int64) ([]redis.XStream, error) {
	startID := ""
	if sc.pendingMode {
		startID = "0"
	} else {
		startID = ">"
	}
	sc.logger.Info("xreadgroup", log.String("startID", startID))
	streams, err := sc.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    sc.consumerGroup,
		Consumer: sc.consumerName,
		Streams:  []string{sc.stream, startID},
		Count:    count,
		Block:    0,
	}).Result()

	if err != nil {
		sc.logger.Info("xread err", log.Error(err))
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read pending messages: %w", err)
	}

	if sc.pendingMode && (len(streams) == 0 || len(streams[0].Messages) == 0) {
		sc.logger.Debug("No more pending messages, leaving pending mode")
		sc.pendingMode = false
	}

	return streams, nil
}

func (sc *consumerImpl) Ack(ctx context.Context, ids ...string) error {
	if len(ids) == 0 || sc.consumerGroup == "" {
		return nil
	}

	return sc.retry.Do(ctx, func() error {
		err := sc.client.XAck(ctx, sc.stream, sc.consumerGroup, ids...).Err()
		if err != nil {
			return fmt.Errorf("failed to ack messages: %w", err)
		}
		return nil
	})
}

func (sc *consumerImpl) DeleteConsumer(ctx context.Context) error {
	return sc.retry.Do(ctx, func() error {
		err := sc.client.XGroupDelConsumer(ctx, sc.stream, sc.consumerGroup, sc.consumerName).Err()
		if err != nil {
			return fmt.Errorf("failed to delete consumer: %w", err)
		}

		sc.logger.Info("Deleted consumer",
			log.String("stream", sc.stream),
			log.String("group", sc.consumerGroup),
			log.String("consumer", sc.consumerName))

		return nil
	})
}

func (sc *consumerImpl) Open(ctx context.Context) error {
	if sc.useGroup() {
		if err := sc.ensureConsumerGroup(ctx); err != nil {
			return err
		}
		sc.pendingMode = true
	} else {
		sc.lastID = sc.minID(broadcastModeBacktime)
	}

	sc.consumeOnce.Do(func() {
		ctx, sc.cancel = context.WithCancel(ctx)
		go sc.consume(ctx)
	})
	return nil
}

func (sc *consumerImpl) Close() {
	if sc.cancel != nil {
		sc.cancel()
	}
}

func (sc *consumerImpl) Channel() <-chan *Message {
	return sc.chMsg
}

func (sc *consumerImpl) consume(ctx context.Context) {
	defer close(sc.chMsg)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := sc.read(ctx, 10)
		if err != nil {
			sc.logger.Error("Failed to read messages", log.Error(err))
			time.Sleep(100 * time.Millisecond) // Backoff ?
			continue
		}

		if len(streams) == 0 || len(streams[0].Messages) == 0 {
			continue
		}

		for _, xmsg := range streams[0].Messages {
			msg := &Message{
				ID:     xmsg.ID,
				Values: xmsg.Values,
				sc:     sc,
				ctx:    ctx,
			}
			select {
			case <-ctx.Done():
				return
			case sc.chMsg <- msg:
			}
		}
	}
}
