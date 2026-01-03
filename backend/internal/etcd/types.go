package etcd

import (
	"context"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type Client interface {
	KV
	Watcher
	Lease
}

// KV is the interface for etcd operations needed by RoomWatcher
type KV interface {
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
	Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error)
	Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error)
}

// Watcher is an interface that wraps the etcd client methods used by the watcher
type Watcher interface {
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
	Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan
}

// Lease is the interface for etcd lease operations
type Lease interface {
	Grant(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error)
}
