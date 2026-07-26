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
//     COALESCE(last_retry_at, cancellation_requested_at) старше таймаута
//  2. Для каждого повторно опубликовать CancelBookingJobCommand
//  3. При успешной публикации отметить прогресс (last_retry_at = now) --
//     это отодвигает запись в конец очереди выборки, поэтому новые зависшие
//     отмены не блокируются старыми при batchSize < числа зависших записей
//  4. Статус в БД не меняется -- переход в Cancelled или откат выполняется
//     обычным flow (успешная обработка команды в Catalog или DLQ-rollback)
//
// NOTE: в системе пока нет триггера успешного завершения отмены
// (models.Booking.CompleteCancellation() нигде не вызывается за пределами
// тестов), поэтому перманентно зависшие записи никогда не покидают
// CancellationPending и будут ретраиться бесконечно. Это отдельная задача.
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

	var retried, errored int
	for _, booking := range bookings {
		if err := w.retryBooking(ctx, &booking); err != nil {
			errored++
			continue
		}
		retried++
	}

	w.logger.Info("итог обработки пакета зависших отмен",
		zap.Int("retry", retried),
		zap.Int("errors", errored),
	)
}

// retryBooking повторно отправляет команду отмены для одного бронирования.
// Ошибка не прерывает обработку остальных бронирований в пакете, но
// возвращается вызывающему коду для подсчёта итогов по пакету.
func (w *CancellationRetryWorker) retryBooking(ctx context.Context, booking *models.Booking) error {
	bookingID := booking.ID()
	logger := w.logger.With(zap.Int64("bookingId", bookingID))

	err := w.publisher.PublishCancelBookingJob(ctx, messaging.CancelBookingJobCommand{
		EventId:   messaging.NewMessageID(),
		RequestId: messaging.BookingIDToRequestID(bookingID),
	})
	if err != nil {
		logger.Error("ошибка повторной публикации CancelBookingJob", zap.Error(err))
		return err
	}

	logger.Warn("повторно отправлена команда отмены для зависшего бронирования",
		zap.Time("cancellationRequestedAt", booking.CancellationRequestedAt()),
	)

	retriedAt := time.Now()
	if err := booking.MarkCancellationRetried(retriedAt); err != nil {
		logger.Error("ошибка отметки прогресса ретрая", zap.Error(err))
		return err
	}
	if err := w.repo.Update(ctx, booking); err != nil {
		logger.Error("ошибка сохранения прогресса ретрая", zap.Error(err))
		return err
	}
	return nil
}
