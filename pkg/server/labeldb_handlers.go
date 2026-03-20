package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/model"
)

// ListLabelsResponse represents the response for the ListLabels endpoint.
type ListLabelsResponse struct {
	// Labels is a list of labels.
	Labels []model.Label `json:"labels"`
	// TotalCount is the total number of labels matching the query.
	TotalCount int `json:"total_count"`
}

// ListLabelKeyOnlyResponse represents the response for listing all available labels
type ListLabelKeyOnlyResponse struct {
	// Labels is a list of labels.
	Labels []string `json:"labels"`
	// TotalCount is the total number of labels matching the query.
	TotalCount int `json:"total_count"`
}

// ListLabels returns a list of labels from the database.
// @Summary List labels
// @Description List labels data from the database with optional filtering
// @Tags Labels
// @Param id query string false "Filter by label ID"
// @Param key query string false "Filter by label key"
// @Param value query string false "Filter by label value"
// @Param keyonly query string false "Return only unique label keys (true/false)"
// @Param limit query string false "Limit the number of labels returned, default is 100"
// @Param page query string false "Page number for pagination, default is 1"
// @Param start_time query string false "Start time for filtering labels (RFC3339 format)"
// @Param end_time query string false "End time for filtering labels (RFC3339 format)"
// @Success 200 {object} ListLabelsResponse
// @Failure 400 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/labels [get]
func ListLabels(c *gin.Context) {

	key := c.Request.URL.Query().Get("key")
	value := c.Request.URL.Query().Get("value")
	startTime := c.Request.URL.Query().Get("start_time")
	endTime := c.Request.URL.Query().Get("end_time")
	keyOnlyValue := c.Request.URL.Query().Get("keyonly")
	id := c.Request.URL.Query().Get("id")

	limit, page, err := getPaginationParamFromURLQuery(c)
	if err != nil {
		logrus.Errorf("getting pagination params: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: "invalid pagination parameters",
		})
		return
	}

	keyOnly := false
	if keyOnlyValue != "" {
		parsedKeyOnly, err := strconv.ParseBool(keyOnlyValue)
		if err != nil {
			c.JSON(http.StatusBadRequest, DefaultResponseModel{
				Err: "invalid keyonly parameter",
			})
			return
		}

		keyOnly = parsedKeyOnly
	}

	switch keyOnly {
	case true:
		results, totalCount, err := database.GetLabelKeyOnlyRecords(c, startTime, endTime, limit, page)
		if err != nil {
			logrus.Errorf("searching for labels: %s", err)
			c.JSON(http.StatusInternalServerError, DefaultResponseModel{
				Err: err.Error(),
			})

			return
		}

		c.JSON(http.StatusOK, ListLabelKeyOnlyResponse{
			Labels:     results,
			TotalCount: totalCount,
		})

		return

	case false:
		results, totalCount, err := database.GetLabelRecords(c, id, key, value, startTime, endTime, limit, page)
		if err != nil {
			logrus.Errorf("searching for labels: %s", err)
			c.JSON(http.StatusInternalServerError, DefaultResponseModel{
				Err: err.Error(),
			})

			return
		}

		c.JSON(http.StatusOK, ListLabelsResponse{
			Labels:     results,
			TotalCount: totalCount,
		})

		return
	}
}

// SearchLabels searches labels from the database using advanced filtering
// @Summary Search labels
// @Description Search labels in the database using advanced filtering
// @Param body body queryData true "Search parameters"
// @Tags Labels
// @Accept json
// @Produce json
// @Success 200 {object} ListLabelsResponse
// @Failure 400 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/labels/search [post]
func SearchLabels(c *gin.Context) {

	type queryData struct {
		// Id is the unique identifier of the label.
		Id string `json:"id"`
		// Key is the key of the label.
		Key string `json:"key"`
		// Value is the value of the label.
		Value string `json:"value"`
		// Limit is the maximum number of labels to return
		// This is optional and can be used to limit the number of labels returned
		Limit int `json:"limit"`
		// Page is the page number for pagination
		// This is optional and can be used to paginate the results
		Page int `json:"page"`
		// StartTime is the start time for the time range filter
		// This is optional and can be used to filter labels by a specific start time
		// Time format is RFC3339: 2006-01-02T15:04:05Z07:00
		StartTime string `json:"start_time"`
		// EndTime is the end time for the time range filter
		// This is optional and can be used to filter labels by a specific end time
		// Time format is RFC3339: 2006-01-02T15:04:05Z07:00
		EndTime string `json:"end_time"`
		// KeyOnly specifies if we only need to retrieve a list of uniq Label keys
		KeyOnly bool `json:"key_only"`
	}

	queryParams := queryData{}

	if err := c.ShouldBindJSON(&queryParams); err != nil {
		logrus.Errorf("failed to read json body: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	switch queryParams.KeyOnly {
	case true:
		results, totalCount, err := database.GetLabelKeyOnlyRecords(
			c,
			queryParams.StartTime,
			queryParams.EndTime,
			queryParams.Limit,
			queryParams.Page,
		)
		if err != nil {
			logrus.Errorf("searching for labels: %s", err)
			c.JSON(http.StatusInternalServerError, DefaultResponseModel{
				Err: err.Error(),
			})

			return
		}

		c.JSON(http.StatusOK, ListLabelKeyOnlyResponse{
			Labels:     results,
			TotalCount: totalCount,
		})

		return

	case false:
		results, totalCount, err := database.GetLabelRecords(
			c,
			queryParams.Id,
			queryParams.Key,
			queryParams.Value,
			queryParams.StartTime,
			queryParams.EndTime,
			queryParams.Limit,
			queryParams.Page)
		if err != nil {
			logrus.Errorf("searching for labels: %s", err)
			c.JSON(http.StatusInternalServerError, DefaultResponseModel{
				Err: err.Error(),
			})

			return
		}

		c.JSON(http.StatusOK, ListLabelsResponse{
			Labels:     results,
			TotalCount: totalCount,
		})

		return
	}

}
