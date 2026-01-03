package redis

import (
	"fmt"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/suite"
)

type UtilsTestSuite struct {
	suite.Suite
}

func TestUtilsTestSuite(t *testing.T) {
	suite.Run(t, new(UtilsTestSuite))
}

func (s *UtilsTestSuite) TestMinID() {
	// Use a fake clock for exact value testing
	fakeClock := clockwork.NewFakeClockAt(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC))
	backtime := 5 * time.Second

	result := minIDWithClock(fakeClock, backtime)

	expectedTime := fakeClock.Now().Add(-backtime).UnixMilli()
	expectedID := fmt.Sprintf("%d-0", expectedTime)

	s.Assert().Equal(expectedID, result)
}

func (s *UtilsTestSuite) TestMinIDWithDifferentDurations() {
	testCases := []struct {
		name     string
		backtime time.Duration
	}{
		{"1 second", 1 * time.Second},
		{"1 minute", 1 * time.Minute},
		{"1 hour", 1 * time.Hour},
		{"1 millisecond", 1 * time.Millisecond},
	}

	fakeClock := clockwork.NewFakeClockAt(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC))

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result := minIDWithClock(fakeClock, tc.backtime)
			s.Assert().Regexp(`^\d+-0$`, result)
		})
	}
}

func (s *UtilsTestSuite) TestMinIDFormat() {
	fakeClock := clockwork.NewFakeClockAt(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC))
	backtime := 1 * time.Second

	result := minIDWithClock(fakeClock, backtime)

	// Test exact value with fake clock
	expectedTime := fakeClock.Now().Add(-backtime).UnixMilli()
	expectedID := fmt.Sprintf("%d-0", expectedTime)
	s.Assert().Equal(expectedID, result)

	// Also verify format
	s.Assert().Regexp(`^\d+-0$`, result, "minID should return format 'timestamp-0'")
}
