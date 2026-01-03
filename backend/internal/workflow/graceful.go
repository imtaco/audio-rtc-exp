package workflow

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type GracefulShutdownAction func(ctx context.Context)

func WaitGracefulShutdown(
	ctx context.Context,
	logger *log.Logger,
	action GracefulShutdownAction,
	timeout time.Duration,
) {
	logger.Info("Graceful shutdown handler registered")

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stop := signal.NotifyContext(
		ctx,
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// with timeout context
	<-ctx.Done()
	done := make(chan struct{})

	ctxClean, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic during graceful shutdown",
					log.Any("error", r))
			}
		}()
		logger.Info("Starting graceful shutdown")
		action(ctxClean)
		close(done)
	}()

	select {
	case <-ctxClean.Done():
		logger.Warn("Shutdown timeout exceeded, forcing exit")
	case <-done:
		logger.Info("Graceful shutdown completed")
	}
}
