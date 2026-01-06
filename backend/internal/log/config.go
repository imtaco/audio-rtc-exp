package log

import (
	"fmt"
	"os"
	"strings"

	"github.com/iancoleman/strcase"
	"go.uber.org/zap/zapcore"
)

var (
	envFunc = env
)

func parseLevel(s string) (zapcore.Level, bool) {
	var lvl zapcore.Level
	err := lvl.Set(strings.ToLower(s))
	if err != nil {
		return zapcore.InfoLevel, false
	}
	return lvl, true
}

func env(key string) (string, bool) {
	v := os.Getenv(key)
	return v, v != ""
}

func parseLevelFromEnv(key string) (zapcore.Level, bool) {
	v, ok := envFunc(key)
	if !ok {
		return zapcore.InfoLevel, false
	}
	return parseLevel(v)
}

func moduleLevel(names []string) zapcore.Level {
	skNames := make([]string, len(names))
	for i, n := range names {
		skNames[i] = strcase.ToScreamingSnake(n)
	}

	keys := []string{}
	for i := len(skNames); i > 0; i-- {
		keys = append(keys, fmt.Sprintf("LOG_LEVEL__%s", strings.Join(skNames[:i], "__")))
	}
	keys = append(keys, "LOG_LEVEL")

	for _, k := range keys {
		if lv, ok := parseLevelFromEnv(k); ok {
			return lv
		}
	}

	return zapcore.InfoLevel
}
