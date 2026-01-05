package otel

import (
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// MetricFactory stores metric definitions and creates them lazily after the provider is set up.
// This ensures metrics are created with the correct resource attributes.
type MetricFactory struct {
	meter  metric.Meter
	prefix string
}

// NewFactory creates a new lazy metric factory.
// Metrics are not created until Register() is called.
func NewFactory(meterName, prefix string) *MetricFactory {
	f := &MetricFactory{
		meter:  otel.Meter(meterName),
		prefix: prefix,
	}
	return f
}

// name prefixes the metric name with the factory's prefix
func (f *MetricFactory) name(suffix string) string {
	if f.prefix == "" {
		return suffix
	}
	return f.prefix + "." + suffix
}

// Int64Counter registers a counter to be created later
func (f *MetricFactory) Int64Counter(target *metric.Int64Counter, name string, options ...metric.Int64CounterOption) {
	fullName := f.name(name)
	counter, err := f.meter.Int64Counter(fullName, options...)
	if err != nil {
		panic(fmt.Sprintf("failed to create counter %s: %v", fullName, err))
	}
	*target = counter
}

// Int64UpDownCounter registers an up-down counter to be created later
func (f *MetricFactory) Int64UpDownCounter(target *metric.Int64UpDownCounter, name string, options ...metric.Int64UpDownCounterOption) {
	fullName := f.name(name)
	counter, err := f.meter.Int64UpDownCounter(fullName, options...)
	if err != nil {
		panic(fmt.Sprintf("failed to create up-down counter %s: %v", fullName, err))
	}
	*target = counter
}

// Float64Histogram registers a histogram to be created later
func (f *MetricFactory) Float64Histogram(target *metric.Float64Histogram, name string, options ...metric.Float64HistogramOption) {
	fullName := f.name(name)
	histogram, err := f.meter.Float64Histogram(fullName, options...)
	if err != nil {
		panic(fmt.Sprintf("failed to create histogram %s: %v", fullName, err))
	}
	*target = histogram
}

// Int64Histogram registers a histogram to be created later
func (f *MetricFactory) Int64Histogram(target *metric.Int64Histogram, name string, options ...metric.Int64HistogramOption) {
	fullName := f.name(name)
	histogram, err := f.meter.Int64Histogram(fullName, options...)
	if err != nil {
		panic(fmt.Sprintf("failed to create histogram %s: %v", fullName, err))
	}
	*target = histogram
}

// Float64Counter registers a counter to be created later
func (f *MetricFactory) Float64Counter(target *metric.Float64Counter, name string, options ...metric.Float64CounterOption) {
	fullName := f.name(name)
	counter, err := f.meter.Float64Counter(fullName, options...)
	if err != nil {
		panic(fmt.Sprintf("failed to create counter %s: %v", fullName, err))
	}
	*target = counter
}

// Float64UpDownCounter registers an up-down counter to be created later
func (f *MetricFactory) Float64UpDownCounter(target *metric.Float64UpDownCounter, name string, options ...metric.Float64UpDownCounterOption) {
	fullName := f.name(name)
	counter, err := f.meter.Float64UpDownCounter(fullName, options...)
	if err != nil {
		panic(fmt.Sprintf("failed to create up-down counter %s: %v", fullName, err))
	}
	*target = counter
}
