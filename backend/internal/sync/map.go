package sync

import "sync"

// Map is a generic thread-safe map wrapper using RWMutex
type Map[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

// NewMap creates a new generic sync Map
func NewMap[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{
		m: make(map[K]V),
	}
}

// Load returns the value stored in the map for a key, or nil if no value is present.
// The ok result indicates whether value was found in the map.
func (m *Map[K, V]) Load(key K) (value V, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok = m.m[key]
	return
}

// Store sets the value for a key.
func (m *Map[K, V]) Store(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = value
}

// Delete deletes the value for a key.
func (m *Map[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, key)
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (m *Map[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	actual, loaded = m.m[key]
	if !loaded {
		m.m[key] = value
		actual = value
	}
	return
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.m {
		if !f(k, v) {
			break
		}
	}
}

// LoadAndDelete deletes the value for a key, returning the previous value if any.
// The loaded result reports whether the key was present.
func (m *Map[K, V]) LoadAndDelete(key K) (value V, loaded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, loaded = m.m[key]
	if loaded {
		delete(m.m, key)
	}
	return
}

// CompareAndSwap swaps the old and new values for key
// if the value stored in the map is equal to old.
// The old value must be of a comparable type.
func (m *Map[K, V]) CompareAndSwap(key K, old, newVal V) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, exists := m.m[key]
	if !exists {
		return false
	}
	// For comparable types, we can use ==
	// This works for basic types and pointers
	if any(current) == any(old) {
		m.m[key] = newVal
		return true
	}
	return false
}

// CompareAndDelete deletes the entry for key if its value is equal to old.
// The old value must be of a comparable type.
func (m *Map[K, V]) CompareAndDelete(key K, old V) (deleted bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, exists := m.m[key]
	if !exists {
		return false
	}
	if any(current) == any(old) {
		delete(m.m, key)
		return true
	}
	return false
}

// View provides read and write operations on the map without acquiring locks.
// It should only be used within the callback of WithLock.
// The underlying map must already be locked when using this interface.
type View[K comparable, V any] interface {
	// Get returns the value for a key.
	Get(key K) (value V, ok bool)

	// Set sets the value for a key.
	Set(key K, value V)

	// Delete deletes the value for a key.
	Delete(key K)

	// Range calls f for each key and value.
	// If f returns false, range stops the iteration.
	Range(f func(key K, value V) bool)

	// Len returns the number of items in the map.
	Len() int
}

// mapView implements the View interface
type mapView[K comparable, V any] struct {
	m map[K]V
}

func (mv *mapView[K, V]) Get(key K) (value V, ok bool) {
	value, ok = mv.m[key]
	return
}

func (mv *mapView[K, V]) Set(key K, value V) {
	mv.m[key] = value
}

func (mv *mapView[K, V]) Delete(key K) {
	delete(mv.m, key)
}

func (mv *mapView[K, V]) Range(f func(key K, value V) bool) {
	for k, v := range mv.m {
		if !f(k, v) {
			break
		}
	}
}

func (mv *mapView[K, V]) Len() int {
	return len(mv.m)
}

// WithLock executes the given function while holding a write lock.
// The function receives a View that provides operations on the locked map.
// This is the RECOMMENDED way to perform multiple operations atomically
// as it guarantees the lock will be released even if the function panics.
//
// Example:
//
//	m.WithLock(func(view sync.View[string, int]) {
//		view.Set("key1", value1)
//		view.Set("key2", value2)
//		val, _ := view.Get("key3")
//		view.Set("key4", val * 2)
//	})
func (m *Map[K, V]) WithLock(f func(view View[K, V])) {
	m.mu.Lock()
	defer m.mu.Unlock()
	view := &mapView[K, V]{m: m.m}
	f(view)
}
