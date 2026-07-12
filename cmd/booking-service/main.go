package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"booking-service/app/api"
	"booking-service/app/api/handler"
	"booking-service/app/clients/catalog"
	"booking-service/app/config"
	"booking-service/app/messaging"
	"booking-service/app/messaging/handlers"
	"booking-service/app/service"
	pgstore "booking-service/storage/postgres"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "не удалось загрузить конфигурацию: %v\n", err)
		os.Exit(1)
	}

	logger := setupLogger(cfg.App.LogLevel)
	defer func() { _ = logger.Sync() }()
	zap.ReplaceGlobals(logger)

	logger.Info("запуск сервиса",
		zap.String("name", cfg.App.Name),
		zap.String("env", cfg.App.Environment),
	)

	// Подключение к БД
	pool, err := pgstore.NewPool(context.Background(), cfg.Postgres.DSN())
	if err != nil {
		logger.Error("не удалось подключиться к БД", zap.Error(err))
		os.Exit(1)
	}
	defer pool.Close()

	repo := pgstore.NewBookingsRepository(pool)

	// Подключение к RabbitMQ
	mqConn, err := messaging.NewConnection(
		cfg.RabbitMQ.URL,
		cfg.RabbitMQ.PrefetchCount,
		logger,
	)
	if err != nil {
		logger.Error("не удалось подключиться к RabbitMQ", zap.Error(err))
		os.Exit(1)
	}
	defer mqConn.Close()

	publisher := messaging.NewPublisher(mqConn, cfg.RabbitMQ.ExchangeName, cfg.RabbitMQ.PublisherExchangeName, logger)

	// Сервисный слой
	bookingsService := service.NewBookingsService(repo, publisher, logger)
	bookingsQueries := service.NewBookingsQueries(repo, logger)

	// Catalog-клиент
	catalogClient := catalog.NewClient(
		cfg.Catalog.BaseURL,
		cfg.Catalog.Timeout,
		cfg.Catalog.MaxRetries,
		cfg.Catalog.RetryBaseDelay,
		logger,
	)
	_ = catalogClient

	// Хендлеры событий RabbitMQ
	confirmedHandler := handlers.NewBookingConfirmedHandler(bookingsService, logger)
	deniedHandler := handlers.NewBookingDeniedHandler(bookingsService, logger)
	cancelErrorHandler := handlers.NewCancelBookingErrorHandler(bookingsService, logger)

	// Контекст для graceful shutdown фоновых задач
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Consumer
	consumer := messaging.NewConsumer(mqConn, cfg.RabbitMQ.ExchangeName, cfg.RabbitMQ.QueuePrefix, logger)
	consumer.Subscribe(messaging.QueueSuffixBookingJobConfirmed, messaging.RoutingKeyBookingJobConfirmed, confirmedHandler.Handle)
	consumer.Subscribe(messaging.QueueSuffixBookingJobDenied, messaging.RoutingKeyBookingJobDenied, deniedHandler.Handle)
	consumer.Subscribe(messaging.QueueSuffixCancelBookingJobFailed, messaging.RoutingKeyCancelBookingJobFailed, cancelErrorHandler.Handle)

	if err := consumer.Start(ctx); err != nil {
		logger.Error("не удалось запустить consumer", zap.Error(err))
		os.Exit(1)
	}

	// HTTP-хендлеры и роутер
	bookingsHandler := handler.NewBookingsHandler(bookingsService, bookingsQueries, logger)
	router := api.NewRouter(bookingsHandler)

	// HTTP-сервер
	srv := &http.Server{
		Addr:         cfg.HTTP.Addr(),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		logger.Info("HTTP сервер запущен", zap.String("addr", cfg.HTTP.Addr()))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("ошибка HTTP сервера", zap.Error(err))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("завершение работы сервиса...")

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("ошибка при остановке сервера", zap.Error(err))
	}

	logger.Info("сервис остановлен")
}

func setupLogger(level string) *zap.Logger {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	cfg := zap.NewProductionConfig()
	cfg.Level.SetLevel(zapLevel)
	logger, _ := cfg.Build()
	return logger
}
