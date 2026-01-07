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
	s.NotNil(z)
	s.Equal(0, z.Len())
}

func (s *ZsetTestSuite) TestPutAndLen() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.Equal(1, s.zset.Len())

	s.zset.Put("key2", "value2", now.Add(time.Second))
	s.Equal(2, s.zset.Len())
}

func (s *ZsetTestSuite) TestPutOverwrite() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.Equal(1, s.zset.Len())

	s.zset.Put("key1", "value2", now.Add(time.Second))
	s.Equal(1, s.zset.Len())

	key, data, ts, ok := s.zset.Pop()
	s.True(ok)
	s.Equal("key1", key)
	s.Equal("value2", data)
	s.Equal(now.Add(time.Second), ts)
}

func (s *ZsetTestSuite) TestRemove() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.zset.Put("key2", "value2", now.Add(time.Second))
	s.zset.Put("key3", "value3", now.Add(2*time.Second))
	s.Equal(3, s.zset.Len())

	s.zset.Remove("key2")
	s.Equal(2, s.zset.Len())

	key, data, ts, ok := s.zset.Pop()
	s.True(ok)
	s.Equal("key1", key)
	s.Equal("value1", data)
	s.Equal(now, ts)
	s.Equal(1, s.zset.Len())

	key, data, ts, ok = s.zset.Pop()
	s.True(ok)
	s.Equal("key3", key)
	s.Equal("value3", data)
	s.Equal(now.Add(2*time.Second), ts)
	s.Equal(0, s.zset.Len())

	_, _, _, ok = s.zset.Pop()
	s.False(ok)
}

func (s *ZsetTestSuite) TestRemoveNonExistent() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.Equal(1, s.zset.Len())

	s.zset.Remove("nonexistent")
	s.Equal(1, s.zset.Len())
}

func (s *ZsetTestSuite) TestPop() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.zset.Put("key2", "value2", now.Add(time.Second))
	s.zset.Put("key3", "value3", now.Add(2*time.Second))

	key, data, ts, ok := s.zset.Pop()
	s.True(ok)
	s.Equal("key1", key)
	s.Equal("value1", data)
	s.Equal(now, ts)
	s.Equal(2, s.zset.Len())

	key, data, ts, ok = s.zset.Pop()
	s.True(ok)
	s.Equal("key2", key)
	s.Equal("value2", data)
	s.Equal(now.Add(time.Second), ts)
	s.Equal(1, s.zset.Len())

	key, data, ts, ok = s.zset.Pop()
	s.True(ok)
	s.Equal("key3", key)
	s.Equal("value3", data)
	s.Equal(now.Add(2*time.Second), ts)
	s.Equal(0, s.zset.Len())
}

func (s *ZsetTestSuite) TestPopEmpty() {
	key, data, ts, ok := s.zset.Pop()
	s.False(ok)
	s.Equal("", key)
	s.Equal("", data)
	s.Equal(time.Time{}, ts)
}

func (s *ZsetTestSuite) TestPeek() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.zset.Put("key2", "value2", now.Add(time.Second))

	key, data, ts, ok := s.zset.Peek()
	s.True(ok)
	s.Equal("key1", key)
	s.Equal("value1", data)
	s.Equal(now, ts)
	s.Equal(2, s.zset.Len())

	key, data, ts, ok = s.zset.Peek()
	s.True(ok)
	s.Equal("key1", key)
	s.Equal("value1", data)
	s.Equal(now, ts)
	s.Equal(2, s.zset.Len())
}

func (s *ZsetTestSuite) TestPeekEmpty() {
	key, data, ts, ok := s.zset.Peek()
	s.False(ok)
	s.Equal("", key)
	s.Equal("", data)
	s.Equal(time.Time{}, ts)
}

func (s *ZsetTestSuite) TestPopBefore() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.zset.Put("key2", "value2", now.Add(time.Second))
	s.zset.Put("key3", "value3", now.Add(2*time.Second))
	s.zset.Put("key4", "value4", now.Add(3*time.Second))

	entries := s.zset.PopBefore(now.Add(2*time.Second), 10)
	s.Equal(3, len(entries))
	s.Equal("key1", entries[0].Key)
	s.Equal("value1", entries[0].Data)
	s.Equal(now, entries[0].TS)
	s.Equal("key2", entries[1].Key)
	s.Equal("value2", entries[1].Data)
	s.Equal(now.Add(time.Second), entries[1].TS)
	s.Equal("key3", entries[2].Key)
	s.Equal("value3", entries[2].Data)
	s.Equal(now.Add(2*time.Second), entries[2].TS)
	s.Equal(1, s.zset.Len())
}

func (s *ZsetTestSuite) TestPopBeforeWithMaxItems() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.zset.Put("key2", "value2", now.Add(time.Second))
	s.zset.Put("key3", "value3", now.Add(2*time.Second))
	s.zset.Put("key4", "value4", now.Add(3*time.Second))

	entries := s.zset.PopBefore(now.Add(5*time.Second), 2)
	s.Equal(2, len(entries))
	s.Equal("key1", entries[0].Key)
	s.Equal("key2", entries[1].Key)
	s.Equal(2, s.zset.Len())
}

func (s *ZsetTestSuite) TestPopBeforeEmpty() {
	now := time.Now()

	entries := s.zset.PopBefore(now, 10)
	s.Equal(0, len(entries))
	s.Equal(0, s.zset.Len())
}

func (s *ZsetTestSuite) TestPopBeforeNoMatch() {
	now := time.Now()

	s.zset.Put("key1", "value1", now.Add(time.Second))
	s.zset.Put("key2", "value2", now.Add(2*time.Second))

	entries := s.zset.PopBefore(now, 10)
	s.Equal(0, len(entries))
	s.Equal(2, s.zset.Len())
}

func (s *ZsetTestSuite) TestTimeOrdering() {
	now := time.Now()

	s.zset.Put("key3", "value3", now.Add(3*time.Second))
	s.zset.Put("key1", "value1", now.Add(time.Second))
	s.zset.Put("key2", "value2", now.Add(2*time.Second))

	key, _, _, _ := s.zset.Pop()
	s.Equal("key1", key)

	key, _, _, _ = s.zset.Pop()
	s.Equal("key2", key)

	key, _, _, _ = s.zset.Pop()
	s.Equal("key3", key)
}

func (s *ZsetTestSuite) TestSameTimeOrderingByKey() {
	now := time.Now()

	s.zset.Put("key3", "value3", now)
	s.zset.Put("key1", "value1", now)
	s.zset.Put("key2", "value2", now)

	key, _, _, _ := s.zset.Pop()
	s.Equal("key1", key)

	key, _, _, _ = s.zset.Pop()
	s.Equal("key2", key)

	key, _, _, _ = s.zset.Pop()
	s.Equal("key3", key)
}

func (s *ZsetTestSuite) TestGenericTypes() {
	intZset := New[int]()
	now := time.Now()

	intZset.Put("key1", 100, now)
	intZset.Put("key2", 200, now.Add(time.Second))

	_, data, _, ok := intZset.Pop()
	s.True(ok)
	s.Equal(100, data)

	_, data, _, ok = intZset.Pop()
	s.True(ok)
	s.Equal(200, data)
}

func (s *ZsetTestSuite) TestComplexOperations() {
	now := time.Now()

	s.zset.Put("key1", "value1", now)
	s.zset.Put("key2", "value2", now.Add(time.Second))
	s.zset.Put("key3", "value3", now.Add(2*time.Second))
	s.Equal(3, s.zset.Len())

	s.zset.Remove("key2")
	s.Equal(2, s.zset.Len())

	key, _, _, _ := s.zset.Pop()
	s.Equal("key1", key)
	s.Equal(1, s.zset.Len())

	s.zset.Put("key4", "value4", now.Add(500*time.Millisecond))
	s.Equal(2, s.zset.Len())

	key, _, _, _ = s.zset.Peek()
	s.Equal("key4", key)

	entries := s.zset.PopBefore(now.Add(2*time.Second), 10)
	s.Equal(2, len(entries))
	s.Equal("key4", entries[0].Key)
	s.Equal("key3", entries[1].Key)
	s.Equal(0, s.zset.Len())
}
