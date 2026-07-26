package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
)

// CancelPublisher -- узкий интерфейс для повторной публикации команды отмены.
// Реализуется существующим *messaging.Publisher; выделен отдельно, чтобы
// CancellationRetryWorker можно было тестировать без реального подключения к RabbitMQ.
type CancelPublisher interface {
	PublishCancelBookingJob(ctx context.Context, cmd messaging.CancelBookingJobCommand) error
}

// CancellationRetryWorker -- фоновый воркер, повторно отправляющий команду
// отмены в Catalog для бронирований, зависших в статусе CancellationPending
// дольше заданного таймаута (например, из-за потери сообщения или таймаута
// ответа от Catalog).
//
// Логика работы:
//  1. Получить бронирования в статусе CancellationPending, у которых
//     cancellation_requested_at старше таймаута
//  2. Для каждого повторно опубликовать CancelBookingJobCommand
//  3. Статус в БД не меняется -- переход в Cancelled или откат выполняется
//     обычным flow (успешная обработка команды в Catalog или DLQ-rollback)
//
// NOTE: воркер не отмечает прогресс (cancellation_requested_at
// не обновляется), а в системе пока нет триггера успешного завершения отмены
// (models.Booking.CompleteCancellation() нигде не вызывается. Из-за этого, если
// количество перманентно зависших бронирований превысит batchSize, выборка
// каждый раз будет возвращать одни и те же самые старые записи -- новые
// зависшие отмены перестанут попадать в ретрай, а старые будут повторно
// отправляться бесконечно. 
// Потенциальное решение для отдельной задачи (progress-marker + completion-trigger).
type CancellationRetryWorker struct {
	repo      models.BookingRepository
	publisher CancelPublisher
	timeout   time.Duration
	interval  time.Duration
	batchSize int
	logger    *zap.Logger
}

// NewCancellationRetryWorker создаёт новый воркер повторной отправки отмены.
func NewCancellationRetryWorker(
	repo models.BookingRepository,
	publisher CancelPublisher,
	timeout time.Duration,
	interval time.Duration,
	batchSize int,
	logger *zap.Logger,
) *CancellationRetryWorker {
	return &CancellationRetryWorker{
		repo:      repo,
		publisher: publisher,
		timeout:   timeout,
		interval:  interval,
		batchSize: batchSize,
		logger:    logger,
	}
}

// Run запускает воркер. Блокирует до отмены контекста.
func (w *CancellationRetryWorker) Run(ctx context.Context) {
	w.logger.Info("воркер повторной отправки отмены запущен",
		zap.Duration("interval", w.interval),
		zap.Duration("timeout", w.timeout),
		zap.Int("batchSize", w.batchSize),
	)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("воркер повторной отправки отмены остановлен")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

// processBatch обрабатывает пакет зависших отмен.
func (w *CancellationRetryWorker) processBatch(ctx context.Context) {
	olderThan := time.Now().Add(-w.timeout)

	bookings, err := w.repo.GetStuckCancellations(ctx, olderThan, w.batchSize)
	if err != nil {
		w.logger.Error("ошибка получения зависших отмен", zap.Error(err))
		return
	}

	if len(bookings) == 0 {
		return
	}

	w.logger.Info("повторная отправка зависших отмен", zap.Int("count", len(bookings)))

	for _, booking := range bookings {
		w.retryBooking(ctx, &booking)
	}
}

// retryBooking повторно отправляет команду отмены для одного бронирования.
// Ошибка не прерывает обработку остальных бронирований в пакете.
func (w *CancellationRetryWorker) retryBooking(ctx context.Context, booking *models.Booking) {
	bookingID := booking.ID()
	logger := w.logger.With(zap.Int64("bookingId", bookingID))

	err := w.publisher.PublishCancelBookingJob(ctx, messaging.CancelBookingJobCommand{
		EventId:   messaging.NewMessageID(),
		RequestId: messaging.BookingIDToRequestID(bookingID),
	})
	if err != nil {
		logger.Error("ошибка повторной публикации CancelBookingJob", zap.Error(err))
		return
	}

	logger.Warn("повторно отправлена команда отмены для зависшего бронирования",
		zap.Time("cancellationRequestedAt", booking.CancellationRequestedAt()),
	)
}
