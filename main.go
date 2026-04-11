package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/joho/godotenv"

	// OpenTelemetry imports
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// App struct
type App struct {
	DB              *sql.DB
	MasterKey       string
	requestCounter  metric.Int64Counter
	requestDuration metric.Float64Histogram
}

// --- LOGICA DE NEGÓCIO (HANDLERS REAIS) ---

func (app *App) healthHandler(w http.ResponseWriter, r *http.Request) {
	err := app.DB.Ping()
	if err != nil {
		http.Error(w, "Banco de dados indisponível", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// validateKeyHandler verifica se uma API Key existe e está ativa
func (app *App) validateKeyHandler(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "Chave não fornecida", http.StatusBadRequest)
		return
	}

	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM api_keys WHERE key_value = $1 AND active = true)`
	err := app.DB.QueryRowContext(r.Context(), query, key).Scan(&exists)
	
	if err != nil {
		http.Error(w, "Erro interno", http.StatusInternalServerError)
		return
	}

	if !exists {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"status": "invalid"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "valid"})
}

// createKeyHandler gera uma nova chave (protegido por Master Key)
func (app *App) createKeyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var input struct {
		Name string `json:"name"`
		Key  string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}

	query := `INSERT INTO api_keys (name, key_value, active, created_at) VALUES ($1, $2, true, NOW())`
	_, err := app.DB.ExecContext(r.Context(), query, input.Name, input.Key)
	if err != nil {
		http.Error(w, "Erro ao salvar no banco", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Chave criada com sucesso"))
}

func (app *App) masterKeyAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != app.MasterKey {
			http.Error(w, "Não autorizado", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- OPEN TELEMETRY SETUP ---

func initOTel(ctx context.Context) (func(), error) {
	otelEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if otelEndpoint == "" {
		otelEndpoint = "otel-collector.monitoring.svc.cluster.local:4317"
	}

	res, _ := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName("auth-service"),
		semconv.ServiceNamespace("togglemaster"),
	))

	traceExp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(otelEndpoint), otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExp), sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)

	metricExp, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpoint(otelEndpoint), otlpmetricgrpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp, sdkmetric.WithInterval(15*time.Second))),
	)
	otel.SetMeterProvider(mp)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tp.Shutdown(c)
		mp.Shutdown(c)
	}, nil
}

// --- MAIN ---

func main() {
	_ = godotenv.Load()
	ctx := context.Background()
	cleanup, _ := initOTel(ctx)
	if cleanup != nil {
		defer cleanup()
	}

	db, err := connectDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Falha no banco: %v", err)
	}
	defer db.Close()

	meter := otel.Meter("auth-service")
	counter, _ := meter.Int64Counter("http_requests_total")
	duration, _ := meter.Float64Histogram("http_request_duration_seconds")

	app := &App{
		DB:              db,
		MasterKey:       os.Getenv("MASTER_KEY"),
		requestCounter:  counter,
		requestDuration: duration,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", app.healthHandler)
	mux.HandleFunc("/validate", app.validateKeyHandler)
	mux.Handle("/admin/keys", app.masterKeyAuthMiddleware(http.HandlerFunc(app.createKeyHandler)))

	// Instrumentação OTel no Handler principal
	handler := otelhttp.NewHandler(mux, "auth-service")

	port := os.Getenv("PORT")
	if port == "" { port = "8001" }

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("Auth Service iniciado na porta %s", port)
	log.Fatal(server.ListenAndServe())
}

func connectDB(url string) (*sql.DB, error) {
	db, err := sql.Open("pgx", url)
	if err != nil { return nil, err }
	return db, db.Ping()
}