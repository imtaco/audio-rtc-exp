package redis

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	redisstream "github.com/imtaco/audio-rtc-exp/internal/stream/redis"
)

// redisStream wraps a redis-stream connection to implement jsonrpc2.ObjectStream
type rsStream struct {
	producer redisstream.Producer
	consumer redisstream.Consumer
	logger   *log.Logger
}

func (rs *rsStream) Write(_ context.Context, obj interface{}) error {
	rs.logger.Debug("write to redis stream", log.Any("obj", obj))

	bs, err := json.Marshal(obj)
	if err != nil {
		return errors.Wrap(err, "failed to marshal object")
	}

	payload := map[string]interface{}{
		"data": bs,
	}
	_, err = rs.producer.Add(context.Background(), payload)
	return err
}

func (rs *rsStream) Read(ctx context.Context, v interface{}) error {
	rs.logger.Debug("read readstream once")
	if rs.consumer == nil {
		<-ctx.Done()
		return jsonrpc.ErrClosed
	}

	var value map[string]interface{}

	select {
	case <-ctx.Done():
		return jsonrpc.ErrClosed
	case msg, ok := <-rs.consumer.Channel():
		if !ok {
			return jsonrpc.ErrClosed
		}
		value = msg.Values
	}

	raw, ok := extractDataField(value)
	if !ok {
		return errors.New("message missing data field")
	}

	err := json.Unmarshal(raw, v)
	if err != nil {
		// TODO: unmarhal error handling, should not broke the whole stream
		// but need to notify the other side about the error ?
		// e.g. DLQ handling
		rs.logger.Warn("unmarshal redis stream payload error", log.Error(err))
		return nil
	}
	rs.logger.Debug("read from redis stream", log.Any("obj", v))
	return nil
}

func (rs *rsStream) Disconnected() <-chan struct{} {
	// TODO: implement proper disconnection notification
	return make(<-chan struct{})
}

func (rs *rsStream) Open(ctx context.Context) error {
	if rs.consumer == nil {
		return nil
	}
	return rs.consumer.Open(ctx)
}

func (rs *rsStream) Close() error {
	if rs.consumer == nil {
		return nil
	}
	rs.consumer.Close()
	return nil
}

func extractDataField(values map[string]interface{}) ([]byte, bool) {
	v, ok := values["data"]
	if !ok {
		return nil, false
	}
	switch val := v.(type) {
	case string:
		return []byte(val), true
	case []byte:
		return val, true
	default:
		return nil, false
	}
}
