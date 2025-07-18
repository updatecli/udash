package server

var (
	// monitoringDurationDays is the default number of days to search for reports.
	// By default we keep this number as low as possible as it directly affects the
	// performance of the database queries.
	// The goal is to minimize the impact in small environment
	monitoringDurationDays int = 2
)
