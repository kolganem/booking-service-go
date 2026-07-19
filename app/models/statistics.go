package models

// BookingStatistics -- агрегированная статистика бронирований за период.
type BookingStatistics struct {
	TotalCount   int64
	ByStatus     map[BookingStatus]int64
	TopResources []ResourceStatistic
}

// ResourceStatistic -- количество бронирований по одному ресурсу.
type ResourceStatistic struct {
	ResourceID int64
	Count      int64
}
