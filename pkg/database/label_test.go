package database

import (
	"context"
	"errors"
	"testing"
)

// The id is rejected before any query runs, so no database is needed.
func TestGetLabelRecordsRejectsMalformedID(t *testing.T) {
	_, _, err := GetLabelRecords(context.Background(), "not-a-uuid", "", "", "", "", 0, 1)
	if !errors.Is(err, ErrInvalidParameter) {
		t.Fatalf("expected an error wrapping ErrInvalidParameter, got %v", err)
	}
}
