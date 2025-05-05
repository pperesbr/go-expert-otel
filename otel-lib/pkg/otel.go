package pkg

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

// Config armazena a configuração para inicialização do OpenTelemetry
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OtelEndpoint   string
	Attributes     []attribute.KeyValue
	Sampler        sdktrace.Sampler
	Timeout        time.Duration
}

// DefaultConfig retorna uma configuração padrão
func DefaultConfig() Config {
	return Config{
		ServiceName:    "unknown-service",
		ServiceVersion: "0.0.1",
		Environment:    "development",
		OtelEndpoint:   "localhost:4317",
		Attributes:     []attribute.KeyValue{},
		Sampler:        sdktrace.AlwaysSample(),
		Timeout:        5 * time.Second,
	}
}

// Provider encapsula o provedor de telemetria
type Provider struct {
	tracerProvider *sdktrace.TracerProvider
	config         Config
}

// Shutdown finaliza o provedor de telemetria
func (p *Provider) Shutdown(ctx context.Context) error {
	return p.tracerProvider.Shutdown(ctx)
}

// GetTracer retorna um tracer com o nome especificado
func (p *Provider) GetTracer(name string) trace.Tracer {
	return p.tracerProvider.Tracer(name)
}

// GetTracerProvider retorna o provider do tracer
func (p *Provider) GetTracerProvider() *sdktrace.TracerProvider {
	return p.tracerProvider
}

// InitProvider inicializa o provedor OpenTelemetry com as configurações fornecidas
func InitProvider(ctx context.Context, cfg Config) (*Provider, error) {
	// Criar atributos de recurso
	attrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String(cfg.ServiceName),
		semconv.ServiceVersionKey.String(cfg.ServiceVersion),
		attribute.String("environment", cfg.Environment),
	}
	attrs = append(attrs, cfg.Attributes...)

	res, err := resource.New(ctx,
		resource.WithAttributes(attrs...),
		resource.WithProcessRuntimeDescription(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Criar conexão gRPC com o coletor OpenTelemetry
	secureOption := otlptracegrpc.WithInsecure()
	traceExporter, err := otlptrace.New(
		ctx,
		otlptracegrpc.NewClient(
			secureOption,
			otlptracegrpc.WithEndpoint(cfg.OtelEndpoint),
			otlptracegrpc.WithDialOption(grpc.WithBlock()),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Criar BatchSpanProcessor que gerenciará os spans
	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)

	// Criar o provedor de tracer
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(cfg.Sampler),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	// Definir o provedor global
	otel.SetTracerProvider(tracerProvider)

	// Configurar a propagação
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Provider{
		tracerProvider: tracerProvider,
		config:         cfg,
	}, nil
}

// WithTracer é um middleware para adicionar rastreamento a handlers HTTP
type TracerMiddleware struct {
	Tracer trace.Tracer
}

// NewTracerMiddleware cria um novo middleware de rastreamento
func NewTracerMiddleware(provider *Provider, tracerName string) *TracerMiddleware {
	return &TracerMiddleware{
		Tracer: provider.GetTracer(tracerName),
	}
}

// Exemplo de uso com um handler HTTP genérico:
//
// func (m *TracerMiddleware) Handle(next http.Handler) http.Handler {
//     return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//         ctx := r.Context()
//         spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
//
//         ctx, span := m.Tracer.Start(ctx, spanName)
//         defer span.End()
//
//         // Adiciona alguns atributos ao span
//         span.SetAttributes(
//             attribute.String("http.method", r.Method),
//             attribute.String("http.url", r.URL.String()),
//         )
//
//         // Chama o próximo handler com o contexto atualizado
//         next.ServeHTTP(w, r.WithContext(ctx))
//     })
// }
