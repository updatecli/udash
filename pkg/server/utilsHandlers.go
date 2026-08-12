package server

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

// getPaginationParamFromURLQuery sanitizes and retrieves pagination parameters from the request context.
// It returns the limit and page values, or an error if the parameters are invalid.
//
// It reports an invalid parameter to its caller rather than answering the request itself:
// writing the response here left the caller believing the request was still its to answer,
// which appended a second body to the one already sent.
func getPaginationParamFromURLQuery(c *gin.Context) (int, int, error) {
	limitStr := c.Request.URL.Query().Get("limit")
	pageStr := c.Request.URL.Query().Get("page")

	atoi := func(s string) (int, error) {
		if s == "" {
			return 0, nil
		}
		return strconv.Atoi(s)
	}

	limit, err := atoi(limitStr)
	if err != nil {
		return 0, 0, errors.New("invalid limit value")
	}

	page, err := atoi(pageStr)
	if err != nil {
		return 0, 0, errors.New("invalid page value")
	}

	if limit < 0 {
		return 0, 0, errors.New("limit cannot be negative")
	}

	if limit > maxPaginationLimit {
		return 0, 0, fmt.Errorf("limit exceeds maximum of %d", maxPaginationLimit)
	}

	if page < 0 {
		return 0, 0, errors.New("page cannot be negative")
	}

	// Pages are one based, so an unspecified page is the first one.
	if page == 0 {
		page = 1
	}

	return limit, page, nil
}

// validateTimeRangeParams checks a start_time and end_time pair before it reaches the
// database, which rejects a half open range. Catching it here turns what is a client
// mistake into a 400 rather than a 500.
func validateTimeRangeParams(startTime, endTime string) error {
	if (startTime == "") != (endTime == "") {
		return errors.New(ErrInvalidTimeRangeParams)
	}

	return nil
}
