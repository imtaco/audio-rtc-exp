package etcd

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/retry"

	"github.com/pkg/errors"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// Heartbeat maintains service presence in etcd by automatically renewing a lease-backed key.
// It stores arbitrary data at a specified key and keeps the key alive by periodically refreshing
// the lease. If the lease expires (e.g., due to network issues), it automatically recreates the
// lease with exponential backoff retry logic.
//
// Example usage:
//
//	type ServiceInfo struct {
//		Addr string `json:"addr"`
//		Port int    `json:"port"`
//	}
//
//	hb := etcd.New(client, "/services/my-service", ServiceInfo{Addr: "127.0.0.1", Port: 8080}, 10*time.Second, logger)
//	if err := hb.Start(); err != nil {
//		logger.Fatal(err)
//	}
//	defer hb.Cleanup()
//
//	// The key will remain in etcd as long as the heartbeat is running
//	// If this process dies, the key will be removed after TTL expires
type Heartbeat[T any] struct {
	client      *clientv3.Client
	key         string
	data        T
	ttl         time.Duration
	leaseID     clientv3.LeaseID
	keepAliveCh <-chan *clientv3.LeaseKeepAliveResponse
	cancel      context.CancelFunc
	logger      *log.Logger
}

func New[T any](client *clientv3.Client, key string, data T, ttl time.Duration, logger *log.Logger) *Heartbeat[T] {
	if ttl <= 0 {
		panic("TTL must be greater than 0")
	}
	return &Heartbeat[T]{
		client: client,
		key:    key,
		data:   data,
		ttl:    ttl,
		logger: logger,
	}
}

func (h *Heartbeat[T]) Start(ctx context.Context) error {
	ctx, h.cancel = context.WithCancel(ctx)

	if err := h.setup(ctx); err != nil {
		return err
	}
	h.logger.Info("Heartbeat started",
		log.String("key", h.key),
		log.Duration("ttl", h.ttl))

	go h.monitorKeepAlive(ctx)
	return nil
}

func (h *Heartbeat[T]) Stop(ctx context.Context) error {
	if h.cancel != nil {
		h.cancel()
	}
	if h.leaseID == 0 {
		return nil
	}

	h.logger.Debug("Revoking heartbeat lease...")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := h.client.Revoke(ctx, h.leaseID)
	if err != nil {
		return errors.Wrap(err, "fail to revoking heartbeat lease")
	}
	h.logger.Debug("Heartbeat lease revoked successfully")

	return nil
}

func (h *Heartbeat[T]) setup(ctx context.Context) error {
	h.logger.Debug("Creating heartbeat lease")

	leaseResp, err := h.client.Grant(ctx, int64(h.ttl.Seconds()))
	if err != nil {
		return errors.Wrapf(err, "fail to create lease key: %s", h.key)
	}
	h.leaseID = leaseResp.ID

	jsonData, err := json.Marshal(h.data)
	if err != nil {
		return errors.Wrap(err, "fail to marshal data")
	}

	_, err = h.client.Put(ctx, h.key, string(jsonData), clientv3.WithLease(h.leaseID))
	if err != nil {
		return errors.Wrapf(err, "fail to put key: %s", h.key)
	}

	// Start automatic keep-alive
	keepAliveCh, err := h.client.KeepAlive(ctx, h.leaseID)
	if err != nil {
		return errors.Wrapf(err, "fail to start keep-alive for key: %s", h.key)
	}
	h.keepAliveCh = keepAliveCh
	return nil
}

func (h *Heartbeat[T]) monitorKeepAlive(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case resp, ok := <-h.keepAliveCh:
			if !ok || resp == nil {
				h.logger.Warn("Keep-alive channel closed or response is nil, lease may have expired",
					log.String("key", h.key))
				// Channel closed, need to recreate lease
				_ = h.recreateLease(ctx)
				continue
			}
			h.logger.Debug("Lease kept alive",
				log.String("key", h.key),
				log.Int64("ttl", resp.TTL))
		}
	}
}

func (h *Heartbeat[T]) recreateLease(ctx context.Context) error {
	operation := func() error {
		select {
		case <-ctx.Done():
			return backoff.Permanent(ctx.Err())
		default:
		}

		h.logger.Info("Attempting to recreate lease", log.String("key", h.key))

		if err := h.setup(ctx); err != nil {
			return errors.Wrapf(err, "fail to recreate lease for key: %s", h.key)
		}

		h.logger.Info("Successfully recreated lease", log.String("key", h.key))
		return nil
	}

	b := retry.New(h.logger, 100*time.Millisecond, 10*time.Second, 0)
	return b.Do(ctx, operation)
}
