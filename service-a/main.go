package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pperesbr/go-expert-otel/otel-lib/pkg" // Alterar para o caminho do seu módulo
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Configurar o provedor OpenTelemetry
	config := otel.DefaultConfig()
	config.ServiceName = "exemplo-servico"
	config.ServiceVersion = "1.0.0"
	config.Environment = "production"
	config.OtelEndpoint = "otel-collector:4317" // Ajuste para o endereço do seu coletor
	config.Attributes = []attribute.KeyValue{
		attribute.String("deployment.region", "br-south"),
	}

	// Inicializar o provedor
	provider, err := otel.InitProvider(ctx, config)
	if err != nil {
		log.Fatalf("Falha ao inicializar o provedor OpenTelemetry: %v", err)
	}
	defer func() {
		// Garantir que todos os spans sejam enviados antes de encerrar
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := provider.Shutdown(shutdownCtx); err != nil {
			log.Fatalf("Erro ao finalizar o provedor: %v", err)
		}
	}()

	// Criar middleware com tracer
	tracerMiddleware := otel.NewTracerMiddleware(provider, "api-handlers")

	// Exemplo de handler que usa o tracer diretamente
	http.HandleFunc("/manual-trace", func(w http.ResponseWriter, r *http.Request) {
		// Obter tracer diretamente
		tracer := provider.GetTracer("manual-example")

		// Iniciar um span
		_, span := tracer.Start(r.Context(), "manual-operation")
		defer span.End()

		// Adicionar atributos ao span
		span.SetAttributes(attribute.String("example.key", "example-value"))

		// Simulação de operação
		time.Sleep(100 * time.Millisecond)

		fmt.Fprintln(w, "Operação com trace manual completada!")
	})

	// Exemplo de como seria um middleware para HTTP
	http.HandleFunc("/api/users", exampleHandler(tracerMiddleware))

	// Iniciar servidor HTTP
	server := &http.Server{
		Addr: ":8080",
	}

	// Iniciar o servidor em uma goroutine
	go func() {
		log.Println("Iniciando servidor na porta 8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Erro ao iniciar servidor: %v", err)
		}
	}()

	// Aguardar sinal para encerramento gracioso
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Encerrando servidor...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Erro ao encerrar servidor: %v", err)
	}

	log.Println("Servidor encerrado com sucesso")
}

// Exemplo de função que usa o middleware de trace
func exampleHandler(middleware *otel.TracerMiddleware) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Criar um span para esta operação
		ctx, span := middleware.Tracer.Start(r.Context(), "process-user-request")
		defer span.End()

		// Adicionar atributos personalizados ao span
		span.SetAttributes(
			attribute.String("user.id", r.URL.Query().Get("id")),
			attribute.String("request.type", "user-info"),
		)

		// Criar um child span para uma sub-operação
		childCtx, childSpan := middleware.Tracer.Start(ctx, "database-query")
		// Simular uma consulta ao banco de dados
		time.Sleep(200 * time.Millisecond)
		childSpan.End()

		// Outra sub-operação
		_, anotherSpan := middleware.Tracer.Start(ctx, "process-user-data")
		// Simular processamento de dados
		time.Sleep(150 * time.Millisecond)
		anotherSpan.End()

		// Responder ao cliente
		fmt.Fprintln(w, "Processamento de usuário concluído com rastreamento!")
	}
}
