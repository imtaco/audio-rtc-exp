package fakes

import (
	"context"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdKV is a minimal fake for testing that ignores all calls
type EtcdKV struct{}

func (f *EtcdKV) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	return &clientv3.GetResponse{}, nil
}

func (f *EtcdKV) Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	return &clientv3.PutResponse{}, nil
}

func (f *EtcdKV) Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	return &clientv3.DeleteResponse{}, nil
}
