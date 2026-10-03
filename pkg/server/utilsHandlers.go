package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/updatecli/udash/pkg/database"
)

// respondWithError answers a request which failed for the given error.
//
// An error caused by the request parameters is returned to the caller, so that they know
// what to fix. Anything else may carry SQL or driver details, which are only meant for the
// logs, so the caller gets a generic message instead.
func respondWithError(c *gin.Context, err error) {
	if errors.Is(err, database.ErrInvalidParameter) {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{Err: err.Error()})
		return
	}

	c.JSON(http.StatusInternalServerError, DefaultResponseModel{Err: ErrInternal})
}

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
