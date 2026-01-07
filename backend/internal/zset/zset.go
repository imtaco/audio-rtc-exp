//nolint:forcetypeassert
package zset

import (
	"container/heap"
	"time"
)

var (
	zeroTime = time.Time{}
)

func New[T any]() *Zset[T] {
	return &Zset[T]{
		items: make(map[string]*item[T]),
	}
}

// Zset is a simple time-based sorted set like Redis ZSET
// Please note that this is NOT thread-safe
type Zset[T any] struct {
	pq    priorityQueue[T]
	items map[string]*item[T]
}

type Entry[T any] struct {
	Key  string
	Data T
	TS   time.Time
}

func (z *Zset[T]) Len() int {
	return len(z.items)
}

func (z *Zset[T]) Put(key string, data T, ts time.Time) {
	z.Remove(key)

	item := &item[T]{
		key:  key,
		data: data,
		ts:   ts,
	}
	z.items[key] = item
	heap.Push(&z.pq, item)
}

func (z *Zset[T]) Remove(key string) {
	item, ok := z.items[key]
	if ok {
		heap.Remove(&z.pq, item.index)
		delete(z.items, key)
	}
}

func (z *Zset[T]) PopBefore(t time.Time, maxItems int) []Entry[T] {
	result := make([]Entry[T], 0, maxItems)
	for len(z.pq) > 0 && len(result) < maxItems {
		if z.pq[0].ts.After(t) {
			break
		}
		item := heap.Pop(&z.pq).(*item[T])
		result = append(result, Entry[T]{
			Key:  item.key,
			Data: item.data,
			TS:   item.ts,
		})
		delete(z.items, item.key)
	}

	return result
}

func (z *Zset[T]) Pop() (string, T, time.Time, bool) {
	if len(z.pq) == 0 {
		var zero T
		return "", zero, zeroTime, false
	}

	item := heap.Pop(&z.pq).(*item[T])
	delete(z.items, item.key)

	return item.key, item.data, item.ts, true
}

func (z *Zset[T]) Peek() (string, T, time.Time, bool) {
	if len(z.pq) == 0 {
		var zero T
		return "", zero, zeroTime, false
	}

	item := z.pq[0]
	return item.key, item.data, item.ts, true
}
