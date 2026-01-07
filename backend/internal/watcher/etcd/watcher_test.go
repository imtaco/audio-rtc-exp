package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.etcd.io/etcd/api/v3/etcdserverpb"
	mvccpb "go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/mock/gomock"

	etcdmock "github.com/imtaco/audio-rtc-exp/internal/etcd/mocks"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/scheduler"
	"github.com/imtaco/audio-rtc-exp/internal/watcher"
	"github.com/imtaco/audio-rtc-exp/internal/watcher/mocks"
)

type TestData struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type WatcherTestSuite struct {
	suite.Suite
}

func (s *WatcherTestSuite) newWatcher(mockTrans watcher.StateTransformer[TestData]) *BaseEtcdWatcher[TestData] {
	logger := log.NewTest(s.T())
	w := New(Config[TestData]{
		Client:           nil, // Will be replaced in tests that need it
		PrefixToWatch:    "/test/prefix/",
		AllowedKeyTypes:  []string{"data", "config"},
		Logger:           logger,
		ProcessChange:    func(_ context.Context, _ string, _ *TestData) error { return nil },
		StateTransformer: mockTrans,
	})
	return w.(*BaseEtcdWatcher[TestData])
}

func (s *WatcherTestSuite) newWatcherWithClient(mockClient *etcdmock.MockWatcher, mockTrans watcher.StateTransformer[TestData]) *BaseEtcdWatcher[TestData] {
	logger := log.NewTest(s.T())
	w := New(Config[TestData]{
		Client:           mockClient,
		PrefixToWatch:    "/test/prefix/",
		AllowedKeyTypes:  []string{"data", "config"},
		Logger:           logger,
		ProcessChange:    func(_ context.Context, _ string, _ *TestData) error { return nil },
		StateTransformer: mockTrans,
	})
	return w.(*BaseEtcdWatcher[TestData])
}

func (s *WatcherTestSuite) TestStart_Success() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	mockClient := etcdmock.NewMockWatcher(ctrl)
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcherWithClient(mockClient, mockTrans)

	// Mock Get response
	getResponse := &clientv3.GetResponse{
		Header: &etcdserverpb.ResponseHeader{Revision: 100},
		Kvs:    []*mvccpb.KeyValue{},
	}
	mockClient.EXPECT().
		Get(gomock.Any(), "/test/prefix/", gomock.Any()).
		Return(getResponse, nil)

	// Mock Watch response
	watchCh := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().
		Watch(gomock.Any(), "/test/prefix/", gomock.Any(), gomock.Any()).
		Return((clientv3.WatchChan)(watchCh))

	// Mock Rebuild
	mockTrans.EXPECT().RebuildStart(gomock.Any()).Return(nil)
	mockTrans.EXPECT().RebuildEnd(gomock.Any()).Return(nil)

	err := watcher.Start(context.Background())
	s.Require().NoError(err)

	// cleanup
	_ = watcher.Stop()
	close(watchCh)
}

func (s *WatcherTestSuite) TestStart_GetError() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	mockClient := etcdmock.NewMockWatcher(ctrl)
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcherWithClient(mockClient, mockTrans)

	// Mock Get error
	mockClient.EXPECT().
		Get(gomock.Any(), "/test/prefix/", gomock.Any()).
		Return(nil, fmt.Errorf("etcd error"))

	// Start spawns a goroutine that calls Get.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := watcher.Start(ctx)
	// Start returns ctx.Err() (deadline exceeded)
	s.Require().Error(err)
}

func (s *WatcherTestSuite) TestStart_RebuildError() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	mockClient := etcdmock.NewMockWatcher(ctrl)
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcherWithClient(mockClient, mockTrans)

	getResponse := &clientv3.GetResponse{
		Header: &etcdserverpb.ResponseHeader{Revision: 100},
		Kvs:    []*mvccpb.KeyValue{},
	}
	mockClient.EXPECT().
		Get(gomock.Any(), "/test/prefix/", gomock.Any()).
		Return(getResponse, nil).AnyTimes() // Might be called multiple times due to retry loop

	mockTrans.EXPECT().RebuildStart(gomock.Any()).Return(fmt.Errorf("rebuild error")).AnyTimes()

	// Similar to GetError, if rebuild fails, initGetCh is not closed.
	// So Start should timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := watcher.Start(ctx)
	s.Require().Error(err)
}

func (s *WatcherTestSuite) TestRunLoop_WatchEvents() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	mockClient := etcdmock.NewMockWatcher(ctrl)
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcherWithClient(mockClient, mockTrans)

	// Setup Get success
	getResponse := &clientv3.GetResponse{
		Header: &etcdserverpb.ResponseHeader{Revision: 100},
		Kvs:    []*mvccpb.KeyValue{},
	}
	mockClient.EXPECT().
		Get(gomock.Any(), "/test/prefix/", gomock.Any()).
		Return(getResponse, nil)

	mockTrans.EXPECT().RebuildStart(gomock.Any()).Return(nil)
	mockTrans.EXPECT().RebuildEnd(gomock.Any()).Return(nil)

	// Setup Watch
	watchCh := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().
		Watch(gomock.Any(), "/test/prefix/", gomock.Any(), gomock.Any()).
		Return((clientv3.WatchChan)(watchCh))

	// Setup expectations for event processing
	data := &TestData{Value: "new", Count: 1}
	jsonData, _ := json.Marshal(data)

	// Use a channel to signal when NewState is called
	stateUpdated := make(chan struct{})
	mockTrans.EXPECT().
		NewState("server1", "data", jsonData, gomock.Any()).
		DoAndReturn(func(_, _ string, _ []byte, _ *TestData) (*TestData, error) {
			defer close(stateUpdated)
			return data, nil
		})

	s.Require().NoError(watcher.Start(context.Background()))
	defer func() { _ = watcher.Stop() }()

	// Send event
	watchCh <- clientv3.WatchResponse{
		Events: []*clientv3.Event{
			{
				Type: clientv3.EventTypePut,
				Kv: &mvccpb.KeyValue{
					Key:   []byte("/test/prefix/server1/data"),
					Value: jsonData,
				},
			},
		},
	}

	// Wait for state to be updated
	<-stateUpdated

	// Check if cache updated
	state, ok := watcher.GetCachedState("server1")
	s.True(ok)
	s.Equal(data, state)
}

func (s *WatcherTestSuite) TestRunLoop_SchedulerEvents() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	mockClient := etcdmock.NewMockWatcher(ctrl)
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcherWithClient(mockClient, mockTrans)

	// Setup Get success
	getResponse := &clientv3.GetResponse{
		Header: &etcdserverpb.ResponseHeader{Revision: 100},
		Kvs:    []*mvccpb.KeyValue{},
	}
	mockClient.EXPECT().
		Get(gomock.Any(), "/test/prefix/", gomock.Any()).
		Return(getResponse, nil)

	mockTrans.EXPECT().RebuildStart(gomock.Any()).Return(nil)
	mockTrans.EXPECT().RebuildEnd(gomock.Any()).Return(nil)

	// Setup Watch
	watchCh := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().
		Watch(gomock.Any(), "/test/prefix/", gomock.Any(), gomock.Any()).
		Return((clientv3.WatchChan)(watchCh))

	// Manually inject a state into cache
	data := &TestData{Value: "scheduled", Count: 1}
	watcher.cache.Store("server1", data)

	mockTrans.EXPECT().RebuildState(gomock.Any(), "server1", data).Return(nil)

	// Setup ProcessChange expectation
	// We need to override the ProcessChange function in the watcher
	processed := make(chan struct{})
	watcher.processChange = func(_ context.Context, id string, state *TestData) error {
		if id == "server1" && state == data {
			close(processed)
		}
		return nil
	}

	s.Require().NoError(watcher.Start(context.Background()))
	defer func() { _ = watcher.Stop() }()

	// Enqueue item to scheduler manually (simulating internal trigger)
	watcher.scheduler.Enqueue("server1", 0)

	// Wait for processing (no timeout needed - test will hang if broken)
	<-processed
}

func (s *WatcherTestSuite) TestRunLoop_WatchError() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	mockClient := etcdmock.NewMockWatcher(ctrl)
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcherWithClient(mockClient, mockTrans)

	// Override retry delay to make test faster
	watcher.retryDelay = time.Millisecond

	// Setup Get success
	getResponse := &clientv3.GetResponse{
		Header: &etcdserverpb.ResponseHeader{Revision: 100},
		Kvs:    []*mvccpb.KeyValue{},
	}
	mockClient.EXPECT().
		Get(gomock.Any(), "/test/prefix/", gomock.Any()).
		Return(getResponse, nil).Times(2) // Initial + Retry

	mockTrans.EXPECT().RebuildStart(gomock.Any()).Return(nil).Times(2)
	mockTrans.EXPECT().RebuildEnd(gomock.Any()).Return(nil).Times(2)

	// Setup Watch - First one fails immediately via channel error
	watchCh1 := make(chan clientv3.WatchResponse)
	// Second watch succeeds
	watchCh2 := make(chan clientv3.WatchResponse)

	gomock.InOrder(
		mockClient.EXPECT().
			Watch(gomock.Any(), "/test/prefix/", gomock.Any(), gomock.Any()).
			Return((clientv3.WatchChan)(watchCh1)),
		mockClient.EXPECT().
			Watch(gomock.Any(), "/test/prefix/", gomock.Any(), gomock.Any()).
			Return((clientv3.WatchChan)(watchCh2)),
	)

	s.Require().NoError(watcher.Start(context.Background()))
	defer func() { _ = watcher.Stop() }()

	// Send error on first watch channel by setting CompactRevision
	// This makes Err() return ErrCompacted
	watchCh1 <- clientv3.WatchResponse{
		CompactRevision: 123,
	}

	// Prepare event data for second watch channel
	data := &TestData{Value: "retry", Count: 2}
	jsonData, _ := json.Marshal(data)

	// Use channel to signal when NewState is called
	stateUpdated := make(chan struct{})
	mockTrans.EXPECT().
		NewState("server1", "data", jsonData, gomock.Any()).
		DoAndReturn(func(_, _ string, _ []byte, _ *TestData) (*TestData, error) {
			close(stateUpdated)
			return data, nil
		})

	// Send event on watchCh2 - this will block until watcher is ready
	go func() {
		watchCh2 <- clientv3.WatchResponse{
			Events: []*clientv3.Event{
				{
					Type: clientv3.EventTypePut,
					Kv: &mvccpb.KeyValue{
						Key:   []byte("/test/prefix/server1/data"),
						Value: jsonData,
					},
				},
			},
		}
	}()

	// Wait for state to be updated
	<-stateUpdated
}

func (s *WatcherTestSuite) TestNewWithEtcdClient() {
	// Just verify it doesn't panic and returns a watcher
	client := &clientv3.Client{}
	logger := log.NewTest(s.T())
	w := NewWithEtcdClient[TestData](client, Config[TestData]{
		Logger: logger,
	})
	s.NotNil(w)
}

func (s *WatcherTestSuite) TestParseKey_ValidKey() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	id, keyType, ok := watcher.parseKey("/test/prefix/server1/data")

	s.True(ok)
	s.Equal("server1", id)
	s.Equal("data", keyType)
}

func (s *WatcherTestSuite) TestParseKey_InvalidPrefix() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	id, keyType, ok := watcher.parseKey("/wrong/prefix/server1/data")

	s.False(ok)
	s.Equal("", id)
	s.Equal("", keyType)
}

func (s *WatcherTestSuite) TestParseKey_InvalidFormat() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	testCases := []struct {
		name string
		key  string
	}{
		{"missing keyType", "/test/prefix/server1"},
		{"too many parts", "/test/prefix/server1/data/extra"},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			_, _, ok := watcher.parseKey(tc.key)
			s.False(ok)
		})
	}
}

func (s *WatcherTestSuite) TestParseKey_EmptyID() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	id, keyType, ok := watcher.parseKey("/test/prefix//data")

	s.True(ok)
	s.Equal("", id)
	s.Equal("data", keyType)
}

func (s *WatcherTestSuite) TestParseAndUpdateCache_ValidKey() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	testData := &TestData{Value: "test", Count: 42}
	jsonData, _ := json.Marshal(testData)

	mockTrans.EXPECT().
		NewState("server1", "data", jsonData, (*TestData)(nil)).
		Return(testData, nil).
		Times(1)

	id, keyType, ok := watcher.parseAndUpdateCache("/test/prefix/server1/data", jsonData)

	s.True(ok)
	s.Equal("server1", id)
	s.Equal("data", keyType)

	cached, found := watcher.GetCachedState("server1")
	s.True(found)
	s.Equal(testData, cached)
}

func (s *WatcherTestSuite) TestParseAndUpdateCache_DisallowedKeyType() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	testData := &TestData{Value: "test", Count: 42}
	jsonData, _ := json.Marshal(testData)

	id, keyType, ok := watcher.parseAndUpdateCache("/test/prefix/server1/metadata", jsonData)

	s.False(ok)
	s.Equal("", id)
	s.Equal("", keyType)

	_, found := watcher.GetCachedState("server1")
	s.False(found)
}

func (s *WatcherTestSuite) TestParseAndUpdateCache_TransformerError() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	jsonData := []byte(`{"value":"test"}`)

	mockTrans.EXPECT().
		NewState("server1", "data", jsonData, (*TestData)(nil)).
		Return(nil, fmt.Errorf("transformation error")).
		Times(1)

	id, keyType, ok := watcher.parseAndUpdateCache("/test/prefix/server1/data", jsonData)

	s.False(ok)
	s.Equal("", id)
	s.Equal("", keyType)
}

func (s *WatcherTestSuite) TestUpdateCache_AddEntry() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	testData := &TestData{Value: "test", Count: 42}

	watcher.updateCache("server1", testData)

	cached, found := watcher.GetCachedState("server1")
	s.True(found)
	s.Equal(testData, cached)
}

func (s *WatcherTestSuite) TestUpdateCache_UpdateEntry() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	originalData := &TestData{Value: "original", Count: 1}
	watcher.updateCache("server1", originalData)

	updatedData := &TestData{Value: "updated", Count: 2}
	watcher.updateCache("server1", updatedData)

	cached, found := watcher.GetCachedState("server1")
	s.True(found)
	s.Equal(updatedData, cached)
}

func (s *WatcherTestSuite) TestUpdateCache_DeleteEntry() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	testData := &TestData{Value: "test", Count: 42}
	watcher.updateCache("server1", testData)

	_, found := watcher.GetCachedState("server1")
	s.True(found)

	watcher.updateCache("server1", nil)

	_, found = watcher.GetCachedState("server1")
	s.False(found)
}

func (s *WatcherTestSuite) TestRebuild_Success() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	data1 := &TestData{Value: "server1", Count: 1}
	data2 := &TestData{Value: "server2", Count: 2}

	watcher.cache.Store("server1", data1)
	watcher.cache.Store("server2", data2)

	gomock.InOrder(
		mockTrans.EXPECT().RebuildStart(gomock.Any()).Return(nil),
		mockTrans.EXPECT().RebuildState(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2),
		mockTrans.EXPECT().RebuildEnd(gomock.Any()).Return(nil),
	)

	err := watcher.rebuild(context.Background())

	s.Require().NoError(err)
}

func (s *WatcherTestSuite) TestRebuild_StartError() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	mockTrans.EXPECT().
		RebuildStart(gomock.Any()).
		Return(fmt.Errorf("start error")).
		Times(1)

	err := watcher.rebuild(context.Background())

	s.Require().Error(err)
	s.Equal("start error", err.Error())
}

func (s *WatcherTestSuite) TestRebuild_StateError() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	data1 := &TestData{Value: "server1", Count: 1}
	watcher.cache.Store("server1", data1)

	gomock.InOrder(
		mockTrans.EXPECT().RebuildStart(gomock.Any()).Return(nil),
		mockTrans.EXPECT().RebuildState(gomock.Any(), "server1", data1).Return(fmt.Errorf("state error")),
	)

	err := watcher.rebuild(context.Background())

	s.Require().Error(err)
	s.Equal("state error", err.Error())
}

func (s *WatcherTestSuite) TestRebuild_EndError() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	data1 := &TestData{Value: "server1", Count: 1}
	watcher.cache.Store("server1", data1)

	gomock.InOrder(
		mockTrans.EXPECT().RebuildStart(gomock.Any()).Return(nil),
		mockTrans.EXPECT().RebuildState(gomock.Any(), "server1", data1).Return(nil),
		mockTrans.EXPECT().RebuildEnd(gomock.Any()).Return(fmt.Errorf("end error")),
	)

	err := watcher.rebuild(context.Background())

	s.Require().Error(err)
	s.Equal("end error", err.Error())
}

func (s *WatcherTestSuite) TestHandleWatch_PutEvent() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	testData := &TestData{Value: "updated", Count: 100}
	jsonData, _ := json.Marshal(testData)

	mockTrans.EXPECT().
		NewState("server1", "data", jsonData, (*TestData)(nil)).
		Return(testData, nil).
		Times(1)

	event := clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:   []byte("/test/prefix/server1/data"),
			Value: jsonData,
		},
	}

	watchResp := clientv3.WatchResponse{
		Events: []*clientv3.Event{&event},
	}

	logger := log.NewTest(s.T())
	watcher.scheduler = scheduler.NewKeyedScheduler(logger)
	watcher.retryAttampts = make(map[string]int)
	defer watcher.scheduler.Shutdown()

	watcher.handleWatch(watchResp)

	cached, found := watcher.GetCachedState("server1")
	s.True(found)
	s.Equal(testData, cached)
}

func (s *WatcherTestSuite) TestHandleWatch_DeleteEvent() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	existingData := &TestData{Value: "existing", Count: 1}
	watcher.cache.Store("server1", existingData)

	mockTrans.EXPECT().
		NewState("server1", "data", []byte(nil), existingData).
		Return(nil, nil).
		Times(1)

	event := clientv3.Event{
		Type: clientv3.EventTypeDelete,
		Kv: &mvccpb.KeyValue{
			Key: []byte("/test/prefix/server1/data"),
		},
	}

	watchResp := clientv3.WatchResponse{
		Events: []*clientv3.Event{&event},
	}

	logger := log.NewTest(s.T())
	watcher.scheduler = scheduler.NewKeyedScheduler(logger)
	watcher.retryAttampts = make(map[string]int)
	defer watcher.scheduler.Shutdown()

	watcher.handleWatch(watchResp)

	_, found := watcher.GetCachedState("server1")
	s.False(found)
}

func (s *WatcherTestSuite) TestHandleWatch_MultipleEvents() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	data1 := &TestData{Value: "server1", Count: 1}
	data2 := &TestData{Value: "server2", Count: 2}
	jsonData1, _ := json.Marshal(data1)
	jsonData2, _ := json.Marshal(data2)

	mockTrans.EXPECT().
		NewState("server1", "data", jsonData1, (*TestData)(nil)).
		Return(data1, nil).
		Times(1)
	mockTrans.EXPECT().
		NewState("server2", "data", jsonData2, (*TestData)(nil)).
		Return(data2, nil).
		Times(1)

	events := []*clientv3.Event{
		{
			Type: clientv3.EventTypePut,
			Kv: &mvccpb.KeyValue{
				Key:   []byte("/test/prefix/server1/data"),
				Value: jsonData1,
			},
		},
		{
			Type: clientv3.EventTypePut,
			Kv: &mvccpb.KeyValue{
				Key:   []byte("/test/prefix/server2/data"),
				Value: jsonData2,
			},
		},
	}

	watchResp := clientv3.WatchResponse{Events: events}

	logger := log.NewTest(s.T())
	watcher.scheduler = scheduler.NewKeyedScheduler(logger)
	watcher.retryAttampts = make(map[string]int)
	defer watcher.scheduler.Shutdown()

	watcher.handleWatch(watchResp)

	cached1, found1 := watcher.GetCachedState("server1")
	s.True(found1)
	s.Equal(data1, cached1)

	cached2, found2 := watcher.GetCachedState("server2")
	s.True(found2)
	s.Equal(data2, cached2)
}

func (s *WatcherTestSuite) TestHandleWatch_IgnoresInvalidKeys() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	logger := log.NewTest(s.T())
	watcher.scheduler = scheduler.NewKeyedScheduler(logger)
	watcher.retryAttampts = make(map[string]int)
	defer watcher.scheduler.Shutdown()

	events := []*clientv3.Event{
		{
			Type: clientv3.EventTypePut,
			Kv: &mvccpb.KeyValue{
				Key:   []byte("/wrong/prefix/server1/data"),
				Value: []byte(`{"value":"test"}`),
			},
		},
		{
			Type: clientv3.EventTypePut,
			Kv: &mvccpb.KeyValue{
				Key:   []byte("/test/prefix/server1/invalid_keytype"),
				Value: []byte(`{"value":"test"}`),
			},
		},
	}

	watchResp := clientv3.WatchResponse{Events: events}

	watcher.handleWatch(watchResp)

	// Check cache is empty by trying to load any key
	_, found := watcher.GetCachedState("server1")
	s.False(found)
}

func (s *WatcherTestSuite) TestNextDelay_ExponentialBackoff() {
	testCases := []struct {
		attempt     int
		expectedMin time.Duration
		expectedMax time.Duration
		description string
	}{
		{0, 100 * time.Millisecond, 100 * time.Millisecond, "first attempt"},
		{1, 200 * time.Millisecond, 200 * time.Millisecond, "second attempt"},
		{2, 400 * time.Millisecond, 400 * time.Millisecond, "third attempt"},
		{3, 800 * time.Millisecond, 800 * time.Millisecond, "fourth attempt"},
		{10, 10 * time.Second, 10 * time.Second, "capped at max"},
		{20, 10 * time.Second, 10 * time.Second, "stays at max"},
	}

	for _, tc := range testCases {
		s.Run(tc.description, func() {
			delay := nextDelay(tc.attempt)
			s.GreaterOrEqual(delay, tc.expectedMin)
			s.LessOrEqual(delay, tc.expectedMax)
		})
	}
}

func (s *WatcherTestSuite) TestGetCachedState_ExistingEntry() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	testData := &TestData{Value: "test", Count: 42}
	watcher.cache.Store("server1", testData)

	result, found := watcher.GetCachedState("server1")

	s.True(found)
	s.Equal(testData, result)
}

func (s *WatcherTestSuite) TestGetCachedState_NonExistingEntry() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	result, found := watcher.GetCachedState("nonexistent")

	s.False(found)
	s.Nil(result)
}

func (s *WatcherTestSuite) TestParseValue_ValidData() {
	testData := &TestData{Value: "test", Count: 42}
	jsonData, _ := json.Marshal(testData)

	result := ParseValue[TestData](jsonData)

	s.NotNil(result)
	s.Equal("test", result.Value)
	s.Equal(42, result.Count)
}

func (s *WatcherTestSuite) TestParseValue_EmptyData() {
	result := ParseValue[TestData]([]byte{})

	s.Nil(result)
}

func (s *WatcherTestSuite) TestParseValue_InvalidJSON() {
	s.Panics(func() {
		ParseValue[TestData]([]byte("invalid json"))
	})
}

func (s *WatcherTestSuite) TestStop_WithNilScheduler() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	logger := log.NewTest(s.T())
	watcher.scheduler = scheduler.NewKeyedScheduler(logger)

	err := watcher.Stop()
	s.Require().NoError(err)
}

func (s *WatcherTestSuite) TestGetCachedState_MultipleConcurrentReads() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	testData := &TestData{Value: "concurrent", Count: 100}
	watcher.cache.Store("test1", testData)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			result, found := watcher.GetCachedState("test1")
			s.True(found)
			s.Equal(testData, result)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func (s *WatcherTestSuite) TestUpdateCache_NilDeletesEntry() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	testData := &TestData{Value: "toDelete", Count: 1}
	watcher.cache.Store("test1", testData)

	_, found := watcher.GetCachedState("test1")
	s.True(found)

	watcher.updateCache("test1", nil)

	_, found = watcher.GetCachedState("test1")
	s.False(found)
}

func (s *WatcherTestSuite) TestParseAndUpdateCache_UpdateExistingState() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	existingData := &TestData{Value: "old", Count: 1}
	watcher.cache.Store("server1", existingData)

	newData := &TestData{Value: "new", Count: 2}
	jsonData, _ := json.Marshal(newData)

	mockTrans.EXPECT().
		NewState("server1", "data", jsonData, existingData).
		Return(newData, nil).
		Times(1)

	id, keyType, ok := watcher.parseAndUpdateCache("/test/prefix/server1/data", jsonData)

	s.True(ok)
	s.Equal("server1", id)
	s.Equal("data", keyType)

	cached, found := watcher.GetCachedState("server1")
	s.True(found)
	s.Equal(newData, cached)
}

func (s *WatcherTestSuite) TestParseAndUpdateCache_AllowAllKeyTypes() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)

	logger := log.NewTest(s.T())
	w := New(Config[TestData]{
		Client:           nil,
		PrefixToWatch:    "/test/prefix/",
		AllowedKeyTypes:  []string{},
		Logger:           logger,
		ProcessChange:    func(_ context.Context, _ string, _ *TestData) error { return nil },
		StateTransformer: mockTrans,
	})
	watcher := w.(*BaseEtcdWatcher[TestData])

	testData := &TestData{Value: "any", Count: 1}
	jsonData, _ := json.Marshal(testData)

	mockTrans.EXPECT().
		NewState("server1", "anykeytype", jsonData, (*TestData)(nil)).
		Return(testData, nil).
		Times(1)

	id, keyType, ok := watcher.parseAndUpdateCache("/test/prefix/server1/anykeytype", jsonData)

	s.True(ok)
	s.Equal("server1", id)
	s.Equal("anykeytype", keyType)
}

func (s *WatcherTestSuite) TestHandleWatch_InvalidUnmarshal() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	testData := &TestData{Value: "test", Count: 1}
	invalidJSON := []byte(`{"value": 123, "count": "invalid"}`)

	mockTrans.EXPECT().
		NewState("server1", "data", invalidJSON, (*TestData)(nil)).
		Return(testData, nil).
		Times(1)

	event := clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:   []byte("/test/prefix/server1/data"),
			Value: invalidJSON,
		},
	}

	watchResp := clientv3.WatchResponse{
		Events: []*clientv3.Event{&event},
	}

	logger := log.NewTest(s.T())
	watcher.scheduler = scheduler.NewKeyedScheduler(logger)
	watcher.retryAttampts = make(map[string]int)
	defer watcher.scheduler.Shutdown()

	watcher.handleWatch(watchResp)
}

func (s *WatcherTestSuite) TestHandleWatch_DeleteNonExistentEntry() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	mockTrans.EXPECT().
		NewState("server999", "data", []byte(nil), (*TestData)(nil)).
		Return(nil, nil).
		Times(1)

	event := clientv3.Event{
		Type: clientv3.EventTypeDelete,
		Kv: &mvccpb.KeyValue{
			Key: []byte("/test/prefix/server999/data"),
		},
	}

	watchResp := clientv3.WatchResponse{
		Events: []*clientv3.Event{&event},
	}

	logger := log.NewTest(s.T())
	watcher.scheduler = scheduler.NewKeyedScheduler(logger)
	watcher.retryAttampts = make(map[string]int)
	defer watcher.scheduler.Shutdown()

	watcher.handleWatch(watchResp)

	_, found := watcher.GetCachedState("server999")
	s.False(found)
}

func (s *WatcherTestSuite) TestNextDelay_Precision() {
	delay0 := nextDelay(0)
	s.Equal(100*time.Millisecond, delay0)

	delay1 := nextDelay(1)
	s.Equal(200*time.Millisecond, delay1)

	delay2 := nextDelay(2)
	s.Equal(400*time.Millisecond, delay2)

	delay7 := nextDelay(7)
	s.Equal(10*time.Second, delay7)
}

func (s *WatcherTestSuite) TestRebuild_EmptyCache() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	gomock.InOrder(
		mockTrans.EXPECT().RebuildStart(gomock.Any()).Return(nil),
		mockTrans.EXPECT().RebuildEnd(gomock.Any()).Return(nil),
	)

	err := watcher.rebuild(context.Background())

	s.Require().NoError(err)
}

func (s *WatcherTestSuite) TestNew_ConfigInitialization() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)

	logger := log.NewTest(s.T())
	processChange := func(_ context.Context, _ string, _ *TestData) error {
		return nil
	}

	w := New(Config[TestData]{
		Client:           nil,
		PrefixToWatch:    "/custom/prefix/",
		AllowedKeyTypes:  []string{"type1", "type2", "type3"},
		Logger:           logger,
		ProcessChange:    processChange,
		StateTransformer: mockTrans,
	})

	watcher := w.(*BaseEtcdWatcher[TestData])

	s.Equal("/custom/prefix/", watcher.prefixToWatch)
	s.Equal([]string{"type1", "type2", "type3"}, watcher.allowedKeyTypes)
	s.NotNil(watcher.cache)
	s.NotNil(watcher.initGetCh)
	s.Equal(logger, watcher.logger)
}

func (s *WatcherTestSuite) TestStop_WithCancel() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	logger := log.NewTest(s.T())
	watcher.scheduler = scheduler.NewKeyedScheduler(logger)

	ctx, cancel := context.WithCancel(context.Background())
	watcher.cancel = cancel

	err := watcher.Stop()
	s.Require().NoError(err)

	// Context should be canceled immediately
	<-ctx.Done()
	s.Require().Error(ctx.Err())
}

func (s *WatcherTestSuite) TestParseKey_EdgeCases() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	testCases := []struct {
		name     string
		key      string
		expectOk bool
		expectID string
		expectKT string
	}{
		{"exact prefix", "/test/prefix/", false, "", ""},
		{"single slash after", "/test/prefix/server1/", true, "server1", ""},
		{"valid with slash in id", "/test/prefix/server/1/data", false, "", ""},
		{"unicode in id", "/test/prefix/服务器1/data", true, "服务器1", "data"},
		{"unicode in keytype", "/test/prefix/server1/数据", true, "server1", "数据"},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			id, kt, ok := watcher.parseKey(tc.key)
			s.Equal(tc.expectOk, ok, "key: %s", tc.key)
			if tc.expectOk {
				s.Equal(tc.expectID, id)
				s.Equal(tc.expectKT, kt)
			}
		})
	}
}

func (s *WatcherTestSuite) TestHandleWatch_MixedEvents() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	data1 := &TestData{Value: "server1", Count: 1}
	data2 := &TestData{Value: "server2", Count: 2}
	existingData := &TestData{Value: "existing", Count: 99}
	watcher.cache.Store("server3", existingData)

	jsonData1, _ := json.Marshal(data1)
	jsonData2, _ := json.Marshal(data2)

	mockTrans.EXPECT().
		NewState("server1", "data", jsonData1, (*TestData)(nil)).
		Return(data1, nil)
	mockTrans.EXPECT().
		NewState("server2", "data", jsonData2, (*TestData)(nil)).
		Return(data2, nil)
	mockTrans.EXPECT().
		NewState("server3", "data", []byte(nil), existingData).
		Return(nil, nil)

	events := []*clientv3.Event{
		{
			Type: clientv3.EventTypePut,
			Kv: &mvccpb.KeyValue{
				Key:   []byte("/test/prefix/server1/data"),
				Value: jsonData1,
			},
		},
		{
			Type: clientv3.EventTypePut,
			Kv: &mvccpb.KeyValue{
				Key:   []byte("/test/prefix/server2/data"),
				Value: jsonData2,
			},
		},
		{
			Type: clientv3.EventTypeDelete,
			Kv: &mvccpb.KeyValue{
				Key: []byte("/test/prefix/server3/data"),
			},
		},
	}

	watchResp := clientv3.WatchResponse{Events: events}

	logger := log.NewTest(s.T())
	watcher.scheduler = scheduler.NewKeyedScheduler(logger)
	watcher.retryAttampts = make(map[string]int)
	defer watcher.scheduler.Shutdown()

	watcher.handleWatch(watchResp)

	cached1, found1 := watcher.GetCachedState("server1")
	s.True(found1)
	s.Equal(data1, cached1)

	cached2, found2 := watcher.GetCachedState("server2")
	s.True(found2)
	s.Equal(data2, cached2)

	_, found3 := watcher.GetCachedState("server3")
	s.False(found3)
}

func (s *WatcherTestSuite) TestHandleWatch_TransformerReturnsNil() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	jsonData := []byte(`{"value":"test","count":1}`)

	mockTrans.EXPECT().
		NewState("server1", "data", jsonData, (*TestData)(nil)).
		Return(nil, nil)

	event := clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:   []byte("/test/prefix/server1/data"),
			Value: jsonData,
		},
	}

	watchResp := clientv3.WatchResponse{
		Events: []*clientv3.Event{&event},
	}

	logger := log.NewTest(s.T())
	watcher.scheduler = scheduler.NewKeyedScheduler(logger)
	watcher.retryAttampts = make(map[string]int)
	defer watcher.scheduler.Shutdown()

	watcher.handleWatch(watchResp)

	_, found := watcher.GetCachedState("server1")
	s.False(found)
}

func (s *WatcherTestSuite) TestParseAndUpdateCache_NilTransformerResult() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	jsonData := []byte(`{"value":"nil"}`)

	mockTrans.EXPECT().
		NewState("server1", "data", jsonData, (*TestData)(nil)).
		Return(nil, nil)

	id, keyType, ok := watcher.parseAndUpdateCache("/test/prefix/server1/data", jsonData)

	s.True(ok)
	s.Equal("server1", id)
	s.Equal("data", keyType)

	_, found := watcher.GetCachedState("server1")
	s.False(found)
}

func (s *WatcherTestSuite) TestRebuild_MultipleStatesOrdered() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	data1 := &TestData{Value: "server1", Count: 1}
	data2 := &TestData{Value: "server2", Count: 2}
	data3 := &TestData{Value: "server3", Count: 3}

	watcher.cache.Store("server1", data1)
	watcher.cache.Store("server2", data2)
	watcher.cache.Store("server3", data3)

	mockTrans.EXPECT().RebuildStart(gomock.Any()).Return(nil)
	mockTrans.EXPECT().RebuildState(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(3)
	mockTrans.EXPECT().RebuildEnd(gomock.Any()).Return(nil)

	err := watcher.rebuild(context.Background())

	s.Require().NoError(err)
}

func (s *WatcherTestSuite) TestRestart_CancelsCurrentWatch() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	mockClient := etcdmock.NewMockWatcher(ctrl)
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcherWithClient(mockClient, mockTrans)

	getResponse := &clientv3.GetResponse{
		Header: &etcdserverpb.ResponseHeader{Revision: 100},
		Kvs:    []*mvccpb.KeyValue{},
	}

	mockClient.EXPECT().
		Get(gomock.Any(), "/test/prefix/", gomock.Any()).
		Return(getResponse, nil)

	mockTrans.EXPECT().RebuildStart(gomock.Any()).Return(nil)
	mockTrans.EXPECT().RebuildEnd(gomock.Any()).Return(nil)

	watchCh := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().
		Watch(gomock.Any(), "/test/prefix/", gomock.Any(), gomock.Any()).
		Return((clientv3.WatchChan)(watchCh))

	s.Require().NoError(watcher.Start(context.Background()))

	// Verify gawCancel is set after start
	s.NotNil(watcher.gawCancel)

	// Call Restart - should cancel the current watch context
	watcher.Restart()

	// Stop the watcher and wait for it to complete
	_ = watcher.Stop()
}

func (s *WatcherTestSuite) TestRestart_BeforeStart() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcher(mockTrans)

	// Calling Restart before Start should not panic
	s.NotPanics(func() {
		watcher.Restart()
	})
}

func (s *WatcherTestSuite) TestRestart_DoesNotPanicWhenCalledMultipleTimes() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	mockClient := etcdmock.NewMockWatcher(ctrl)
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcherWithClient(mockClient, mockTrans)

	getResponse := &clientv3.GetResponse{
		Header: &etcdserverpb.ResponseHeader{Revision: 100},
		Kvs:    []*mvccpb.KeyValue{},
	}

	mockClient.EXPECT().
		Get(gomock.Any(), "/test/prefix/", gomock.Any()).
		Return(getResponse, nil)

	mockTrans.EXPECT().RebuildStart(gomock.Any()).Return(nil)
	mockTrans.EXPECT().RebuildEnd(gomock.Any()).Return(nil)

	watchCh := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().
		Watch(gomock.Any(), "/test/prefix/", gomock.Any(), gomock.Any()).
		Return((clientv3.WatchChan)(watchCh))

	s.Require().NoError(watcher.Start(context.Background()))
	defer func() { _ = watcher.Stop() }()

	// Call Restart multiple times - should not panic
	s.NotPanics(func() {
		watcher.Restart()
		watcher.Restart()
		watcher.Restart()
	})
}

// TestRestart_TriggersNewGetAndWatchCycle verifies that Restart() actually restarts
// the watcher by triggering a new getAndWatchOnce cycle (Get + Rebuild + Watch).
func (s *WatcherTestSuite) TestRestart_TriggersNewGetAndWatchCycle() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	mockClient := etcdmock.NewMockWatcher(ctrl)
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcherWithClient(mockClient, mockTrans)
	watcher.retryDelay = time.Millisecond

	getResponse := &clientv3.GetResponse{
		Header: &etcdserverpb.ResponseHeader{Revision: 100},
		Kvs:    []*mvccpb.KeyValue{},
	}

	// Get should be called twice: initial + after restart
	getCalled := 0
	firstGetCh := make(chan struct{})
	secondGetCh := make(chan struct{})
	mockClient.EXPECT().
		Get(gomock.Any(), "/test/prefix/", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, _ ...clientv3.OpOption) (*clientv3.GetResponse, error) {
			getCalled++
			switch getCalled {
			case 1:
				close(firstGetCh)
			case 2:
				close(secondGetCh)
			}
			return getResponse, nil
		}).Times(2)

	// RebuildStart should be called twice: initial + after restart
	rebuildStartCalled := 0
	firstRebuildCh := make(chan struct{})
	secondRebuildCh := make(chan struct{})
	mockTrans.EXPECT().RebuildStart(gomock.Any()).DoAndReturn(func(_ context.Context) error {
		rebuildStartCalled++
		switch rebuildStartCalled {
		case 1:
			close(firstRebuildCh)
		case 2:
			close(secondRebuildCh)
		}
		return nil
	}).Times(2)

	mockTrans.EXPECT().RebuildEnd(gomock.Any()).Return(nil).Times(2)

	// Watch should be called twice
	watchCh1 := make(chan clientv3.WatchResponse)
	watchCh2 := make(chan clientv3.WatchResponse)
	watchCalled := 0
	mockClient.EXPECT().
		Watch(gomock.Any(), "/test/prefix/", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, _ ...clientv3.OpOption) clientv3.WatchChan {
			watchCalled++
			if watchCalled == 1 {
				return (clientv3.WatchChan)(watchCh1)
			}
			return (clientv3.WatchChan)(watchCh2)
		}).Times(2)

	s.Require().NoError(watcher.Start(context.Background()))
	defer func() { _ = watcher.Stop() }()

	// Wait for initial setup to complete
	<-firstGetCh
	<-firstRebuildCh
	s.Equal(1, getCalled)
	s.Equal(1, rebuildStartCalled)

	// Trigger restart - should trigger a new getAndWatchOnce cycle
	watcher.Restart()

	// Wait for second cycle to complete
	<-secondGetCh
	<-secondRebuildCh

	// Verify Get and Rebuild were called again after restart
	s.Equal(2, getCalled, "Get should be called twice (initial + restart)")
	s.Equal(2, rebuildStartCalled, "RebuildStart should be called twice (initial + restart)")
}

func (s *WatcherTestSuite) TestRestart_AfterStop() {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	mockClient := etcdmock.NewMockWatcher(ctrl)
	mockTrans := mocks.NewMockStateTransformer[TestData](ctrl)
	watcher := s.newWatcherWithClient(mockClient, mockTrans)

	getResponse := &clientv3.GetResponse{
		Header: &etcdserverpb.ResponseHeader{Revision: 100},
		Kvs:    []*mvccpb.KeyValue{},
	}

	mockClient.EXPECT().
		Get(gomock.Any(), "/test/prefix/", gomock.Any()).
		Return(getResponse, nil)

	mockTrans.EXPECT().RebuildStart(gomock.Any()).Return(nil)
	mockTrans.EXPECT().RebuildEnd(gomock.Any()).Return(nil)

	watchCh := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().
		Watch(gomock.Any(), "/test/prefix/", gomock.Any(), gomock.Any()).
		Return((clientv3.WatchChan)(watchCh))

	s.Require().NoError(watcher.Start(context.Background()))

	// Stop the watcher
	_ = watcher.Stop()

	// Restart after stop should not panic
	s.NotPanics(func() {
		watcher.Restart()
	})
}

func TestWatcherSuite(t *testing.T) {
	suite.Run(t, new(WatcherTestSuite))
}
