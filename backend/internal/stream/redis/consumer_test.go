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

type ConsumerTestSuite struct {
	suite.Suite
	mr     *miniredis.Miniredis
	client *redis.Client
	logger *log.Logger
}

func TestConsumerSuite(t *testing.T) {
	suite.Run(t, new(ConsumerTestSuite))
}

func (s *ConsumerTestSuite) SetupTest() {
	mr := miniredis.RunT(s.T())
	s.mr = mr
	s.client = redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	s.logger = log.NewNop()
}

func (s *ConsumerTestSuite) TearDownTest() {
	s.client.Close()
	s.mr.Close()
}

func (s *ConsumerTestSuite) TestNewConsumer() {
	consumer, err := NewConsumer(s.client, "test-stream", "test-group", "test-consumer", 1*time.Second, s.logger)
	s.Assert().NoError(err)
	s.Assert().NotNil(consumer)
}

func (s *ConsumerTestSuite) TestNewConsumerWithoutGroup() {
	consumer, err := NewConsumer(s.client, "test-stream", "", "", 1*time.Second, s.logger)
	s.Assert().NoError(err)
	s.Assert().NotNil(consumer)
}

func (s *ConsumerTestSuite) TestNewConsumerNilClient() {
	consumer, err := NewConsumer(nil, "test-stream", "test-group", "test-consumer", 1*time.Second, s.logger)
	s.Assert().Error(err)
	s.Assert().Nil(consumer)
	s.Assert().Contains(err.Error(), "redis client is required")
}

func (s *ConsumerTestSuite) TestNewConsumerEmptyStream() {
	consumer, err := NewConsumer(s.client, "", "test-group", "test-consumer", 1*time.Second, s.logger)
	s.Assert().Error(err)
	s.Assert().Nil(consumer)
	s.Assert().Contains(err.Error(), "stream name is required")
}

func (s *ConsumerTestSuite) TestNewConsumerGroupWithoutName() {
	consumer, err := NewConsumer(s.client, "test-stream", "test-group", "", 1*time.Second, s.logger)
	s.Assert().Error(err)
	s.Assert().Nil(consumer)
	s.Assert().Contains(err.Error(), "consumer name is required")
}

func (s *ConsumerTestSuite) TestNewConsumerNilLogger() {
	consumer, err := NewConsumer(s.client, "test-stream", "test-group", "test-consumer", 1*time.Second, nil)
	s.Assert().Error(err)
	s.Assert().Nil(consumer)
	s.Assert().Contains(err.Error(), "logger is required")
}

func (s *ConsumerTestSuite) TestNewConsumerDefaultBlockTime() {
	consumer, err := NewConsumer(s.client, "test-stream", "test-group", "test-consumer", 0, s.logger)
	s.Assert().NoError(err)
	s.Assert().NotNil(consumer)

	impl := consumer.(*consumerImpl)
	s.Assert().Equal(defaultBlockTime, impl.blockTime)
}

func (s *ConsumerTestSuite) TestOpenWithGroup() {
	consumer, err := NewConsumer(s.client, "test-stream", "test-group", "test-consumer", 1*time.Second, s.logger)
	s.Require().NoError(err)

	ctx := context.Background()
	err = consumer.Open(ctx)
	s.Assert().NoError(err)

	defer consumer.Close()

	groups, err := s.client.XInfoGroups(ctx, "test-stream").Result()
	s.Assert().NoError(err)
	s.Assert().Len(groups, 1)
	s.Assert().Equal("test-group", groups[0].Name)
}

func (s *ConsumerTestSuite) TestOpenWithoutGroup() {
	s.client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: "test-stream",
		Values: map[string]interface{}{"key": "value"},
	})

	consumer, err := NewConsumer(s.client, "test-stream", "", "", 100*time.Millisecond, s.logger)
	s.Require().NoError(err)

	ctx := context.Background()
	err = consumer.Open(ctx)
	s.Assert().NoError(err)

	defer consumer.Close()

	s.Assert().NotNil(consumer.Channel())
}

func (s *ConsumerTestSuite) TestConsumeMessagesWithoutGroup() {
	ctx := context.Background()

	consumer, err := NewConsumer(s.client, "test-stream", "", "", 100*time.Millisecond, s.logger)
	s.Require().NoError(err)

	err = consumer.Open(ctx)
	s.Require().NoError(err)
	defer consumer.Close()

	s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: "test-stream",
		Values: map[string]interface{}{"msg": "test"},
	})

	select {
	case msg := <-consumer.Channel():
		s.Assert().NotNil(msg)
		s.Assert().NotEmpty(msg.ID)
		s.Assert().Equal("test", msg.Values["msg"])
	case <-time.After(100 * time.Millisecond):
		s.Fail("timeout waiting for message")
	}
}

func (s *ConsumerTestSuite) TestConsumeMessagesWithGroup() {
	ctx := context.Background()

	consumer, err := NewConsumer(s.client, "test-stream", "test-group", "test-consumer", 100*time.Millisecond, s.logger)
	s.Require().NoError(err)

	err = consumer.Open(ctx)
	s.Require().NoError(err)
	defer consumer.Close()

	// protected by group, so add message after opening consumer immediately is ok
	s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: "test-stream",
		Values: map[string]interface{}{"msg": "test"},
	})

	select {
	case msg := <-consumer.Channel():
		s.Assert().NotNil(msg)
		s.Assert().NotEmpty(msg.ID)
		s.Assert().Equal("test", msg.Values["msg"])
	case <-time.After(100 * time.Millisecond):
		s.Fail("timeout waiting for message")
	}
}

func (s *ConsumerTestSuite) TestAckMessage() {
	ctx := context.Background()

	consumer, err := NewConsumer(s.client, "test-stream", "test-group", "test-consumer", 100*time.Millisecond, s.logger)
	s.Require().NoError(err)

	err = consumer.Open(ctx)
	s.Require().NoError(err)
	defer consumer.Close()

	id, err := s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: "test-stream",
		Values: map[string]interface{}{"msg": "test"},
	}).Result()
	s.Require().NoError(err)

	select {
	case msg := <-consumer.Channel():
		err = msg.Ack()
		s.Assert().NoError(err)

		pending, err := s.client.XPending(ctx, "test-stream", "test-group").Result()
		s.Assert().NoError(err)
		s.Assert().Equal(int64(0), pending.Count)
	case <-time.After(500 * time.Millisecond):
		s.Fail("timeout waiting for message")
	}

	_ = id
}

func (s *ConsumerTestSuite) TestAckWithoutGroup() {
	ctx := context.Background()

	consumer, err := NewConsumer(s.client, "test-stream", "", "", 100*time.Millisecond, s.logger)
	s.Require().NoError(err)

	err = consumer.Ack(ctx, "123-0")
	s.Assert().NoError(err)
}

func (s *ConsumerTestSuite) TestDeleteConsumer() {
	ctx := context.Background()

	consumer, err := NewConsumer(s.client, "test-stream", "test-group", "test-consumer", 1*time.Second, s.logger)
	s.Require().NoError(err)

	err = consumer.Open(ctx)
	s.Require().NoError(err)
	defer consumer.Close()

	err = consumer.DeleteConsumer(ctx)
	s.Assert().NoError(err)
}

func (s *ConsumerTestSuite) TestCloseConsumer() {
	consumer, err := NewConsumer(s.client, "test-stream", "test-group", "test-consumer", 1*time.Second, s.logger)
	s.Require().NoError(err)

	ctx := context.Background()
	err = consumer.Open(ctx)
	s.Require().NoError(err)

	consumer.Close()

	select {
	case _, ok := <-consumer.Channel():
		s.Assert().False(ok, "channel should be closed")
	case <-time.After(100 * time.Millisecond):
		s.Fail("timeout waiting for channel to close")
	}
}

func (s *ConsumerTestSuite) TestChannel() {
	consumer, err := NewConsumer(s.client, "test-stream", "", "", 1*time.Second, s.logger)
	s.Require().NoError(err)

	ch := consumer.Channel()
	s.Assert().NotNil(ch)
}

func (s *ConsumerTestSuite) TestConsumerWithFakeClock() {
	// Use a fake clock for exact value testing
	fakeClock := clockwork.NewFakeClockAt(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC))

	consumer, err := NewConsumer(
		s.client,
		"test-stream",
		"",
		"test-consumer",
		1*time.Second,
		s.logger,
	)
	consumer.(*consumerImpl).clock = fakeClock
	s.Require().NoError(err)
	s.Assert().NotNil(consumer)

	_ = consumer.Open(context.Background())
	defer consumer.Close()

	// Verify the lastID is calculated correctly using the fake clock
	impl := consumer.(*consumerImpl)
	expectedID := "1735732797000-0"
	s.Assert().Equal(expectedID, impl.lastID)
}

func (s *ConsumerTestSuite) TestBroadcastMode_MultipleConsumersReceiveSameMessages() {
	ctx := context.Background()

	// Create two consumers without consumer group (broadcast mode)
	consumer1, err := NewConsumer(s.client, "test-stream", "", "", 100*time.Millisecond, s.logger)
	s.Require().NoError(err)
	err = consumer1.Open(ctx)
	s.Require().NoError(err)
	defer consumer1.Close()

	consumer2, err := NewConsumer(s.client, "test-stream", "", "", 100*time.Millisecond, s.logger)
	s.Require().NoError(err)
	err = consumer2.Open(ctx)
	s.Require().NoError(err)
	defer consumer2.Close()

	// Add a message to the stream
	s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: "test-stream",
		Values: map[string]interface{}{"msg": "broadcast-test", "id": "1"},
	})

	// Both consumers should receive the same message
	var msg1, msg2 *Message

	select {
	case msg1 = <-consumer1.Channel():
		s.Assert().NotNil(msg1)
		s.Assert().Equal("broadcast-test", msg1.Values["msg"])
	case <-time.After(500 * time.Millisecond):
		s.Fail("consumer1 timeout waiting for message")
	}

	select {
	case msg2 = <-consumer2.Channel():
		s.Assert().NotNil(msg2)
		s.Assert().Equal("broadcast-test", msg2.Values["msg"])
	case <-time.After(500 * time.Millisecond):
		s.Fail("consumer2 timeout waiting for message")
	}

	// Both should have received the same message ID
	s.Assert().Equal(msg1.ID, msg2.ID)
}

func (s *ConsumerTestSuite) TestConsumerGroup_PendingMessagesAfterReconnection() {
	ctx := context.Background()

	// Create first consumer and consume a message without acking
	consumer1, err := NewConsumer(s.client, "test-stream", "test-group", "consumer-1", 100*time.Millisecond, s.logger)
	s.Require().NoError(err)
	err = consumer1.Open(ctx)
	s.Require().NoError(err)

	// Add a message
	msgID, err := s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: "test-stream",
		Values: map[string]interface{}{"msg": "pending-test", "seq": "1"},
	}).Result()
	s.Require().NoError(err)

	// Consumer1 receives the message but doesn't ack it
	var msg1 *Message
	select {
	case msg1 = <-consumer1.Channel():
		s.Assert().NotNil(msg1)
		s.Assert().Equal("pending-test", msg1.Values["msg"])
		s.Assert().Equal(msgID, msg1.ID)
		// Intentionally NOT calling msg1.Ack()
	case <-time.After(500 * time.Millisecond):
		s.Fail("consumer1 timeout waiting for message")
	}

	// Verify message is now pending for consumer-1
	pending, err := s.client.XPending(ctx, "test-stream", "test-group").Result()
	s.Require().NoError(err)
	s.Assert().Equal(int64(1), pending.Count)

	// Close consumer1 (simulating disconnection)
	consumer1.Close()

	// Reconnect the SAME consumer (same name)
	consumer1Reconnected, err := NewConsumer(s.client, "test-stream", "test-group", "consumer-1", 100*time.Millisecond, s.logger)
	s.Require().NoError(err)
	err = consumer1Reconnected.Open(ctx)
	s.Require().NoError(err)
	defer consumer1Reconnected.Close()

	// The reconnected consumer should receive its own pending message
	var msg2 *Message
	select {
	case msg2 = <-consumer1Reconnected.Channel():
		s.Assert().NotNil(msg2)
		s.Assert().Equal("pending-test", msg2.Values["msg"])
		s.Assert().Equal(msgID, msg2.ID)
		// Now ack it
		err = msg2.Ack()
		s.Assert().NoError(err)
	case <-time.After(1 * time.Second):
		// Check if message is still pending
		pendingInfo, _ := s.client.XPending(ctx, "test-stream", "test-group").Result()
		s.T().Logf("Pending count: %d", pendingInfo.Count)
		s.Fail("reconnected consumer timeout waiting for pending message")
	}

	// Verify the message was acked
	pending, err = s.client.XPending(ctx, "test-stream", "test-group").Result()
	s.Assert().NoError(err)
	s.Assert().Equal(int64(0), pending.Count)
}

func (s *ConsumerTestSuite) TestConsumerGroup_MessagesDistributedAmongConsumers() {
	ctx := context.Background()

	// Create two consumers in the same group
	consumer1, err := NewConsumer(s.client, "test-stream", "test-group", "consumer-1", 100*time.Millisecond, s.logger)
	s.Require().NoError(err)
	err = consumer1.Open(ctx)
	s.Require().NoError(err)
	defer consumer1.Close()

	consumer2, err := NewConsumer(s.client, "test-stream", "test-group", "consumer-2", 100*time.Millisecond, s.logger)
	s.Require().NoError(err)
	err = consumer2.Open(ctx)
	s.Require().NoError(err)
	defer consumer2.Close()

	// Add multiple messages to the stream
	messageCount := 100
	for i := 0; i < messageCount; i++ {
		s.client.XAdd(ctx, &redis.XAddArgs{
			Stream: "test-stream",
			Values: map[string]interface{}{"msg": fmt.Sprintf("test-%d", i), "seq": i},
		})
	}

	// Collect messages from both consumers
	receivedByConsumer1 := 0
	receivedByConsumer2 := 0
	seenIDs := make(map[string]bool)

	timeout := time.After(time.Second)
	c1 := make(chan string)
	c2 := make(chan string)
	done := false

	go func() {
		for msg := range consumer1.Channel() {
			_ = msg.Ack()
			c1 <- msg.ID
		}
	}()

	go func() {
		for msg := range consumer2.Channel() {
			_ = msg.Ack()
			c2 <- msg.ID
		}
	}()

	for !done {
		select {
		case <-timeout:
			done = true
		case id := <-c1:
			receivedByConsumer1++
			seenIDs[id] = true
		case id := <-c2:
			receivedByConsumer2++
			seenIDs[id] = true
		}
		if receivedByConsumer1+receivedByConsumer2 >= messageCount {
			done = true
		}
	}

	// Both consumers should have received messages (load distribution)
	s.Assert().Greater(receivedByConsumer1, 0, "consumer1 should receive at least some messages")
	s.Assert().Greater(receivedByConsumer2, 0, "consumer2 should receive at least some messages")
	s.Assert().Equal(messageCount, receivedByConsumer1+receivedByConsumer2, "all messages should be consumed")

	// Each message should be delivered exactly once
	s.Assert().Equal(messageCount, len(seenIDs), "each message should be delivered exactly once")
}

func (s *ConsumerTestSuite) TestConsumerGroup_MultipleAcksSameMessage() {
	ctx := context.Background()

	consumer, err := NewConsumer(s.client, "test-stream", "test-group", "test-consumer", 100*time.Millisecond, s.logger)
	s.Require().NoError(err)
	err = consumer.Open(ctx)
	s.Require().NoError(err)
	defer consumer.Close()

	// Add a message
	s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: "test-stream",
		Values: map[string]interface{}{"msg": "test"},
	})

	// Consume and ack the message
	select {
	case msg := <-consumer.Channel():
		s.Assert().NotNil(msg)

		// Ack the message once
		err = msg.Ack()
		s.Assert().NoError(err)

		// Ack the same message again (should not error, idempotent)
		err = msg.Ack()
		s.Assert().NoError(err)
	case <-time.After(500 * time.Millisecond):
		s.Fail("timeout waiting for message")
	}
}

func (s *ConsumerTestSuite) TestBroadcastMode_NoAckNeeded() {
	ctx := context.Background()

	consumer, err := NewConsumer(s.client, "test-stream", "", "", 100*time.Millisecond, s.logger)
	s.Require().NoError(err)
	err = consumer.Open(ctx)
	s.Require().NoError(err)
	defer consumer.Close()

	s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: "test-stream",
		Values: map[string]interface{}{"msg": "test"},
	})

	select {
	case msg := <-consumer.Channel():
		s.Assert().NotNil(msg)
		// In broadcast mode, Ack should be a no-op
		err = msg.Ack()
		s.Assert().NoError(err)
	case <-time.After(500 * time.Millisecond):
		s.Fail("timeout waiting for message")
	}
}

func (s *ConsumerTestSuite) TestConsumerGroup_OldMessagesNotRedelivered() {
	ctx := context.Background()

	// Add a message BEFORE creating the consumer group
	oldMsgID, err := s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: "test-stream",
		Values: map[string]interface{}{"msg": "old-message"},
	}).Result()
	s.Require().NoError(err)

	// Create consumer (this creates the group at "$" position)
	consumer, err := NewConsumer(s.client, "test-stream", "test-group", "test-consumer", 100*time.Millisecond, s.logger)
	s.Require().NoError(err)
	err = consumer.Open(ctx)
	s.Require().NoError(err)
	defer consumer.Close()

	// Add a new message AFTER opening consumer
	newMsgID, err := s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: "test-stream",
		Values: map[string]interface{}{"msg": "new-message"},
	}).Result()
	s.Require().NoError(err)

	// Consumer should receive the new message, not the old one
	select {
	case msg := <-consumer.Channel():
		s.Assert().NotNil(msg)
		s.Assert().Equal(newMsgID, msg.ID, "should receive new message")
		s.Assert().NotEqual(oldMsgID, msg.ID, "should not receive old message")
		s.Assert().Equal("new-message", msg.Values["msg"])
		_ = msg.Ack()
	case <-time.After(500 * time.Millisecond):
		s.Fail("timeout waiting for message")
	}
}

func (s *ConsumerTestSuite) TestBroadcastMode_LateJoinerMissesOldMessages() {
	ctx := context.Background()

	// Add some messages first
	for i := range 3 {
		s.client.XAdd(ctx, &redis.XAddArgs{
			Stream: "test-stream",
			Values: map[string]interface{}{"msg": fmt.Sprintf("old-%d", i)},
		})
	}

	// Create consumer AFTER messages were added
	consumer, err := NewConsumer(s.client, "test-stream", "", "", 100*time.Millisecond, s.logger)
	// simulate longer cap from xadd to open 3001ms
	consumer.(*consumerImpl).clock = clockwork.NewFakeClockAt(time.Now().Add(3*time.Second + 1*time.Millisecond))

	s.Require().NoError(err)
	err = consumer.Open(ctx)
	s.Require().NoError(err)
	defer consumer.Close()

	// simulate to just pass the gap
	time.Sleep(2 * time.Millisecond)

	// Add a new message
	s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: "test-stream",
		Values: map[string]interface{}{"msg": "new"},
	})

	// Consumer should only receive the new message
	select {
	case msg := <-consumer.Channel():
		s.Assert().NotNil(msg)
		s.Assert().Equal("new", msg.Values["msg"])
	case <-time.After(100 * time.Millisecond):
		s.Fail("timeout waiting for message")
	}
}

func (s *ConsumerTestSuite) TestConsumerCloseIdempotent() {
	consumer, err := NewConsumer(s.client, "test-stream", "", "", 1*time.Second, s.logger)
	s.Require().NoError(err)

	ctx := context.Background()
	err = consumer.Open(ctx)
	s.Require().NoError(err)

	// Close multiple times should not panic
	consumer.Close()
	consumer.Close()
	consumer.Close()
}

func (s *ConsumerTestSuite) TestConsumerMultipleOpen() {
	consumer, err := NewConsumer(s.client, "test-stream", "", "", 1*time.Second, s.logger)
	s.Require().NoError(err)

	ctx := context.Background()

	// Open multiple times should only start consumption once
	err = consumer.Open(ctx)
	s.Assert().NoError(err)

	err = consumer.Open(ctx)
	s.Assert().NoError(err)

	err = consumer.Open(ctx)
	s.Assert().NoError(err)

	consumer.Close()
}
