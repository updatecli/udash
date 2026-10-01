package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
)

type SearchOpenActionsResponse struct {
	Data       []database.OpenActionData `json:"data"`
	TotalCount int                       `json:"total_count"`
}

// SearchOpenActions returns the actions left open, such as pull requests waiting to be merged
// @Summary Search open actions
// @Description Search the actions left open by the latest report of every pipeline, one entry per action URL along with the pipelines feeding it
// @Tags Pipeline Actions
// @Accept json
// @Produce json
// @Success 200 {object} SearchOpenActionsResponse
// @Failure 400 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/actions/search [post]
func SearchOpenActions(c *gin.Context) {

	type queryData struct {
		// Limit is the maximum number of actions to return
		// This is optional, unset returns every action
		Limit int `json:"limit"`
		// Page is the page number for pagination
		Page int `json:"page"`
		// StartTime is the start time for the time range filter
		// Time format is RFC3339: 2006-01-02T15:04:05Z07:00
		StartTime string `json:"start_time"`
		// EndTime is the end time for the time range filter
		// Time format is RFC3339: 2006-01-02T15:04:05Z07:00
		EndTime string `json:"end_time"`
		// Labels is a map of labels to filter the pipelines by
		Labels map[string]string `json:"labels,omitempty"`
		// ScmID only keeps the pipelines targeting that scm, such as a repository branch
		ScmID string `json:"scmid"`
		// Results only keeps the actions fed by at least one pipeline whose latest result
		// is one of them, such as "✔", "✗", "⚠" or "-". An empty list does not filter
		// anything out.
		Results []string `json:"results,omitempty"`
	}

	queryParams := queryData{}

	if err := c.ShouldBindJSON(&queryParams); err != nil {
		logrus.Errorf("failed to read json body: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	if err := validateTimeRangeParams(queryParams.StartTime, queryParams.EndTime); err != nil {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	dataset, totalCount, err := database.SearchOpenActions(
		database.SearchOpenActionsParams{
			Ctx:       c,
			Options:   database.ReportSearchOptions{Days: monitoringDurationDays},
			StartTime: queryParams.StartTime,
			EndTime:   queryParams.EndTime,
			Labels:    queryParams.Labels,
			ScmID:     queryParams.ScmID,
			Results:   queryParams.Results,
			Limit:     queryParams.Limit,
			Page:      queryParams.Page,
		},
	)
	if err != nil {
		logrus.Errorf("searching for open actions: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, SearchOpenActionsResponse{
		Data:       dataset,
		TotalCount: totalCount,
	})
}
