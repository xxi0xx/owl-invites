package stats

// InstanceStats holds all aggregate statistics for the admin dashboard.
type InstanceStats struct {
	Events        EventStats        `json:"events"`
	Guests        GuestStats        `json:"guests"`
	Users         UserStats         `json:"users"`
	Features      FeatureAdoption   `json:"features"`
	Notifications NotificationStats `json:"notifications"`
}

// EventStats contains aggregate event counts by status.
type EventStats struct {
	Total     int `json:"total"`
	Draft     int `json:"draft"`
	Published int `json:"published"`
	Cancelled int `json:"cancelled"`
	Archived  int `json:"archived"`
}

// GuestStats contains aggregate per-guest response metrics.
type GuestStats struct {
	Total          int     `json:"total"`
	TotalHeadcount int     `json:"totalHeadcount"`
	Attending      int     `json:"attending"`
	Maybe          int     `json:"maybe"`
	Declined       int     `json:"declined"`
	Pending        int     `json:"pending"`
	AvgPerEvent    float64 `json:"avgPerEvent"`
}

type UserStats struct {
	Total int `json:"total"`
}

// FeatureAdoption tracks how many events use optional features.
type FeatureAdoption struct {
	OpenEnrollmentEvents int `json:"openEnrollmentEvents"`
	CohostedEvents       int `json:"cohostedEvents"`
	EventsWithQuestions  int `json:"eventsWithQuestions"`
	EventsWithCapacity   int `json:"eventsWithCapacity"`
	SeriesEvents         int `json:"seriesEvents"`
}

// NotificationStats contains aggregate email/notification metrics.
type NotificationStats struct {
	Total      int `json:"total"`
	Sent       int `json:"sent"`
	Failed     int `json:"failed"`
	Delivered  int `json:"delivered"`
	Opened     int `json:"opened"`
	Bounced    int `json:"bounced"`
	Complained int `json:"complained"`
}
