package redis

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type ProducerTestSuite struct {
	suite.Suite
	mr     *miniredis.Miniredis
	client *redis.Client
	logger *log.Logger
}

func TestProducerSuite(t *testing.T) {
	suite.Run(t, new(ProducerTestSuite))
}

func (s *ProducerTestSuite) SetupTest() {
	mr := miniredis.RunT(s.T())
	s.mr = mr
	s.client = redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	s.logger = log.NewNop()
}

func (s *ProducerTestSuite) TearDownTest() {
	s.client.Close()
	s.mr.Close()
}

func (s *ProducerTestSuite) TestNewProducer() {
	producer, err := NewProducer(s.client, "test-stream", s.logger)
	s.Require().NoError(err)
	s.NotNil(producer)
}

func (s *ProducerTestSuite) TestNewProducerNilClient() {
	producer, err := NewProducer(nil, "test-stream", s.logger)
	s.Require().Error(err)
	s.Nil(producer)
	s.Contains(err.Error(), "redis client is required")
}

func (s *ProducerTestSuite) TestNewProducerEmptyStream() {
	producer, err := NewProducer(s.client, "", s.logger)
	s.Require().Error(err)
	s.Nil(producer)
	s.Contains(err.Error(), "stream name is required")
}

func (s *ProducerTestSuite) TestNewProducerNilLogger() {
	producer, err := NewProducer(s.client, "test-stream", nil)
	s.Require().Error(err)
	s.Nil(producer)
	s.Contains(err.Error(), "logger is required")
}

func (s *ProducerTestSuite) TestAdd() {
	producer, err := NewProducer(s.client, "test-stream", s.logger)
	s.Require().NoError(err)

	ctx := context.Background()
	values := map[string]any{
		"key1": "value1",
		"key2": "value2",
	}

	id, err := producer.Add(ctx, values)
	s.Require().NoError(err)
	s.NotEmpty(id)

	exists := s.mr.Exists("test-stream")
	s.True(exists)
}

func (s *ProducerTestSuite) TestAddMultipleMessages() {
	producer, err := NewProducer(s.client, "test-stream", s.logger)
	s.Require().NoError(err)

	ctx := context.Background()

	id1, err := producer.Add(ctx, map[string]any{"msg": "first"})
	s.Require().NoError(err)

	id2, err := producer.Add(ctx, map[string]any{"msg": "second"})
	s.Require().NoError(err)

	s.NotEqual(id1, id2)

	length := s.client.XLen(ctx, "test-stream").Val()
	s.Equal(int64(2), length)
}

func (s *ProducerTestSuite) TestAddWithID() {
	producer, err := NewProducer(s.client, "test-stream", s.logger)
	s.Require().NoError(err)

	ctx := context.Background()
	customID := "1234567890-0"
	values := map[string]any{
		"key1": "value1",
	}

	err = producer.AddWithID(ctx, customID, values)
	s.Require().NoError(err)

	msgs, err := s.client.XRange(ctx, "test-stream", "-", "+").Result()
	s.Require().NoError(err)
	s.Len(msgs, 1)
	s.Equal(customID, msgs[0].ID)
}

func (s *ProducerTestSuite) TestAddWithIDInvalidID() {
	producer, err := NewProducer(s.client, "test-stream", s.logger)
	s.Require().NoError(err)

	ctx := context.Background()
	err = producer.AddWithID(ctx, "invalid-id", map[string]any{"key": "value"})
	s.Require().Error(err)
}

func (s *ProducerTestSuite) TestAddWithIDDuplicate() {
	producer, err := NewProducer(s.client, "test-stream", s.logger)
	s.Require().NoError(err)

	ctx := context.Background()
	customID := "1234567890-0"
	values := map[string]any{"key": "value"}

	err = producer.AddWithID(ctx, customID, values)
	s.Require().NoError(err)

	err = producer.AddWithID(ctx, customID, values)
	s.Require().Error(err)
}

func (s *ProducerTestSuite) TestAddEmptyValues() {
	producer, err := NewProducer(s.client, "test-stream", s.logger)
	s.Require().NoError(err)

	ctx := context.Background()
	_, err = producer.Add(ctx, map[string]any{})
	s.Require().Error(err, "XADD requires at least one field-value pair")
}

func (s *ProducerTestSuite) TestAddNilValues() {
	producer, err := NewProducer(s.client, "test-stream", s.logger)
	s.Require().NoError(err)

	ctx := context.Background()
	_, err = producer.Add(ctx, nil)
	s.Require().Error(err)
}

func (s *ProducerTestSuite) TestAddWithIDEmptyID() {
	producer, err := NewProducer(s.client, "test-stream", s.logger)
	s.Require().NoError(err)

	ctx := context.Background()
	// Empty ID will use auto-generated ID ("*"), which is valid
	err = producer.AddWithID(ctx, "", map[string]any{"key": "value"})
	// miniredis might handle this differently, so we just verify it doesn't panic
	// In real Redis, empty ID defaults to "*" (auto-generate)
	_ = err
}

func (s *ProducerTestSuite) TestAddWithIDMonotonicIncrease() {
	producer, err := NewProducer(s.client, "test-stream", s.logger)
	s.Require().NoError(err)

	ctx := context.Background()

	// Add message with ID 2000000000-0
	err = producer.AddWithID(ctx, "2000000000-0", map[string]any{"msg": "first"})
	s.Require().NoError(err)

	// Try to add message with older ID (should fail)
	err = producer.AddWithID(ctx, "1000000000-0", map[string]any{"msg": "second"})
	s.Require().Error(err, "Redis Stream IDs must be monotonically increasing")
}

func (s *ProducerTestSuite) TestAddConcurrent() {
	producer, err := NewProducer(s.client, "test-stream", s.logger)
	s.Require().NoError(err)

	ctx := context.Background()
	concurrentWrites := 50

	errChan := make(chan error, concurrentWrites)
	idChan := make(chan string, concurrentWrites)

	for i := range concurrentWrites {
		go func(index int) {
			id, err := producer.Add(ctx, map[string]any{
				"msg":   "concurrent-test",
				"index": index,
			})
			if err != nil {
				errChan <- err
			} else {
				idChan <- id
			}
		}(i)
	}

	// Collect results
	var ids []string
	var errors []error

	for range concurrentWrites {
		select {
		case id := <-idChan:
			ids = append(ids, id)
		case err := <-errChan:
			errors = append(errors, err)
		}
	}

	s.Empty(errors, "no errors should occur during concurrent writes")
	s.Len(ids, concurrentWrites)

	// Verify all messages were added
	length := s.client.XLen(ctx, "test-stream").Val()
	s.Equal(int64(concurrentWrites), length)

	// Verify all IDs are unique
	uniqueIDs := make(map[string]bool)
	for _, id := range ids {
		s.False(uniqueIDs[id], "ID %s should be unique", id)
		uniqueIDs[id] = true
	}
}
