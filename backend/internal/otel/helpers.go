package otel

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// StartSpan starts a new span with the given name and attributes
func StartSpan(ctx context.Context, tracer trace.Tracer, spanName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	ctx, span := tracer.Start(ctx, spanName)
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	return ctx, span
}

// RecordError records an error on the span and sets its status
func RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// SetSpanAttributes sets attributes on the span
func SetSpanAttributes(span trace.Span, attrs ...attribute.KeyValue) {
	span.SetAttributes(attrs...)
}
