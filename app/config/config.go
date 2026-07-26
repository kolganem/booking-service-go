package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Postgres PostgresConfig
	Catalog  CatalogConfig
	Worker   WorkerConfig
	RabbitMQ RabbitMQConfig
}

type AppConfig struct {
	Name        string `envconfig:"APP_NAME" default:"booking-service"`
	Environment string `envconfig:"APP_ENV" default:"development"`
	LogLevel    string `envconfig:"LOG_LEVEL" default:"info"`
}

type HTTPConfig struct {
	Host string `envconfig:"HTTP_HOST" default:"0.0.0.0"`
	Port int    `envconfig:"HTTP_PORT" default:"8080"`
}

type PostgresConfig struct {
	Host     string `envconfig:"POSTGRES_HOST" default:"localhost"`
	Port     int    `envconfig:"POSTGRES_PORT" default:"5432"`
	User     string `envconfig:"POSTGRES_USER" default:"postgres"`
	Password string `envconfig:"POSTGRES_PASSWORD" default:"postgres"`
	Database string `envconfig:"POSTGRES_DB" default:"booking"`
	SSLMode  string `envconfig:"POSTGRES_SSL_MODE" default:"disable"`
}

type CatalogConfig struct {
	BaseURL        string        `envconfig:"CATALOG_BASE_URL" default:"http://localhost:8000"`
	Timeout        time.Duration `envconfig:"CATALOG_TIMEOUT" default:"10s"`
	MaxRetries     int           `envconfig:"CATALOG_MAX_RETRIES" default:"3"`
	RetryBaseDelay time.Duration `envconfig:"CATALOG_RETRY_BASE_DELAY" default:"1s"`
}

type WorkerConfig struct {
	ConfirmationInterval time.Duration `envconfig:"WORKER_CONFIRMATION_INTERVAL" default:"30s"`
	ConfirmationBatch    int           `envconfig:"WORKER_CONFIRMATION_BATCH" default:"10"`

	// CancellationStuckTimeout -- через сколько бронирование в cancellation_pending
	// считается зависшим (Catalog не ответил timeout/потеря сообщения).
	CancellationStuckTimeout  time.Duration `envconfig:"WORKER_CANCELLATION_STUCK_TIMEOUT" default:"5m"`
	CancellationRetryInterval time.Duration `envconfig:"WORKER_CANCELLATION_RETRY_INTERVAL" default:"30s"`
	CancellationRetryBatch    int           `envconfig:"WORKER_CANCELLATION_RETRY_BATCH" default:"10"`
}

type RabbitMQConfig struct {
	URL                   string `envconfig:"RABBITMQ_URL" default:"amqp://admin:admin@localhost:5672/"`
	ExchangeName          string `envconfig:"RABBITMQ_EXCHANGE" default:"booking-service"`
	PublisherExchangeName string `envconfig:"RABBITMQ_PUBLISHER_EXCHANGE" default:"booking-service-topics"`
	QueuePrefix           string `envconfig:"RABBITMQ_QUEUE_PREFIX" default:"booking-service"`
	PrefetchCount         int    `envconfig:"RABBITMQ_PREFETCH_COUNT" default:"10"`
}

func (p PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		p.User, p.Password, p.Host, p.Port, p.Database, p.SSLMode,
	)
}

func (h HTTPConfig) Addr() string {
	return fmt.Sprintf("%s:%d", h.Host, h.Port)
}

// Load читает конфигурацию из переменных окружения.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("загрузка конфигурации: %w", err)
	}
	return &cfg, nil
}
