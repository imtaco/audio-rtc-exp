package control

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type TrimerTestSuite struct {
	suite.Suite
	redisClient *redis.Client
	mr          *miniredis.Miniredis
	trimer      *Trimer
}

func (s *TrimerTestSuite) SetupTest() {
	mr, err := miniredis.Run()
	s.Require().NoError(err)

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := log.NewNop()
	trimer, err := NewTrimer(
		redisClient,
		"test:stream:in",
		"test:stream:reply",
		"test:ws:stream",
		100*time.Millisecond,
		logger,
	)
	s.Require().NoError(err)

	s.redisClient = redisClient
	s.mr = mr
	s.trimer = trimer
}

func (s *TrimerTestSuite) TearDownTest() {
	if s.trimer != nil {
		s.trimer.Stop()
	}
	if s.redisClient != nil {
		s.redisClient.Close()
	}
	if s.mr != nil {
		s.mr.Close()
	}
}

func TestTrimerSuite(t *testing.T) {
	suite.Run(t, new(TrimerTestSuite))
}

func (s *TrimerTestSuite) TestNewTrimer() {
	s.NotNil(s.trimer.inTrimer)
	s.NotNil(s.trimer.outTrimer)
	s.NotNil(s.trimer.wsTrimer)
	s.Equal(100*time.Millisecond, s.trimer.interval)
	s.NotNil(s.trimer.logger)
}

func (s *TrimerTestSuite) TestTrimOnce() {
	ctx := context.Background()

	s.redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: "test:stream:in",
		Values: map[string]interface{}{"data": "test1"},
	})
	s.redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: "test:stream:reply",
		Values: map[string]interface{}{"data": "test2"},
	})
	s.redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: "test:ws:stream",
		Values: map[string]interface{}{"data": "test3"},
	})

	s.trimer.trimOnce(ctx)
}

func (s *TrimerTestSuite) TestStartStop_Multiple() {
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	err := s.trimer.Start(ctx1)
	s.NoError(err)
	s.trimer.Stop()

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	err = s.trimer.Start(ctx2)
	s.NoError(err)
	s.trimer.Stop()
}
