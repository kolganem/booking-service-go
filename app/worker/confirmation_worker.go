package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"booking-service/app/clients/catalog"
	"booking-service/app/models"
	"booking-service/app/service"
)

// ConfirmationWorker -- фоновый воркер для опроса Catalog и подтверждения бронирований.
//
// Логика работы:
//  1. Получить бронирования в статусе AwaitsConfirmation
//  2. Для каждого бронирования запросить статус у Catalog-сервиса
//  3. Если Catalog подтвердил -- вызвать BookingsService.Confirm()
//  4. Если Catalog отклонил -- вызвать BookingsService.Cancel()
type ConfirmationWorker struct {
	service       *service.BookingsService
	repo          models.BookingRepository
	catalogClient *catalog.Client
	interval      time.Duration
	batchSize     int
	logger        *zap.Logger
}

// NewConfirmationWorker создаёт новый воркер подтверждения.
func NewConfirmationWorker(
	svc *service.BookingsService,
	repo models.BookingRepository,
	catalogClient *catalog.Client,
	interval time.Duration,
	batchSize int,
	logger *zap.Logger,
) *ConfirmationWorker {
	return &ConfirmationWorker{
		service:       svc,
		repo:          repo,
		catalogClient: catalogClient,
		interval:      interval,
		batchSize:     batchSize,
		logger:        logger,
	}
}

// Run запускает воркер. Блокирует до отмены контекста.
func (w *ConfirmationWorker) Run(ctx context.Context) {
	w.logger.Info("воркер подтверждения запущен",
		zap.Duration("interval", w.interval),
		zap.Int("batchSize", w.batchSize),
	)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("воркер подтверждения остановлен")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

// processBatch обрабатывает пакет бронирований, ожидающих подтверждения.
func (w *ConfirmationWorker) processBatch(ctx context.Context) {
	bookings, err := w.repo.GetAwaitingConfirmation(ctx, w.batchSize)
	if err != nil {
		w.logger.Error("ошибка получения бронирований для подтверждения", zap.Error(err))
		return
	}

	if len(bookings) == 0 {
		return
	}

	w.logger.Info("обработка бронирований", zap.Int("count", len(bookings)))

	for _, booking := range bookings {
		w.processBooking(ctx, &booking)
	}
}

// processBooking обрабатывает одно бронирование: запрашивает статус у Catalog
// и обновляет статус в нашей БД.
func (w *ConfirmationWorker) processBooking(ctx context.Context, booking *models.Booking) {
	bookingID := booking.ID()
	logger := w.logger.With(zap.Int64("bookingId", bookingID))

	// Запрос статуса у Catalog
	job, err := w.catalogClient.GetBookingJobByBookingID(ctx, bookingID)
	if err != nil {
		logger.Error("ошибка запроса статуса у Catalog", zap.Error(err))
		return
	}

	switch job.Status {
	case "confirmed":
		if _, err := w.service.Confirm(ctx, bookingID); err != nil {
			logger.Error("ошибка подтверждения бронирования", zap.Error(err))
			return
		}
		logger.Info("бронирование подтверждено через polling")

	case "denied":
		if err := w.service.Cancel(ctx, bookingID); err != nil {
			logger.Error("ошибка отмены бронирования", zap.Error(err))
			return
		}
		logger.Info("бронирование отклонено через polling")

	case "pending":
		logger.Debug("бронирование ещё ожидает решения")

	default:
		logger.Warn("неизвестный статус задания", zap.String("status", job.Status))
	}
}
