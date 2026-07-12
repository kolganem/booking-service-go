package messaging

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
)

// CreateBookingJobCommand -- команда на создание задания в Catalog.
// Публикуется при создании бронирования.
// JSON-теги в PascalCase для совместимости с Rebus (C# default JsonSerializerOptions).
type CreateBookingJobCommand struct {
	EventId    string `json:"EventId"`
	RequestId  string `json:"RequestId"` // BookingID в формате UUID
	ResourceId int64  `json:"ResourceId"`
	StartDate  string `json:"StartDate"` // формат: "2006-01-02"
	EndDate    string `json:"EndDate"`   // формат: "2006-01-02"
}

// CancelBookingJobCommand -- команда на отмену задания в Catalog.
// Публикуется при отмене бронирования.
type CancelBookingJobCommand struct {
	EventId   string `json:"EventId"`
	RequestId string `json:"RequestId"` // BookingID в формате UUID
}

// BookingJobConfirmed -- событие подтверждения бронирования от Catalog.
// Поля соответствуют C# классу BookingJobConfirmed (Rebus default serialization).
type BookingJobConfirmed struct {
	EventId   string `json:"EventId"`
	Id        int64  `json:"Id"`
	RequestId string `json:"RequestId"` // BookingID в формате UUID
}

// BookingJobDenied -- событие отклонения бронирования от Catalog.
type BookingJobDenied struct {
	EventId   string `json:"EventId"`
	Id        int64  `json:"Id"`
	RequestId string `json:"RequestId"` // BookingID в формате UUID
	Reason    string `json:"Reason"`
}

// CancelBookingJobFailed -- событие ошибки обработки команды CancelBookingJob в Catalog (DLQ).
type CancelBookingJobFailed struct {
	EventId   string `json:"EventId"`
	RequestId string `json:"RequestId"` // BookingID в формате UUID
}

// Routing keys для входящих событий от Catalog (consumer side, Rebus convention).
const (
	RoutingKeyBookingJobConfirmed    = "BookingService.Catalog.Async.Api.Contracts.Events.BookingJobConfirmed, BookingService.Catalog.Async.Api.Contracts"
	RoutingKeyBookingJobDenied       = "BookingService.Catalog.Async.Api.Contracts.Events.BookingJobDenied, BookingService.Catalog.Async.Api.Contracts"
	RoutingKeyCancelBookingJobFailed = "BookingService.Catalog.Async.Api.Contracts.Events.CancelBookingJobFailed, BookingService.Catalog.Async.Api.Contracts"
)

// QueueSuffixes для входящих событий — читаемые имена суффиксов очередей.
const (
	QueueSuffixBookingJobConfirmed    = "booking-job.confirmed"
	QueueSuffixBookingJobDenied       = "booking-job.denied"
	QueueSuffixCancelBookingJobFailed = "cancel-booking-job.failed"
)

// Routing keys и типы для исходящих команд в Catalog (publisher side, Rebus convention).
const (
	RoutingKeyCreateBookingJob = "BookingService.Catalog.Async.Api.Contracts.Requests.CreateBookingJobRequest, BookingService.Catalog.Async.Api.Contracts"
	RoutingKeyCancelBookingJob = "BookingService.Catalog.Async.Api.Contracts.Requests.CancelBookingJobByRequestIdRequest, BookingService.Catalog.Async.Api.Contracts"
)

// NewMessageID генерирует случайный UUID v4.
func NewMessageID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// BookingIDToRequestID кодирует bookingID в детерминированный UUID.
// Формат: 00000000-0000-0000-0000-{id:012x}.
func BookingIDToRequestID(id int64) string {
	return fmt.Sprintf("00000000-0000-0000-0000-%012x", id)
}

// RequestIDToBookingID декодирует bookingID из UUID, созданного BookingIDToRequestID.
func RequestIDToBookingID(requestID string) (int64, error) {
	parts := strings.Split(requestID, "-")
	if len(parts) != 5 {
		return 0, fmt.Errorf("неверный формат RequestId: %s", requestID)
	}
	id, err := strconv.ParseInt(parts[4], 16, 64)
	if err != nil {
		return 0, fmt.Errorf("невозможно извлечь bookingId из RequestId %s: %w", requestID, err)
	}
	return id, nil
}
