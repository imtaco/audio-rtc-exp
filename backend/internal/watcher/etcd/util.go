package etcd

import "encoding/json"

func ParseValue[T any](data []byte) *T {
	if len(data) == 0 {
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		panic("unable to unmarshal data")
	}
	return &value
}
