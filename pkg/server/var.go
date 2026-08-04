package server

var (
	// monitoringDurationDays is the default number of days to search for reports.
	// By default we keep this number as low as possible as it directly affects the
	// performance of the database queries.
	// The goal is to minimize the impact in small environment
	monitoringDurationDays int = 7
	// maxMonitoringDurationDays is the largest number of days a summary query may span.
	// The time range itself is indexed but the aggregation runs over every matching row,
	// so a wide window means scanning most of the table.
	maxMonitoringDurationDays int = 366
	// maxSummaryBuckets is the largest number of buckets a summary may return.
	// maxMonitoringDurationDays bounds how much of the table a summary scans, this bounds
	// how large its response gets: an hourly summary of a year is a cheap scan but would
	// return more than eight thousand entries.
	maxSummaryBuckets int = 1000
	// errMessageType is the key used in JSON responses to indicate an error message.
	errMessageType = "error"
	// successMessageType is used to indicate a successful operation in API responses.
	successMessageType = "success"
)

const (
	// ErrInvalidPaginationParams is the error message returned when pagination parameters are invalid.
	ErrInvalidPaginationParams = "invalid pagination parameters"
	// ErrInvalidSummaryParam is the error message returned when the summary parameter is invalid.
	ErrInvalidSummaryParam = "invalid summary parameter"
	// ErrInvalidKeyOnlyParam is the error message returned when the keyonly parameter is invalid.
	ErrInvalidKeyOnlyParam = "invalid keyonly parameter"
	// ErrInvalidDaysParam is the error message returned when the days parameter is out of range.
	ErrInvalidDaysParam = "invalid days parameter"
	// ErrInvalidTimeRangeParams is the error message returned when only one of the time range boundaries is provided.
	ErrInvalidTimeRangeParams = "both start_time and end_time must be provided"
	// ErrInvalidMetricParam is the error message returned when the requested summary metric is not supported.
	ErrInvalidMetricParam = "invalid metric parameter"
	// ErrInvalidGranularityParam is the error message returned when the requested summary granularity is not supported.
	ErrInvalidGranularityParam = "invalid granularity parameter"
	// ErrTimeRangeTooWide is the error message returned when the requested time range spans more
	// than maxMonitoringDurationDays days.
	ErrTimeRangeTooWide = "requested time range exceeds the maximum allowed span"
	// ErrInvalidHoursParam is the error message returned when the hours parameter is out of range.
	ErrInvalidHoursParam = "invalid hours parameter"
	// ErrConflictingWindowParams is the error message returned when both days and hours are provided.
	ErrConflictingWindowParams = "days and hours cannot be combined"
	// ErrTooManyBuckets is the error message returned when the requested time range and granularity
	// would produce more than maxSummaryBuckets entries.
	ErrTooManyBuckets = "requested time range and granularity produce too many buckets"
	ErrInvalidJWT     = "JWT is invalid"

	// summaryMetricResult counts the pipeline reports per Updatecli result. It is the
	// only metric supported by the reports summary so far.
	summaryMetricResult = "result"
)
