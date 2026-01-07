//nolint:forcetypeassert
package scheduler

import (
	"container/heap"
	"context"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

// KeyedScheduler is a priority-based scheduler that manages delayed execution of items
// identified by unique string keys. It uses a min-heap to efficiently schedule the next
// item to fire based on timestamp. If multiple schedule requests are made for the same key,
// only the earliest timestamp is kept.
//
// Example usage:
//
//	scheduler := NewKeyedScheduler[any](logger)
//	defer scheduler.Shutdown()
//
//	// Schedule tasks with delays
//	scheduler.Enqueue("task1", 5*time.Second)
//	scheduler.Enqueue("task2", 3*time.Second)
//
//	// Listen for fired tasks
//	for key := range scheduler.Chan() {
//		fmt.Printf("Task fired: %s\n", key)
//	}
//
// Scheduling the same key multiple times keeps only the earliest:
//
//	scheduler.Enqueue("retry", 10*time.Second)
//	scheduler.Enqueue("retry", 5*time.Second)  // This one will be used
//	scheduler.Enqueue("retry", 15*time.Second) // Ignored, later than 5s
type KeyedScheduler struct {
	items       map[string]*item
	heap        priorityQueue
	chSig       chan string
	chanEnqueue chan func()
	timer       clockwork.Timer
	timerTS     time.Time
	ctx         context.Context
	cancel      context.CancelFunc
	clock       clockwork.Clock
	logger      *log.Logger
}

func NewKeyedScheduler(logger *log.Logger) *KeyedScheduler {
	return newKeyedSchedulerWithClock(logger, clockwork.NewRealClock())
}

func newKeyedSchedulerWithClock(logger *log.Logger, clock clockwork.Clock) *KeyedScheduler {
	if logger == nil {
		panic("logger is required")
	}
	if clock == nil {
		panic("clock is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	ks := &KeyedScheduler{
		chSig:       make(chan string),
		items:       make(map[string]*item),
		heap:        make(priorityQueue, 0),
		chanEnqueue: make(chan func(), 100),
		timer:       clock.NewTimer(time.Second),
		ctx:         ctx,
		cancel:      cancel,
		clock:       clock,
		logger:      logger,
	}
	heap.Init(&ks.heap)

	go ks.loop()
	return ks
}

func (ks *KeyedScheduler) Chan() <-chan string {
	return ks.chSig
}

func (ks *KeyedScheduler) Enqueue(key string, delay time.Duration) {
	ts := ks.clock.Now().Add(delay)
	ks.chanEnqueue <- func() {
		ks.doEnqueue(&item{key: key, ts: ts})
	}
}

func (ks *KeyedScheduler) doEnqueue(item *item) {
	curItem, ok := ks.items[item.key]
	if ok {
		// late events
		if item.ts.After(curItem.ts) || item.ts.Equal(curItem.ts) {
			return
		}

		// remove orignal item
		heap.Remove(&ks.heap, curItem.index)
	}

	ks.items[item.key] = item
	heap.Push(&ks.heap, item)
	ks.scheduleNextTimer()
}

func (ks *KeyedScheduler) Cancel(key string) {
	ks.chanEnqueue <- func() {
		ks.doCancel(key)
	}
}

func (ks *KeyedScheduler) doCancel(key string) {
	if item, exists := ks.items[key]; exists {
		delete(ks.items, key)
		heap.Remove(&ks.heap, item.index)
		ks.scheduleNextTimer()
	}
}

func (ks *KeyedScheduler) Clear() {
	ks.chanEnqueue <- func() {
		ks.doClear()
	}
}

func (ks *KeyedScheduler) doClear() {
	ks.items = make(map[string]*item)
	ks.heap = make(priorityQueue, 0)
	heap.Init(&ks.heap)
	ks.clearTimer()
}

func (ks *KeyedScheduler) Shutdown() {
	// TODO: more cleanup if needed ?!
	ks.cancel()
	ks.clearTimer()
}

func (ks *KeyedScheduler) clearTimer() {
	ks.timer.Stop()
	ks.timerTS = time.Time{}
}

func (ks *KeyedScheduler) scheduleNextTimer() {
	if len(ks.items) == 0 {
		ks.clearTimer()
		return
	}

	top := ks.heap[0]
	// the same due, no need to reschedule
	if ks.timerTS.Equal(top.ts) {
		return
	}

	delay := top.ts.Sub(ks.clock.Now())
	if delay < 0 {
		delay = 0
	}

	ks.timerTS = top.ts
	ks.timer.Stop()
	ks.timer.Reset(delay)
}

func (ks *KeyedScheduler) loop() {
	for {
		select {
		case <-ks.ctx.Done():
			close(ks.chSig)
			return
		case action, ok := <-ks.chanEnqueue:
			if !ok {
				return
			}
			action()
		case <-ks.timer.Chan():
			ks.clearTimer()
			ks.fireDue()
		}
	}
}

func (ks *KeyedScheduler) popTop() *item {
	top := heap.Pop(&ks.heap).(*item)
	delete(ks.items, top.key)
	return top
}

func (ks *KeyedScheduler) fireDue() {
	now := ks.clock.Now()

	for len(ks.items) > 0 {
		select {
		case <-ks.ctx.Done():
			return
		default:
		}

		if ks.heap[0].ts.After(now) {
			break
		}

		item := ks.popTop()
		ks.chSig <- item.key
	}

	ks.scheduleNextTimer()
}
