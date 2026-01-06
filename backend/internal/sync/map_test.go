package sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMap_StoreAndLoad(t *testing.T) {
	m := NewMap[string, int]()

	m.Store("key1", 42)

	value, ok := m.Load("key1")
	assert.True(t, ok)
	assert.Equal(t, 42, value)
}

func TestMap_LoadNonExistent(t *testing.T) {
	m := NewMap[string, int]()

	value, ok := m.Load("nonexistent")
	assert.False(t, ok)
	assert.Equal(t, 0, value)
}

func TestMap_Delete(t *testing.T) {
	m := NewMap[string, int]()

	m.Store("key1", 42)
	m.Delete("key1")

	_, ok := m.Load("key1")
	assert.False(t, ok)
}

func TestMap_LoadOrStore(t *testing.T) {
	m := NewMap[string, int]()

	// First call should store
	actual, loaded := m.LoadOrStore("key1", 42)
	assert.False(t, loaded)
	assert.Equal(t, 42, actual)

	// Second call should load existing
	actual, loaded = m.LoadOrStore("key1", 100)
	assert.True(t, loaded)
	assert.Equal(t, 42, actual)
}

func TestMap_Range(t *testing.T) {
	m := NewMap[string, int]()

	m.Store("key1", 1)
	m.Store("key2", 2)
	m.Store("key3", 3)

	count := 0
	sum := 0
	m.Range(func(_ string, value int) bool {
		count++
		sum += value
		return true
	})

	assert.Equal(t, 3, count)
	assert.Equal(t, 6, sum)
}

func TestMap_RangeStopEarly(t *testing.T) {
	m := NewMap[string, int]()

	m.Store("key1", 1)
	m.Store("key2", 2)
	m.Store("key3", 3)

	count := 0
	m.Range(func(_ string, _ int) bool {
		count++
		return count < 2
	})

	assert.LessOrEqual(t, count, 2)
}

func TestMap_LoadAndDelete(t *testing.T) {
	m := NewMap[string, int]()

	m.Store("key1", 42)

	value, loaded := m.LoadAndDelete("key1")
	assert.True(t, loaded)
	assert.Equal(t, 42, value)

	_, ok := m.Load("key1")
	assert.False(t, ok)
}

func TestMap_CompareAndSwap(t *testing.T) {
	m := NewMap[string, int]()

	m.Store("key1", 42)

	// Should succeed when old value matches
	swapped := m.CompareAndSwap("key1", 42, 100)
	assert.True(t, swapped)

	value, _ := m.Load("key1")
	assert.Equal(t, 100, value)

	// Should fail when old value doesn't match
	swapped = m.CompareAndSwap("key1", 42, 200)
	assert.False(t, swapped)

	value, _ = m.Load("key1")
	assert.Equal(t, 100, value)
}

func TestMap_CompareAndDelete(t *testing.T) {
	m := NewMap[string, int]()

	m.Store("key1", 42)

	// Should fail when value doesn't match
	deleted := m.CompareAndDelete("key1", 100)
	assert.False(t, deleted)

	_, ok := m.Load("key1")
	assert.True(t, ok)

	// Should succeed when value matches
	deleted = m.CompareAndDelete("key1", 42)
	assert.True(t, deleted)

	_, ok = m.Load("key1")
	assert.False(t, ok)
}

func TestMap_WithPointers(t *testing.T) {
	type Data struct {
		Value string
		Count int
	}

	m := NewMap[string, *Data]()

	data1 := &Data{Value: "test1", Count: 1}
	m.Store("key1", data1)

	loaded, ok := m.Load("key1")
	assert.True(t, ok)
	assert.Equal(t, data1, loaded)
	assert.Equal(t, "test1", loaded.Value)
}

func TestMap_ConcurrentAccess(t *testing.T) {
	m := NewMap[int, int]()

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			m.Store(n, n*10)
			value, ok := m.Load(n)
			assert.True(t, ok)
			assert.Equal(t, n*10, value)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestMap_WithLock_Basic(t *testing.T) {
	m := NewMap[string, int]()

	// Use WithLock to do multiple operations atomically
	m.WithLock(func(view View[string, int]) {
		view.Set("key1", 1)
		view.Set("key2", 2)
		view.Set("key3", 3)
	})

	value, ok := m.Load("key1")
	assert.True(t, ok)
	assert.Equal(t, 1, value)
}

func TestMap_WithLock_GetAndSet(t *testing.T) {
	m := NewMap[string, int]()
	m.Store("key1", 10)
	m.Store("key2", 20)

	m.WithLock(func(view View[string, int]) {
		val1, ok1 := view.Get("key1")
		val2, ok2 := view.Get("key2")
		assert.True(t, ok1)
		assert.True(t, ok2)
		view.Set("sum", val1+val2)
	})

	sum, ok := m.Load("sum")
	assert.True(t, ok)
	assert.Equal(t, 30, sum)
}

func TestMap_WithLock_Delete(t *testing.T) {
	m := NewMap[string, int]()

	m.WithLock(func(view View[string, int]) {
		view.Set("key1", 100)
		value, ok := view.Get("key1")
		assert.True(t, ok)
		assert.Equal(t, 100, value)

		view.Delete("key1")
		_, ok = view.Get("key1")
		assert.False(t, ok)
	})

	_, ok := m.Load("key1")
	assert.False(t, ok)
}

func TestMap_WithLock_Range(t *testing.T) {
	m := NewMap[string, int]()
	m.Store("key1", 1)
	m.Store("key2", 2)
	m.Store("key3", 3)

	var count, sum int
	m.WithLock(func(view View[string, int]) {
		view.Range(func(_ string, value int) bool {
			count++
			sum += value
			return true
		})
	})

	assert.Equal(t, 3, count)
	assert.Equal(t, 6, sum)
}

func TestMap_WithLock_Len(t *testing.T) {
	m := NewMap[string, int]()
	m.Store("key1", 1)
	m.Store("key2", 2)

	var length int
	m.WithLock(func(view View[string, int]) {
		length = view.Len()
	})

	assert.Equal(t, 2, length)
}

func TestMap_WithLock_Concurrency(_ *testing.T) {
	m := NewMap[int, int]()

	// Initialize map
	for i := 0; i < 10; i++ {
		m.Store(i, i)
	}

	done := make(chan bool)

	// Writer goroutine
	go func() {
		m.WithLock(func(view View[int, int]) {
			for i := 0; i < 10; i++ {
				view.Set(i, i*10)
			}
		})
		done <- true
	}()

	// Another writer goroutine
	go func() {
		m.WithLock(func(view View[int, int]) {
			sum := 0
			view.Range(func(_ int, value int) bool {
				sum += value
				return true
			})
		})
		done <- true
	}()

	<-done
	<-done
}

func TestMap_WithLock_Panic(t *testing.T) {
	m := NewMap[string, int]()
	m.Store("key1", 1)

	// Test that lock is released even when panic occurs
	func() {
		defer func() {
			_ = recover()
		}()
		m.WithLock(func(view View[string, int]) {
			view.Set("key2", 2)
			panic("test panic")
		})
	}()

	// Should be able to acquire lock after panic
	value, ok := m.Load("key1")
	assert.True(t, ok)
	assert.Equal(t, 1, value)
}
