module auth-service

go 1.21

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/jackc/pgx/v4 v4.18.3
	github.com/joho/godotenv v1.5.1
	go.opentelemetry.io/otel v1.28.0
	go.opentelemetry.io/otel/sdk v1.28.0
	go.opentelemetry.io/otel/metric v1.28.0
	go.opentelemetry.io/otel/sdk/metric v1.28.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.28.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.28.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.53.0
	go.opentelemetry.io/otel/propagation v0.0.0-00010101000000-000000000000
)
