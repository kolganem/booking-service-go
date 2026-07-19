package models

import "time"

// BookingStatus представляет статус бронирования.
type BookingStatus string

const (
	BookingStatusAwaitsConfirmation  BookingStatus = "awaits_confirmation"
	BookingStatusConfirmed           BookingStatus = "confirmed"
	BookingStatusCancelled           BookingStatus = "cancelled"
	BookingStatusCancellationPending BookingStatus = "cancellation_pending"
)

// IsValid проверяет, что статус принадлежит допустимому множеству.
func (s BookingStatus) IsValid() bool {
	switch s {
	case
		BookingStatusAwaitsConfirmation,
		BookingStatusConfirmed,
		BookingStatusCancelled,
		BookingStatusCancellationPending:

		return true
	default:
		return false
	}
}

// Booking -- доменная сущность бронирования.
// Поля неэкспортируемые для обеспечения инкапсуляции.
type Booking struct {
	id                      int64
	status                  BookingStatus
	userID                  int64
	resourceID              int64
	startDate               time.Time
	endDate                 time.Time
	createdAt               time.Time
	previousStatus          BookingStatus
	cancellationRequestedAt time.Time
}

func (b *Booking) ID() int64                          { return b.id }
func (b *Booking) Status() BookingStatus              { return b.status }
func (b *Booking) UserID() int64                      { return b.userID }
func (b *Booking) ResourceID() int64                  { return b.resourceID }
func (b *Booking) StartDate() time.Time               { return b.startDate }
func (b *Booking) EndDate() time.Time                 { return b.endDate }
func (b *Booking) CreatedAt() time.Time               { return b.createdAt }
func (b *Booking) PreviousStatus() BookingStatus      { return b.previousStatus }
func (b *Booking) CancellationRequestedAt() time.Time { return b.cancellationRequestedAt }

// NewBooking создаёт новое бронирование в статусе AwaitsConfirmation.
func NewBooking(userID, resourceID int64, startDate, endDate time.Time) (*Booking, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}
	if resourceID <= 0 {
		return nil, ErrInvalidResourceID
	}
	if startDate.IsZero() || endDate.IsZero() {
		return nil, ErrInvalidDateRange
	}
	if !endDate.After(startDate) {
		return nil, ErrEndDateBeforeStartDate
	}

	return &Booking{
		status:     BookingStatusAwaitsConfirmation,
		userID:     userID,
		resourceID: resourceID,
		startDate:  startDate,
		endDate:    endDate,
		createdAt:  time.Now(),
	}, nil
}

// Confirm подтверждает бронирование.
// Допустимый переход: AwaitsConfirmation -> Confirmed.
func (b *Booking) Confirm() error {
	if b.status != BookingStatusAwaitsConfirmation {
		return ErrInvalidStatusTransition
	}
	b.status = BookingStatusConfirmed
	return nil
}

// Cancel отменяет бронирование.
// Допустимые переходы:
//   - AwaitsConfirmation -> Cancelled
//   - Confirmed -> Cancelled (только если StartDate > today)
func (b *Booking) Cancel(today time.Time) error {
	switch b.status {
	case BookingStatusAwaitsConfirmation:
		b.status = BookingStatusCancelled
		return nil
	case BookingStatusConfirmed:
		if !b.startDate.After(today) {
			return ErrCannotCancelPastBooking
		}
		b.status = BookingStatusCancelled
		return nil
	case BookingStatusCancelled, BookingStatusCancellationPending:
		return ErrInvalidStatusTransition
	default:
		return ErrInvalidStatusTransition
	}
}

// RestoreBooking восстанавливает Booking из данных хранилища.
// Используется только в слое storage для маппинга строк БД на доменный объект.
func RestoreBooking(
	id int64,
	status BookingStatus,
	userID, resourceID int64,
	startDate, endDate, createdAt time.Time,
	previousStatus BookingStatus,
	cancellationRequestedAt time.Time,
) *Booking {
	return &Booking{
		id:                      id,
		status:                  status,
		userID:                  userID,
		resourceID:              resourceID,
		startDate:               startDate,
		endDate:                 endDate,
		createdAt:               createdAt,
		previousStatus:          previousStatus,
		cancellationRequestedAt: cancellationRequestedAt,
	}
}

// BeginCancellation начинает отмену бронирования по Compensating Transaction Pattern:
// переводит бронирование в промежуточный статус CancellationPending и запоминает
// предыдущий статус и время отправки команды -- для возможного отката.
// Допустимые переходы:
//   - AwaitsConfirmation -> CancellationPending
//   - Confirmed -> CancellationPending (только если StartDate > today)
func (b *Booking) BeginCancellation(cancelationTime time.Time) error {
	switch b.status {
	case BookingStatusAwaitsConfirmation:
		b.previousStatus = b.status
		b.status = BookingStatusCancellationPending
		b.cancellationRequestedAt = cancelationTime
		return nil
	case BookingStatusConfirmed:
		if !b.startDate.After(cancelationTime) {
			return ErrCannotCancelPastBooking
		}

		b.previousStatus = b.status
		b.status = BookingStatusCancellationPending
		b.cancellationRequestedAt = cancelationTime

		return nil
	case BookingStatusCancelled, BookingStatusCancellationPending:
		return ErrInvalidStatusTransition
	default:
		return ErrInvalidStatusTransition
	}
}

// CompleteCancellation завершает отмену после успешной обработки команды Catalog.
// Допустимый переход: CancellationPending -> Cancelled.
func (b *Booking) CompleteCancellation() error {
	if b.status != BookingStatusCancellationPending {
		return ErrInvalidStatusTransition
	}
	b.status = BookingStatusCancelled
	b.previousStatus = ""
	b.cancellationRequestedAt = time.Time{}
	return nil
}

// RollbackCancellation откатывает отмену к предыдущему статусу при ошибке
// обработки команды в Catalog (DLQ).
// Допустимый переход: CancellationPending -> previousStatus.
func (b *Booking) RollbackCancellation() error {
	if b.status != BookingStatusCancellationPending {
		return ErrInvalidStatusTransition
	}
	if b.previousStatus == "" {
		return ErrInvalidStatusTransition
	}
	b.status = b.previousStatus
	b.previousStatus = ""
	b.cancellationRequestedAt = time.Time{}
	return nil
}
