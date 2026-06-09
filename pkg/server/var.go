package server

var (
	// monitoringDurationDays is the default number of days to search for reports.
	// By default we keep this number as low as possible as it directly affects the
	// performance of the database queries.
	// The goal is to minimize the impact in small environment
	monitoringDurationDays int = 7
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
	ErrInvalidJWT          = "JWT is invalid"
)
