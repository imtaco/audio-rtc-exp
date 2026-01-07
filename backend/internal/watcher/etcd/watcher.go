package etcd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/imtaco/audio-rtc-exp/internal/log"

	"github.com/imtaco/audio-rtc-exp/internal/etcd"
	"github.com/imtaco/audio-rtc-exp/internal/scheduler"
	"github.com/imtaco/audio-rtc-exp/internal/sync"
	"github.com/imtaco/audio-rtc-exp/internal/watcher"
)

// BaseEtcdWatcher watches etcd keys with a specified prefix and maintains an in-memory cache
// of the current state. It handles initial data fetching, continuous watching for changes, and
// automatic recovery from connection failures. Changes are processed through a scheduler with
// retry logic using exponential backoff.
//
// Example usage:
//
//	type MyData struct {
//		Value string `json:"value"`
//	}
//
//	watcher := etcd.New(etcd.Config[MyData]{
//		Client:        etcdClient,
//		PrefixToWatch: "/my/prefix/",
//		AllowedKeyTypes: []string{"type1", "type2"},
//		Logger:        logger,
//		ProcessChange: func(id string) error {
//			// Handle state change for this ID
//			return nil
//		},
//		StateTransformer: myTransformer,
//	})
//
//	if err := watcher.Initialize(ctx); err != nil {
//		logger.Fatal(err)
//	}
//	defer watcher.Cleanup()
//
// Key format expected: {prefix}{id}/{keyType}
// Example: /my/prefix/server-1/heartbeat
type BaseEtcdWatcher[T any] struct {
	client          etcd.Watcher
	prefixToWatch   string
	allowedKeyTypes []string
	scheduler       *scheduler.KeyedScheduler

	cache     *sync.Map[string, *T]
	cancel    context.CancelFunc
	gawCancel context.CancelFunc // getAndWatchOnce cancel func for force restart
	initGetCh chan struct{}
	stoppedCh chan struct{}

	processChange watcher.ProcessChangeFunc[T]
	stateTrans    watcher.StateTransformer[T]
	retryAttampts map[string]int
	retryDelay    time.Duration // configurable retry delay for testing

	logger *log.Logger
}

type Config[T any] struct {
	Client           etcd.Watcher
	PrefixToWatch    string
	AllowedKeyTypes  []string
	Logger           *log.Logger
	ProcessChange    watcher.ProcessChangeFunc[T]
	StateTransformer watcher.StateTransformer[T]
}

// NewWithEtcdClient creates a new watcher with a real etcd client
func NewWithEtcdClient[T any](client *clientv3.Client, cfg Config[T]) watcher.Watcher[T] {
	cfg.Client = client
	return New(cfg)
}

func New[T any](cfg Config[T]) watcher.Watcher[T] {
	return &BaseEtcdWatcher[T]{
		client:          cfg.Client,
		prefixToWatch:   cfg.PrefixToWatch,
		allowedKeyTypes: cfg.AllowedKeyTypes,
		cache:           sync.NewMap[string, *T](),
		processChange:   cfg.ProcessChange,
		stateTrans:      cfg.StateTransformer,
		initGetCh:       make(chan struct{}),
		retryDelay:      time.Second, // default retry delay
		logger:          cfg.Logger,
	}
}

func (w *BaseEtcdWatcher[T]) Start(ctx context.Context) error {
	w.logger.Info("Initializing etcd watcher...")

	// timeout for initial get
	ctxInit, cancelInit := context.WithTimeout(ctx, 30*time.Second)
	defer cancelInit()

	// create cancellable context for whole watcher
	ctx, w.cancel = context.WithCancel(ctx)
	w.stoppedCh = make(chan struct{})

	go w.getAndWatch(ctx)

	// Wait for initial get to complete or timeout
	select {
	case <-ctxInit.Done():
		return ctx.Err()
	case <-w.initGetCh:
	}

	w.logger.Info("Etcd watcher initialization complete")
	return nil
}

func (w *BaseEtcdWatcher[T]) Stop() error {
	w.scheduler.Shutdown()
	if w.cancel != nil {
		w.cancel()
	}
	if w.stoppedCh != nil {
		<-w.stoppedCh
	}
	return nil
}

func (w *BaseEtcdWatcher[T]) Restart() {
	if w.gawCancel != nil {
		w.gawCancel()
	}
}

func (w *BaseEtcdWatcher[T]) GetCachedState(id string) (*T, bool) {
	return w.cache.Load(id)
}

func (w *BaseEtcdWatcher[T]) rebuild(ctx context.Context) error {
	if err := w.stateTrans.RebuildStart(ctx); err != nil {
		return err
	}
	var rebuildErr error
	w.cache.Range(func(id string, etcdData *T) bool {
		if err := w.stateTrans.RebuildState(ctx, id, etcdData); err != nil {
			rebuildErr = err
			return false
		}
		return true
	})
	if rebuildErr != nil {
		return rebuildErr
	}
	if err := w.stateTrans.RebuildEnd(ctx); err != nil {
		return err
	}
	return nil
}

func (w *BaseEtcdWatcher[T]) updateCache(id string, state *T) {
	if state == nil {
		w.cache.Delete(id)
	} else {
		w.cache.Store(id, state)
	}
}

func (w *BaseEtcdWatcher[T]) parseKey(key string) (id, keyType string, ok bool) {
	if !strings.HasPrefix(key, w.prefixToWatch) {
		return "", "", false
	}

	remaining := strings.TrimPrefix(key, w.prefixToWatch)
	parts := strings.Split(remaining, "/")
	if len(parts) != 2 {
		return "", "", false
	}

	return parts[0], parts[1], true
}

func (w *BaseEtcdWatcher[T]) parseAndUpdateCache(key string, value []byte) (id, keyType string, ok bool) {
	id, keyType, ok = w.parseKey(key)
	if !ok {
		return "", "", false
	}

	if len(w.allowedKeyTypes) > 0 {
		allowed := false
		for _, kt := range w.allowedKeyTypes {
			if kt == keyType {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", "", false
		}
	}

	curState, _ := w.cache.Load(id)
	newState, err := w.stateTrans.NewState(id, keyType, value, curState)
	if err != nil {
		w.logger.Error("Error updating cache", log.String("key", key), log.Error(err))
		return "", "", false
	}
	w.updateCache(id, newState)

	return id, keyType, true
}

func (w *BaseEtcdWatcher[T]) getAndWatch(ctx context.Context) {
	first := true

	// user new scheduler
	if w.scheduler != nil {
		w.scheduler.Shutdown()
	}

	w.scheduler = scheduler.NewKeyedScheduler(w.logger)
	defer close(w.stoppedCh)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// On first iteration, pass the initGetCh to notify when done
		var ch chan struct{}
		if first {
			ch = w.initGetCh
			first = false
		}

		gawCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		w.gawCancel = cancel

		if err := w.getAndWatchOnce(gawCtx, ch); err != nil {
			if !errors.Is(err, context.Canceled) {
				w.logger.Error("Error in getAndWatch loop", log.Error(err))
				time.Sleep(w.retryDelay)
				continue
			}

			select {
			case <-ctx.Done():
				w.logger.Info("Etcd context canceled, exiting getAndWatch loop")
				return
			default:
				// only gawCtx was canceled, restart the loop
				w.logger.Info("Etcd getAndWatch canceled, restarting watcher")
			}
		}
	}
}

func (w *BaseEtcdWatcher[T]) getAndWatchOnce(ctx context.Context, getNotify chan struct{}) error {
	w.logger.Info("Getting current data and starting watcher...")

	// clear retry attempts on each full restart
	w.retryAttampts = make(map[string]int)

	resp, err := w.client.Get(ctx, w.prefixToWatch, clientv3.WithPrefix())
	if err != nil {
		return err
	}

	revision := resp.Header.Revision
	w.logger.Info("Got etcd revision", log.Int64("revision", revision))

	kvs := resp.Kvs
	w.logger.Info("Found keys in etcd, rebuilding state...", log.Int("count", len(kvs)))

	idsToProcess := make(map[string]struct{})

	for _, kv := range kvs {
		key := string(kv.Key)
		value := kv.Value
		id, _, ok := w.parseAndUpdateCache(key, value)
		if ok {
			idsToProcess[id] = struct{}{}
		}
	}

	// cacheSize := len(w.cache)
	w.logger.Info("Rebuilt state from etcd")

	w.logger.Info("Running rebuild hook...")
	if err := w.rebuild(ctx); err != nil {
		w.logger.Error("Error in rebuild hook", log.Error(err))
		return err
	}

	// Notify that initial get is done
	if getNotify != nil {
		close(getNotify)
	}

	for id := range idsToProcess {
		w.scheduler.Enqueue(id, 0)
	}

	// need to get from last revision
	nextRev := revision + 1
	w.logger.Info("Starting etcd watcher from revision", log.Int64("revision", nextRev))

	watchChan := w.client.Watch(ctx, w.prefixToWatch,
		clientv3.WithPrefix(),
		clientv3.WithRev(nextRev))

	w.logger.Info("Etcd watcher started successfully")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case key := <-w.scheduler.Chan():
			state, _ := w.GetCachedState(key)
			if err := w.processChange(ctx, key, state); err != nil {
				w.logger.Error("Error processing change for key", log.String("key", key), log.Error(err))
				// re-enqueue
				retryCount := w.retryAttampts[key]
				w.scheduler.Enqueue(key, nextDelay(retryCount))
				w.retryAttampts[key] = retryCount + 1
			} else {
				delete(w.retryAttampts, key)
			}
		case watchResp := <-watchChan:
			if watchResp.Err() != nil {
				w.logger.Error("Etcd watcher error", log.Error(watchResp.Err()))
				return watchResp.Err()
			}

			w.handleWatch(watchResp)
		}
	}
}

func (w *BaseEtcdWatcher[T]) handleWatch(watchResp clientv3.WatchResponse) {
	for _, event := range watchResp.Events {
		key := string(event.Kv.Key)

		switch event.Type {
		case clientv3.EventTypePut:
			value := event.Kv.Value
			id, _, ok := w.parseAndUpdateCache(key, value)
			if !ok {
				continue
			}

			var data T
			if err := json.Unmarshal(value, &data); err != nil {
				// TODO: handle error properly, not ok to ignore it
				w.logger.Error("Error unmarshaling data for logging",
					log.String("key", key),
					log.Error(err))
				continue
			}
			w.logger.Info("Key updated",
				log.String("key", key),
				log.Any("value", data))

			w.scheduler.Enqueue(id, 0)
			// new attempt, reset counter
			delete(w.retryAttampts, id)

		case clientv3.EventTypeDelete:
			id, _, ok := w.parseAndUpdateCache(key, nil)
			if ok {
				w.logger.Info("Key deleted", log.String("key", key))
				w.scheduler.Enqueue(id, 0)
				// new attempt, reset counter
				delete(w.retryAttampts, id)
			}
		}
	}
}

func nextDelay(attempt int) time.Duration {
	// Exponential backoff with jitter
	baseDelay := time.Duration(100*(1<<attempt)) * time.Millisecond
	if baseDelay > 10*time.Second {
		baseDelay = 10 * time.Second
	}
	return baseDelay
}
