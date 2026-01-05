package otel

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// MetricRecorder helps record metrics with common patterns
type MetricRecorder struct {
	meter metric.Meter
}

// NewMetricRecorder creates a new metric recorder
func NewMetricRecorder(meter metric.Meter) *MetricRecorder {
	return &MetricRecorder{
		meter: meter,
	}
}

// RecordInt64Counter records an int64 counter metric
func (m *MetricRecorder) RecordInt64Counter(ctx context.Context, name string, value int64, attrs ...attribute.KeyValue) error {
	counter, err := m.meter.Int64Counter(name)
	if err != nil {
		return err
	}
	counter.Add(ctx, value, metric.WithAttributes(attrs...))
	return nil
}

// RecordFloat64Counter records a float64 counter metric
func (m *MetricRecorder) RecordFloat64Counter(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue) error {
	counter, err := m.meter.Float64Counter(name)
	if err != nil {
		return err
	}
	counter.Add(ctx, value, metric.WithAttributes(attrs...))
	return nil
}

// RecordInt64Histogram records an int64 histogram metric
func (m *MetricRecorder) RecordInt64Histogram(ctx context.Context, name string, value int64, attrs ...attribute.KeyValue) error {
	histogram, err := m.meter.Int64Histogram(name)
	if err != nil {
		return err
	}
	histogram.Record(ctx, value, metric.WithAttributes(attrs...))
	return nil
}

// RecordFloat64Histogram records a float64 histogram metric
func (m *MetricRecorder) RecordFloat64Histogram(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue) error {
	histogram, err := m.meter.Float64Histogram(name)
	if err != nil {
		return err
	}
	histogram.Record(ctx, value, metric.WithAttributes(attrs...))
	return nil
}
