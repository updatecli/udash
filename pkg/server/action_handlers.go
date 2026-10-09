package server

import (
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
)

type SearchOpenActionsResponse struct {
	Data []database.OpenActionData `json:"data"`
	// TotalCount counts the actions matching the request, across every page.
	TotalCount int `json:"total_count"`
	// AcknowledgedCount counts the acknowledged actions, whatever the request asked for.
	AcknowledgedCount int `json:"acknowledged_count"`
}

// acknowledgeDays lists the accepted acknowledgement durations, in days.
var acknowledgeDays = []int{1, 7, 30}

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
		// Acknowledged returns only the acknowledged actions instead of only the others.
		Acknowledged bool `json:"acknowledged"`
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

	dataset, totalCount, acknowledgedCount, err := database.SearchOpenActions(
		database.SearchOpenActionsParams{
			Ctx:          c,
			Options:      database.ReportSearchOptions{Days: monitoringDurationDays},
			StartTime:    queryParams.StartTime,
			EndTime:      queryParams.EndTime,
			Labels:       queryParams.Labels,
			ScmID:        queryParams.ScmID,
			Results:      queryParams.Results,
			Acknowledged: queryParams.Acknowledged,
			Limit:        queryParams.Limit,
			Page:         queryParams.Page,
		},
	)
	if err != nil {
		logrus.Errorf("searching for open actions: %s", err)
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, SearchOpenActionsResponse{
		Data:              dataset,
		TotalCount:        totalCount,
		AcknowledgedCount: acknowledgedCount,
	})
}

// AcknowledgeAction hides an open action, such as a pull request somebody is handling
// @Summary Acknowledge an open action
// @Description Leave an open action out of the search for a number of days, or until the worst result of its pipelines is no longer the given one
// @Tags Pipeline Actions
// @Accept json
// @Produce json
// @Success 204
// @Failure 400 {object} DefaultResponseModel
// @Failure 401 {object} DefaultResponseModel
// @Failure 403 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/actions/ack [put]
func AcknowledgeAction(c *gin.Context) {
	type queryData struct {
		// URL is the link of the action
		URL string `json:"url" binding:"required"`
		// Days is how long the action stays acknowledged: 1, 7 or 30
		Days int `json:"days" binding:"required"`
		// Result is the worst result shown by the search when the action was
		// acknowledged, such as "✗", "⚠", "?", "-" or "✔"
		Result string `json:"result" binding:"required"`
	}

	queryParams := queryData{}
	if err := c.ShouldBindJSON(&queryParams); err != nil {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{Err: err.Error()})
		return
	}

	if !slices.Contains(acknowledgeDays, queryParams.Days) {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{Err: fmt.Sprintf("days must be one of %v", acknowledgeDays)})
		return
	}

	if !slices.Contains(database.ActionResults, queryParams.Result) {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{Err: fmt.Sprintf("result must be one of %v", database.ActionResults)})
		return
	}

	until := time.Now().UTC().AddDate(0, 0, queryParams.Days)
	if err := database.AcknowledgeAction(c, queryParams.URL, until, queryParams.Result, acknowledgerFromContext(c)); err != nil {
		logrus.Errorf("acknowledging an open action: %s", err)
		respondWithError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// UnacknowledgeAction removes the acknowledgement of an open action
// @Summary Unacknowledge an open action
// @Description Bring an acknowledged open action back in the search
// @Tags Pipeline Actions
// @Produce json
// @Param url query string true "Link of the action"
// @Success 204
// @Failure 400 {object} DefaultResponseModel
// @Failure 401 {object} DefaultResponseModel
// @Failure 403 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/actions/ack [delete]
func UnacknowledgeAction(c *gin.Context) {
	url := c.Query("url")
	if url == "" {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{Err: "url is required"})
		return
	}

	if err := database.UnacknowledgeAction(c, url); err != nil {
		logrus.Errorf("unacknowledging an open action: %s", err)
		respondWithError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// acknowledgerFromContext returns the name shown to other viewers for whoever
// acknowledged an action. It is empty for an unauthenticated request, which is every
// request when authentication is off.
func acknowledgerFromContext(c *gin.Context) string {
	principal, ok := principalFromContext(c)
	if !ok {
		return ""
	}

	if principal.Name != "" {
		return principal.Name
	}

	return principal.Subject
}
