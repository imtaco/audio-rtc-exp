package log

import (
	"time"

	"go.uber.org/zap"
)

// Field is an alias for zap.Field to avoid importing zap in other packages.
type Field = zap.Field

func Bool(key string, val bool) Field {
	return zap.Bool(key, val)
}

func ByteString(key string, val []byte) Field {
	return zap.ByteString(key, val)
}

func Int(key string, val int) Field {
	return zap.Int(key, val)
}

func Int32(key string, val int32) Field {
	return zap.Int32(key, val)
}

func Int64(key string, val int64) Field {
	return zap.Int64(key, val)
}

func String(key string, val string) Field {
	return zap.String(key, val)
}

func Strings(key string, val []string) Field {
	return zap.Strings(key, val)
}

func Error(err error) Field {
	return zap.Error(err)
}

func Float64(key string, val float64) Field {
	return zap.Float64(key, val)
}

func Any(key string, val any) Field {
	return zap.Any(key, val)
}

func Duration(key string, val time.Duration) Field {
	return zap.Duration(key, val)
}

func Time(key string, val time.Time) Field {
	return zap.Time(key, val)
}
