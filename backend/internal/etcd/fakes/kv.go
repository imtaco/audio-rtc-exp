package fakes

import (
	"context"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdKV is a minimal fake for testing that ignores all calls
type EtcdKV struct{}

func (f *EtcdKV) Get(_ context.Context, _ string, _ ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	return &clientv3.GetResponse{}, nil
}

func (f *EtcdKV) Put(_ context.Context, _, _ string, _ ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	return &clientv3.PutResponse{}, nil
}

func (f *EtcdKV) Delete(_ context.Context, _ string, _ ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	return &clientv3.DeleteResponse{}, nil
}
