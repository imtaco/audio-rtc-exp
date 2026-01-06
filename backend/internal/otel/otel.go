package otel

import (
	"context"
	"fmt"

	runtimeotel "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

// ShutdownFunc is a function that shuts down the OTEL providers
type ShutdownFunc func(context.Context) error

type providers struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	config         *Config
	logger         *log.Logger
}

// Init creates and configures OpenTelemetry providers, sets them globally,
// and returns a shutdown function for cleanup
func Init(ctx context.Context, config *Config, logger *log.Logger) (ShutdownFunc, error) {
	p := &providers{
		config: config,
		logger: logger,
	}

	// Log OTEL configuration
	if logger != nil {
		logger.Info("OTEL configuration",
			log.Bool("tracing_enabled", config.TracingEnabled),
			log.Bool("metrics_enabled", config.MetricsEnabled),
			log.Bool("go_metrics_enabled", config.RuntimeMetricsEnabled),
			log.String("endpoint", config.Endpoint),
			log.String("service_name", config.ServiceName))
	}

	// Create resource with service name and automatic environment detection
	// This will automatically detect:
	// - Kubernetes pod name, namespace, cluster, etc. (from downward API env vars)
	// - Container ID
	// - Host information
	// - Process information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
		),
		// Detect environment automatically (K8s, Docker, host, process)
		resource.WithFromEnv(),   // Read OTEL_RESOURCE_ATTRIBUTES env var
		resource.WithHost(),      // Add host.* attributes
		resource.WithDetectors(), // Use all default detectors
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Initialize tracing if enabled
	if config.TracingEnabled {
		tracerProvider, err := initTracing(ctx, config, res)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize tracing: %w", err)
		}
		p.tracerProvider = tracerProvider
	} else {
		// Use noop provider
		p.tracerProvider = sdktrace.NewTracerProvider()
	}

	// Initialize metrics if enabled
	if config.MetricsEnabled {
		meterProvider, err := initMetrics(ctx, config, res)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize metrics: %w", err)
		}
		p.meterProvider = meterProvider

		// Initialize runtime metrics if enabled
		if config.RuntimeMetricsEnabled {
			if err := runtimeotel.Start(
				runtimeotel.WithMeterProvider(meterProvider),
			); err != nil {
				return nil, fmt.Errorf("failed to start runtime metrics: %w", err)
			}
		}
	} else {
		// Use noop provider
		p.meterProvider = sdkmetric.NewMeterProvider()
	}

	return p.shutdown, nil
}

func initTracing(ctx context.Context, config *Config, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	// Create OTLP trace exporter
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(config.Endpoint),
		otlptracegrpc.WithTimeout(config.Timeout),
	}

	if config.Insecure {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(insecure.NewCredentials()))
		opts = append(opts, otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	// Create sampler based on sampling rate
	var sampler sdktrace.Sampler
	switch {
	case config.SamplingRate >= 1.0:
		sampler = sdktrace.AlwaysSample()
	case config.SamplingRate <= 0.0:
		sampler = sdktrace.NeverSample()
	default:
		sampler = sdktrace.TraceIDRatioBased(config.SamplingRate)
	}

	// Create tracer provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global tracer provider
	otel.SetTracerProvider(provider)

	// Set global propagator to propagate trace context
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return provider, nil
}

func initMetrics(ctx context.Context, config *Config, res *resource.Resource) (*sdkmetric.MeterProvider, error) {
	// Create OTLP metric exporter
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(config.Endpoint),
		otlpmetricgrpc.WithTimeout(config.Timeout),
	}

	if config.Insecure {
		opts = append(opts, otlpmetricgrpc.WithTLSCredentials(insecure.NewCredentials()))
		opts = append(opts, otlpmetricgrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}

	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
	}

	// Create meter provider with periodic reader
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			exporter,
			sdkmetric.WithInterval(config.MetricsExportInterval),
		)),
	)

	// Set global meter provider
	// new provider will delegate to existed meters automatically (already defined in init() in metric.go of each module)
	otel.SetMeterProvider(provider)

	return provider, nil
}

// shutdown gracefully shuts down both tracer and meter providers
func (p *providers) shutdown(ctx context.Context) error {
	var errs []error

	if p.tracerProvider != nil {
		if err := p.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown tracer provider: %w", err))
		}
	}
	if p.meterProvider != nil {
		if err := p.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown meter provider: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	return nil
}
