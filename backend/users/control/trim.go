package control

import (
	"context"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/log"
	redisstream "github.com/imtaco/audio-rtc-exp/internal/stream/redis"
	"github.com/redis/go-redis/v9"
)

const (
	inStreamRetention    = 3 * time.Minute
	replyStreamRetention = 3 * time.Minute
	wsStreamRetention    = 3 * time.Minute
)

func NewTrimer(
	redisClient *redis.Client,
	streamIn string,
	streamReply string,
	wsStream string,
	interval time.Duration,
	logger *log.Logger,
) (*Trimer, error) {
	inTrimer := redisstream.NewTrimer(redisClient, streamIn, logger.Module("InTrimer"))
	outTrimer := redisstream.NewTrimer(redisClient, streamReply, logger.Module("OutTrimer"))
	wsTrimer := redisstream.NewTrimer(redisClient, wsStream, logger.Module("WsTrimer"))

	return &Trimer{
		inTrimer:  inTrimer,
		outTrimer: outTrimer,
		wsTrimer:  wsTrimer,
		interval:  interval,
		logger:    logger,
	}, nil
}

type Trimer struct {
	inTrimer  redisstream.Trimer
	outTrimer redisstream.Trimer
	wsTrimer  redisstream.Trimer
	interval  time.Duration
	cancel    context.CancelFunc
	logger    *log.Logger
}

func (t *Trimer) Start(ctx context.Context) error {
	ticker := time.NewTicker(t.interval)

	ctx, t.cancel = context.WithCancel(ctx)
	defer t.cancel()

	go func() {
		for {
			select {
			case <-ticker.C:
				t.trimOnce(ctx)
			case <-ctx.Done():
				ticker.Stop()
			}
		}
	}()

	return nil
}

func (t *Trimer) Stop() {
	if t.cancel != nil {
		t.cancel()
	}
}

func (t *Trimer) trimOnce(ctx context.Context) {

	if err := t.inTrimer.TrimByTime(ctx, inStreamRetention); err != nil {
		t.logger.Error("failed to trim in stream", log.Error(err))
	}
	if err := t.outTrimer.TrimByTime(ctx, replyStreamRetention); err != nil {
		t.logger.Error("failed to trim reply stream", log.Error(err))
	}
	if err := t.wsTrimer.TrimByTime(ctx, wsStreamRetention); err != nil {
		t.logger.Error("failed to trim ws stream", log.Error(err))
	}
}
