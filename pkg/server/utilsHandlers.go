package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// getPaginationParams sanitizes and retrieves pagination parameters from the request context.
// It returns the limit and page values, or an error if the parameters are invalid.
func getPaginationParams(c *gin.Context) (int, int, error) {
	limitStr := c.Request.URL.Query().Get("limit")
	pageStr := c.Request.URL.Query().Get("page")

	atoi := func(s string) (int, error) {
		if s == "" {
			return 0, nil
		}
		return strconv.Atoi(s)
	}

	errs := []string{}
	limit, err := atoi(limitStr)
	if err != nil {
		errs = append(errs, "invalid limit value")
	}

	page, err := atoi(pageStr)
	if err != nil {
		errs = append(errs, "invalid page value")
	}

	if limit > 1000 {
		errs = append(errs, "limit exceeds maximum of 1000")
	}

	// Set default page value if not specified
	if page == 0 {
		page = 1
	}

	if len(errs) > 0 {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: "invalid query parameters: " + errs[0],
		})
		return 0, 0, err
	}

	return limit, page, nil
}
