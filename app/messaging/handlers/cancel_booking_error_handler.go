package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/service"
)

// CancelBookingErrorHandler обрабатывает события ошибки обработки CancelBookingJob
// в Catalog (DLQ) и выполняет откат отмены бронирования.
type CancelBookingErrorHandler struct {
	service *service.BookingsService
	logger  *zap.Logger
}

// NewCancelBookingErrorHandler создаёт новый обработчик.
func NewCancelBookingErrorHandler(svc *service.BookingsService, logger *zap.Logger) *CancelBookingErrorHandler {
	return &CancelBookingErrorHandler{
		service: svc,
		logger:  logger,
	}
}

// Handle обрабатывает событие ошибки отмены бронирования и выполняет откат статуса.
func (h *CancelBookingErrorHandler) Handle(ctx context.Context, body []byte) error {
	var event messaging.CancelBookingJobFailed
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("десериализация CancelBookingJobFailed: %w", err)
	}

	h.logger.Info("получено событие CancelBookingJobFailed", zap.String("requestId", event.RequestId))

	if err := h.service.HandleCancelError(ctx, event.RequestId); err != nil {
		return fmt.Errorf("откат отмены бронирования (requestId=%s): %w", event.RequestId, err)
	}

	h.logger.Info("отмена бронирования откачена через DLQ-событие", zap.String("requestId", event.RequestId))
	return nil
}
