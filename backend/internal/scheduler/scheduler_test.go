package scheduler

import (
	"sync"
	"testing"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/log"

	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/suite"
)

type SchedulerTestSuite struct {
	suite.Suite
	logger    *log.Logger
	clock     *clockwork.FakeClock
	scheduler *KeyedScheduler
	mu        sync.Mutex
	triggered map[string]int
}

func TestSchedulerSuite(t *testing.T) {
	suite.Run(t, new(SchedulerTestSuite))
}

func (s *SchedulerTestSuite) SetupTest() {
	s.logger = log.NewNop()
	s.clock = clockwork.NewFakeClock()
	s.scheduler = newKeyedSchedulerWithClock(s.logger, s.clock)
	s.triggered = make(map[string]int)
}

func (s *SchedulerTestSuite) TearDownTest() {
	s.scheduler.Shutdown()
}

func (s *SchedulerTestSuite) onTrigger(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.triggered[key]++
}

func (s *SchedulerTestSuite) getTriggeredCount(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.triggered[key]
}

func (s *SchedulerTestSuite) getTriggeredKeys() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.triggered)
}

func (s *SchedulerTestSuite) TestBasic() {
	triggered := make(chan string, 2)

	go func() {
		for key := range s.scheduler.Chan() {
			s.onTrigger(key)
			triggered <- key
		}
	}()

	s.scheduler.Enqueue("key1", 50*time.Millisecond)
	s.scheduler.Enqueue("key2", 100*time.Millisecond)

	// Advance time to trigger key1
	s.clock.Advance(50 * time.Millisecond)
	s.Assert().Equal("key1", <-triggered) // wait for key1

	// Advance time to trigger key2
	s.clock.Advance(50 * time.Millisecond)
	s.Assert().Equal("key2", <-triggered) // wait for key2

	s.Assert().Equal(1, s.getTriggeredCount("key1"))
	s.Assert().Equal(1, s.getTriggeredCount("key2"))
}

func (s *SchedulerTestSuite) TestCancel() {
	nowPlus100ms := s.clock.Now().Add(100 * time.Millisecond)
	nowPlus200ms := s.clock.Now().Add(200 * time.Millisecond)

	// cannot use Enqueue, because it only send to channel
	s.scheduler.doEnqueue(&item{key: "key1", ts: nowPlus100ms})
	s.scheduler.doEnqueue(&item{key: "key2", ts: nowPlus200ms})

	s.Assert().Equal(2, len(s.scheduler.items))
	s.Assert().Equal(2, len(s.scheduler.heap))
	s.Assert().Equal(s.scheduler.timerTS, nowPlus100ms)

	s.scheduler.doCancel("key1")

	s.Assert().Equal(1, len(s.scheduler.items))
	s.Assert().Equal(1, len(s.scheduler.heap))
	s.Assert().Equal(s.scheduler.timerTS, nowPlus200ms)
	_, ok := s.scheduler.items["key2"]
	s.Assert().True(ok)
}

func (s *SchedulerTestSuite) TestClear() {
	nowPlus100ms := s.clock.Now().Add(100 * time.Millisecond)

	// cannot use Enqueue, because it only send to channel
	s.scheduler.doEnqueue(&item{key: "key1", ts: nowPlus100ms})
	s.scheduler.doEnqueue(&item{key: "key2", ts: nowPlus100ms})
	s.scheduler.doClear()

	// empty
	s.Assert().Equal(0, len(s.scheduler.items))
}

func (s *SchedulerTestSuite) TestUpdate() {
	triggered := make(chan string, 1)

	go func() {
		for key := range s.scheduler.Chan() {
			s.onTrigger(key)
			triggered <- key
		}
	}()

	s.scheduler.Enqueue("key1", 100*time.Millisecond)
	s.scheduler.Enqueue("key1", 50*time.Millisecond)

	// Advance to the earlier time (50ms)
	s.clock.Advance(50 * time.Millisecond)
	<-triggered

	s.Assert().Equal(1, s.getTriggeredCount("key1"))
	s.Assert().Equal(0, len(s.scheduler.items))
}

func (s *SchedulerTestSuite) TestUpdateLater() {
	triggered := make(chan string, 1)

	go func() {
		for key := range s.scheduler.Chan() {
			s.onTrigger(key)
			triggered <- key
		}
	}()

	nowPlus100ms := s.clock.Now().Add(100 * time.Millisecond)
	nowPlus200ms := s.clock.Now().Add(200 * time.Millisecond)

	s.scheduler.doEnqueue(&item{key: "key1", ts: nowPlus100ms})
	s.scheduler.doEnqueue(&item{key: "key1", ts: nowPlus200ms})

	// Advance to the earlier time (50ms)
	s.clock.Advance(100 * time.Millisecond)
	<-triggered

	s.Assert().Equal(1, s.getTriggeredCount("key1"))
	// no more items
	s.Assert().Equal(0, len(s.scheduler.items))
}

func (s *SchedulerTestSuite) TestConcurrentKeys() {
	expectedCount := 10
	triggered := make(chan string, expectedCount)

	go func() {
		for key := range s.scheduler.Chan() {
			s.onTrigger(key)
			triggered <- key
		}
	}()

	for i := range expectedCount {
		key := "key" + string(rune('0'+i))
		s.scheduler.Enqueue(key, 50*time.Millisecond)
	}

	// Advance time to trigger all keys
	s.clock.Advance(50 * time.Millisecond)

	// Wait for all to be triggered
	for range expectedCount {
		<-triggered
	}

	s.Assert().Equal(expectedCount, s.getTriggeredKeys())
}
