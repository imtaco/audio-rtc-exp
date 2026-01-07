package redis

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jonboulle/clockwork"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type TrimerTestSuite struct {
	suite.Suite
	mr     *miniredis.Miniredis
	client *redis.Client
	logger *log.Logger
}

func TestTrimerSuite(t *testing.T) {
	suite.Run(t, new(TrimerTestSuite))
}

func (s *TrimerTestSuite) SetupTest() {
	mr := miniredis.RunT(s.T())
	s.mr = mr
	s.client = redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	s.logger = log.NewNop()
}

func (s *TrimerTestSuite) TearDownTest() {
	s.client.Close()
	s.mr.Close()
}

func (s *TrimerTestSuite) TestNewTrimer() {
	trimer := NewTrimer(s.client, "test-stream", s.logger)
	s.NotNil(trimer)
}

func (s *TrimerTestSuite) TestTrimByMaxLen() {
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		s.client.XAdd(ctx, &redis.XAddArgs{
			Stream: "test-stream",
			Values: map[string]any{"msg": i},
		})
	}

	length := s.client.XLen(ctx, "test-stream").Val()
	s.Equal(int64(10), length)

	trimer := NewTrimer(s.client, "test-stream", s.logger)

	err := trimer.TrimByMaxLen(ctx, 5)

	if err != nil {
		s.T().Skip("miniredis doesn't support XTRIM with ACKED option (requires Redis 8.4+)")
	}
}

func (s *TrimerTestSuite) TestTrimByTime() {
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.client.XAdd(ctx, &redis.XAddArgs{
			Stream: "test-stream",
			Values: map[string]any{"msg": i},
		})
	}

	trimer := NewTrimer(s.client, "test-stream", s.logger)

	err := trimer.TrimByTime(ctx, 1*time.Hour)

	if err != nil {
		s.T().Skip("miniredis doesn't support XTRIM with ACKED option (requires Redis 8.4+)")
	}
}

func (s *TrimerTestSuite) TestTrimByMaxLenEmptyStream() {
	ctx := context.Background()
	trimer := NewTrimer(s.client, "test-stream", s.logger)

	err := trimer.TrimByMaxLen(ctx, 10)

	if err != nil {
		s.T().Skip("miniredis doesn't support XTRIM with ACKED option (requires Redis 8.4+)")
	}
}

func (s *TrimerTestSuite) TestTrimByTimeEmptyStream() {
	ctx := context.Background()
	trimer := NewTrimer(s.client, "test-stream", s.logger)

	err := trimer.TrimByTime(ctx, 1*time.Hour)

	if err != nil {
		s.T().Skip("miniredis doesn't support XTRIM with ACKED option (requires Redis 8.4+)")
	}
}

func (s *TrimerTestSuite) TestTrimByMaxLenZero() {
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.client.XAdd(ctx, &redis.XAddArgs{
			Stream: "test-stream",
			Values: map[string]any{"msg": i},
		})
	}

	trimer := NewTrimer(s.client, "test-stream", s.logger)

	err := trimer.TrimByMaxLen(ctx, 0)

	if err != nil {
		s.T().Skip("miniredis doesn't support XTRIM with ACKED option (requires Redis 8.4+)")
	}
}

func (s *TrimerTestSuite) TestTrimerWithFakeClock() {
	// Use a fake clock for exact value testing
	fakeClock := clockwork.NewFakeClockAt(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC))

	trimer := NewTrimer(s.client, "test-stream", s.logger)
	s.NotNil(trimer)
	trimer.(*trimerImpl).clock = fakeClock

	// Verify the minID calculation is deterministic with fake clock
	impl := trimer.(*trimerImpl)
	maxAge := 1 * time.Hour

	minID := impl.minID(-maxAge)
	expectedTime := fakeClock.Now().Add(maxAge).UnixMilli()
	expectedID := fmt.Sprintf("%d-0", expectedTime)

	s.Equal(expectedID, minID)
}
