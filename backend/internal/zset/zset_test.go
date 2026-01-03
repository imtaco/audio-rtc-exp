package zset

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type ZsetTestSuite struct {
	suite.Suite
	zset *Zset[string]
}

func TestZsetSuite(t *testing.T) {
	suite.Run(t, new(ZsetTestSuite))
}

func (s *ZsetTestSuite) SetupTest() {
	s.zset = New[string]()
}

func (s *ZsetTestSuite) TestNew() {
	z := New[int]()
	s.Assert().NotNil(z)
	s.Assert().Equal(0, z.Len())
}

func (s *ZsetTestSuite) TestPutAndLen() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.Assert().Equal(1, s.zset.Len())

	s.zset.Put("key2", "value2", now.Add(time.Second))
	s.Assert().Equal(2, s.zset.Len())
}

func (s *ZsetTestSuite) TestPutOverwrite() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.Assert().Equal(1, s.zset.Len())

	s.zset.Put("key1", "value2", now.Add(time.Second))
	s.Assert().Equal(1, s.zset.Len())

	key, data, ts, ok := s.zset.Pop()
	s.Assert().True(ok)
	s.Assert().Equal("key1", key)
	s.Assert().Equal("value2", data)
	s.Assert().Equal(now.Add(time.Second), ts)
}

func (s *ZsetTestSuite) TestRemove() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.zset.Put("key2", "value2", now.Add(time.Second))
	s.zset.Put("key3", "value3", now.Add(2*time.Second))
	s.Assert().Equal(3, s.zset.Len())

	s.zset.Remove("key2")
	s.Assert().Equal(2, s.zset.Len())

	key, data, ts, ok := s.zset.Pop()
	s.Assert().True(ok)
	s.Assert().Equal("key1", key)
	s.Assert().Equal("value1", data)
	s.Assert().Equal(now, ts)
	s.Assert().Equal(1, s.zset.Len())

	key, data, ts, ok = s.zset.Pop()
	s.Assert().True(ok)
	s.Assert().Equal("key3", key)
	s.Assert().Equal("value3", data)
	s.Assert().Equal(now.Add(2*time.Second), ts)
	s.Assert().Equal(0, s.zset.Len())

	_, _, _, ok = s.zset.Pop()
	s.Assert().False(ok)
}

func (s *ZsetTestSuite) TestRemoveNonExistent() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.Assert().Equal(1, s.zset.Len())

	s.zset.Remove("nonexistent")
	s.Assert().Equal(1, s.zset.Len())
}

func (s *ZsetTestSuite) TestPop() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.zset.Put("key2", "value2", now.Add(time.Second))
	s.zset.Put("key3", "value3", now.Add(2*time.Second))

	key, data, ts, ok := s.zset.Pop()
	s.Assert().True(ok)
	s.Assert().Equal("key1", key)
	s.Assert().Equal("value1", data)
	s.Assert().Equal(now, ts)
	s.Assert().Equal(2, s.zset.Len())

	key, data, ts, ok = s.zset.Pop()
	s.Assert().True(ok)
	s.Assert().Equal("key2", key)
	s.Assert().Equal("value2", data)
	s.Assert().Equal(now.Add(time.Second), ts)
	s.Assert().Equal(1, s.zset.Len())

	key, data, ts, ok = s.zset.Pop()
	s.Assert().True(ok)
	s.Assert().Equal("key3", key)
	s.Assert().Equal("value3", data)
	s.Assert().Equal(now.Add(2*time.Second), ts)
	s.Assert().Equal(0, s.zset.Len())
}

func (s *ZsetTestSuite) TestPopEmpty() {
	key, data, ts, ok := s.zset.Pop()
	s.Assert().False(ok)
	s.Assert().Equal("", key)
	s.Assert().Equal("", data)
	s.Assert().Equal(time.Time{}, ts)
}

func (s *ZsetTestSuite) TestPeek() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.zset.Put("key2", "value2", now.Add(time.Second))

	key, data, ts, ok := s.zset.Peek()
	s.Assert().True(ok)
	s.Assert().Equal("key1", key)
	s.Assert().Equal("value1", data)
	s.Assert().Equal(now, ts)
	s.Assert().Equal(2, s.zset.Len())

	key, data, ts, ok = s.zset.Peek()
	s.Assert().True(ok)
	s.Assert().Equal("key1", key)
	s.Assert().Equal("value1", data)
	s.Assert().Equal(now, ts)
	s.Assert().Equal(2, s.zset.Len())
}

func (s *ZsetTestSuite) TestPeekEmpty() {
	key, data, ts, ok := s.zset.Peek()
	s.Assert().False(ok)
	s.Assert().Equal("", key)
	s.Assert().Equal("", data)
	s.Assert().Equal(time.Time{}, ts)
}

func (s *ZsetTestSuite) TestPopBefore() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.zset.Put("key2", "value2", now.Add(time.Second))
	s.zset.Put("key3", "value3", now.Add(2*time.Second))
	s.zset.Put("key4", "value4", now.Add(3*time.Second))

	entries := s.zset.PopBefore(now.Add(2*time.Second), 10)
	s.Assert().Equal(3, len(entries))
	s.Assert().Equal("key1", entries[0].Key)
	s.Assert().Equal("value1", entries[0].Data)
	s.Assert().Equal(now, entries[0].TS)
	s.Assert().Equal("key2", entries[1].Key)
	s.Assert().Equal("value2", entries[1].Data)
	s.Assert().Equal(now.Add(time.Second), entries[1].TS)
	s.Assert().Equal("key3", entries[2].Key)
	s.Assert().Equal("value3", entries[2].Data)
	s.Assert().Equal(now.Add(2*time.Second), entries[2].TS)
	s.Assert().Equal(1, s.zset.Len())
}

func (s *ZsetTestSuite) TestPopBeforeWithMaxItems() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.zset.Put("key2", "value2", now.Add(time.Second))
	s.zset.Put("key3", "value3", now.Add(2*time.Second))
	s.zset.Put("key4", "value4", now.Add(3*time.Second))

	entries := s.zset.PopBefore(now.Add(5*time.Second), 2)
	s.Assert().Equal(2, len(entries))
	s.Assert().Equal("key1", entries[0].Key)
	s.Assert().Equal("key2", entries[1].Key)
	s.Assert().Equal(2, s.zset.Len())
}

func (s *ZsetTestSuite) TestPopBeforeEmpty() {
	now := time.Now()

	entries := s.zset.PopBefore(now, 10)
	s.Assert().Equal(0, len(entries))
	s.Assert().Equal(0, s.zset.Len())
}

func (s *ZsetTestSuite) TestPopBeforeNoMatch() {
	now := time.Now()

	s.zset.Put("key1", "value1", now.Add(time.Second))
	s.zset.Put("key2", "value2", now.Add(2*time.Second))

	entries := s.zset.PopBefore(now, 10)
	s.Assert().Equal(0, len(entries))
	s.Assert().Equal(2, s.zset.Len())
}

func (s *ZsetTestSuite) TestTimeOrdering() {
	now := time.Now()

	s.zset.Put("key3", "value3", now.Add(3*time.Second))
	s.zset.Put("key1", "value1", now.Add(time.Second))
	s.zset.Put("key2", "value2", now.Add(2*time.Second))

	key, _, _, _ := s.zset.Pop()
	s.Assert().Equal("key1", key)

	key, _, _, _ = s.zset.Pop()
	s.Assert().Equal("key2", key)

	key, _, _, _ = s.zset.Pop()
	s.Assert().Equal("key3", key)
}

func (s *ZsetTestSuite) TestSameTimeOrderingByKey() {
	now := time.Now()

	s.zset.Put("key3", "value3", now)
	s.zset.Put("key1", "value1", now)
	s.zset.Put("key2", "value2", now)

	key, _, _, _ := s.zset.Pop()
	s.Assert().Equal("key1", key)

	key, _, _, _ = s.zset.Pop()
	s.Assert().Equal("key2", key)

	key, _, _, _ = s.zset.Pop()
	s.Assert().Equal("key3", key)
}

func (s *ZsetTestSuite) TestGenericTypes() {
	intZset := New[int]()
	now := time.Now()

	intZset.Put("key1", 100, now)
	intZset.Put("key2", 200, now.Add(time.Second))

	_, data, _, ok := intZset.Pop()
	s.Assert().True(ok)
	s.Assert().Equal(100, data)

	_, data, _, ok = intZset.Pop()
	s.Assert().True(ok)
	s.Assert().Equal(200, data)
}

func (s *ZsetTestSuite) TestComplexOperations() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.zset.Put("key2", "value2", now.Add(time.Second))
	s.zset.Put("key3", "value3", now.Add(2*time.Second))
	s.Assert().Equal(3, s.zset.Len())

	s.zset.Remove("key2")
	s.Assert().Equal(2, s.zset.Len())

	key, _, _, _ := s.zset.Pop()
	s.Assert().Equal("key1", key)
	s.Assert().Equal(1, s.zset.Len())

	s.zset.Put("key4", "value4", now.Add(500*time.Millisecond))
	s.Assert().Equal(2, s.zset.Len())

	key, _, _, _ = s.zset.Peek()
	s.Assert().Equal("key4", key)

	entries := s.zset.PopBefore(now.Add(2*time.Second), 10)
	s.Assert().Equal(2, len(entries))
	s.Assert().Equal("key4", entries[0].Key)
	s.Assert().Equal("key3", entries[1].Key)
	s.Assert().Equal(0, s.zset.Len())
}
