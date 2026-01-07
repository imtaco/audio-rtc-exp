package log

import (
	"encoding/json"
	//nolint:depguard
	"log"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
)

// for init only
func Fatal(v ...any) {
	log.Fatal(v...)
}

type Logger struct {
	*zap.Logger
	names      []string
	moduleFunc func(names []string) *zap.Logger
}

func (l *Logger) Module(name string) *Logger {
	names := make([]string, len(l.names)+1)
	copy(names, l.names)
	names[len(l.names)] = name

	return &Logger{
		names:      names,
		Logger:     l.moduleFunc(names),
		moduleFunc: l.moduleFunc,
	}
}

func NewLogger(configFile string) (*Logger, error) {
	if configFile == "" {
		return newDefaultLogger(), nil
	}
	return loadLoggerFromFile(configFile)
}

func loadLoggerFromFile(configFile string) (*Logger, error) {
	bs, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	cfg := zap.Config{}
	if err := json.Unmarshal(bs, &cfg); err != nil {
		return nil, err
	}

	zapLogger, err := cfg.Build()
	if err != nil {
		return nil, err
	}

	moduleFunc := func(names []string) *zap.Logger {
		return zapLogger.Named(strings.Join(names, "."))
	}

	return &Logger{
		moduleFunc: moduleFunc,
		Logger:     zapLogger.Named("main"),
	}, nil
}

func newDefaultLogger() *Logger {
	encCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeName: func(name string, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString("[" + name + "]")
		},
	}

	encoder := zapcore.NewConsoleEncoder(encCfg)
	writer := zapcore.AddSync(os.Stdout)
	level := zapcore.InfoLevel
	if lv, ok := parseLevelFromEnv("LOG_LEVEL"); ok {
		level = lv
	}

	core := zapcore.NewCore(
		encoder,
		writer,
		zap.NewAtomicLevelAt(level),
	)
	baseLogger := zap.New(
		core,
		zap.AddStacktrace(zapcore.FatalLevel),
	)

	moduleFunc := func(names []string) *zap.Logger {
		lv := moduleLevel(names)
		core := zapcore.NewCore(
			encoder,
			writer,
			zap.NewAtomicLevelAt(lv),
		)
		logger := zap.New(
			core,
			zap.AddStacktrace(zapcore.FatalLevel),
		).Named(strings.Join(names, "."))

		logger.Info("use module log", zap.Any("level", lv))
		return logger
	}

	return &Logger{
		moduleFunc: moduleFunc,
		Logger:     baseLogger.Named("main"),
	}
}

func NewTest(t *testing.T) *Logger {
	logger := zaptest.NewLogger(t)
	return &Logger{
		Logger: logger,
		moduleFunc: func(names []string) *zap.Logger {
			return logger.Named(strings.Join(names, "."))
		},
	}
}

func NewNop() *Logger {
	logger := zap.NewNop()
	return &Logger{
		Logger: logger,
		moduleFunc: func(_ []string) *zap.Logger {
			return logger
		},
	}
}
