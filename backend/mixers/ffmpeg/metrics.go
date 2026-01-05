package ffmpeg

import (
	"go.opentelemetry.io/otel/metric"

	intotel "github.com/imtaco/audio-rtc-exp/internal/otel"
)

var (
	// Package-level metrics
	activeProcesses  metric.Int64UpDownCounter
	processesStarted metric.Int64Counter
	processesStopped metric.Int64Counter
	processesFailed  metric.Int64Counter
	startDuration    metric.Int64Histogram
)

func init() {
	f := intotel.NewFactory("mixer.ffmpeg", intotel.PrefixMixers)

	f.Int64UpDownCounter(&activeProcesses, "ffmpeg.processes.active",
		metric.WithDescription("Number of active FFmpeg processes"))

	f.Int64Counter(&processesStarted, "ffmpeg.processes.started",
		metric.WithDescription("Total number of FFmpeg processes started"))

	f.Int64Counter(&processesStopped, "ffmpeg.processes.stopped",
		metric.WithDescription("Total number of FFmpeg processes stopped"))

	f.Int64Counter(&processesFailed, "ffmpeg.processes.failed",
		metric.WithDescription("Total number of FFmpeg processes that failed"))

	f.Int64Histogram(&startDuration, "ffmpeg.start.duration",
		metric.WithDescription("Duration of FFmpeg start operations in milliseconds"),
		metric.WithUnit("ms"))
}
