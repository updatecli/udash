package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/model"
)

// ListSCMsResponse represents the response for the ListSCMs endpoint.
type ListSCMsResponse struct {
	// SCMs is a list of SCMs.
	SCMs []model.SCM `json:"scms"`
	// TotalCount is the total number of SCMs matching the query.
	TotalCount int `json:"total_count"`
}

// SearchSCMsRequest represents the filters used to search SCM records.
type SearchSCMsRequest struct {
	// ScmID is the ID of the SCM to filter by.
	ScmID string `json:"scmid"`
	// StartTime is the start time for the time range filter.
	// Time format is RFC3339: 2006-01-02T15:04:05Z07:00
	StartTime string `json:"start_time"`
	// EndTime is the end time for the time range filter.
	// Time format is RFC3339: 2006-01-02T15:04:05Z07:00
	EndTime string `json:"end_time"`
	// Labels filters SCM summaries by report labels.
	Labels map[string]string `json:"labels,omitempty"`
	// URL is the SCM URL to filter by.
	URL string `json:"url,omitempty"`
	// Branch is the SCM branch to filter by.
	Branch string `json:"branch,omitempty"`
	// Summary indicates if the response should contain SCM summary data.
	Summary bool `json:"summary,omitempty"`
	// Limit is the maximum number of SCMs to return.
	Limit int `json:"limit,omitempty"`
	// Page is the page number for pagination.
	Page int `json:"page,omitempty"`
}

// SearchSCMs searches SCMs using JSON filters.
// @Summary Search SCMs
// @Description Search SCM data using JSON filters. When summary is true, the response contains SCM summary data for all matching SCMs.
// @Tags SCMs
// @Accept json
// @Produce json
// @Param body body SearchSCMsRequest true "SCM search filters"
// @Success 200 {object} ListSCMsResponse
// @Failure 400 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/scms/search [post]
func SearchSCMs(c *gin.Context) {
	queryParams := SearchSCMsRequest{}

	if err := c.ShouldBindJSON(&queryParams); err != nil {
		logrus.Errorf("failed to read json body: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	rows, totalCount, err := getSCMRows(c, queryParams)
	if err != nil {
		logrus.Errorf("searching for scms: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})

		return
	}

	if queryParams.Summary {
		findSCMSummary(
			c,
			rows,
			totalCount,
			queryParams.StartTime,
			queryParams.EndTime,
			queryParams.Labels,
		)
		return
	}

	c.JSON(http.StatusOK, ListSCMsResponse{
		SCMs:       rows,
		TotalCount: totalCount,
	})
}

// ListSCMs returns a list of SCMs from the database.
// @Summary List SCMs
// @Description List SCMs data from the database
// @Tags SCMs
// @Param scmid query string false "ID of the SCM"
// @Param url query string false "URL of the SCM"
// @Param branch query string false "Branch of the SCM"
// @Param summary query bool false "Return a summary of the SCMs"
// @Param limit query string false "Limit the number of reports returned, default is 100"
// @Param page query string false "Page number for pagination, default is 1"
// @Param start_time query string false "Start time for filtering SCMs (RFC3339 format)"
// @Param end_time query string false "End time for filtering SCMs (RFC3339 format)"
// @Success 200 {object} ListSCMsResponse
// @Failure 400 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/scms [get]
func ListSCMs(c *gin.Context) {
	queryValues := c.Request.URL.Query()
	summaryValue := queryValues.Get("summary")

	summary := false
	if summaryValue != "" {
		parsedSummary, err := strconv.ParseBool(summaryValue)
		if err != nil {
			c.JSON(http.StatusBadRequest, DefaultResponseModel{
				Err: "invalid summary parameter",
			})
			return
		}

		summary = parsedSummary
	}

	limit, page, err := getPaginationParamFromURLQuery(c)
	if err != nil {
		logrus.Errorf("getting pagination params: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: "invalid pagination parameters",
		})
		return
	}

	rows, totalCount, err := getSCMRows(c, SearchSCMsRequest{
		ScmID:     queryValues.Get("scmid"),
		URL:       queryValues.Get("url"),
		Branch:    queryValues.Get("branch"),
		StartTime: queryValues.Get("start_time"),
		EndTime:   queryValues.Get("end_time"),
		Summary:   summary,
		Limit:     limit,
		Page:      page,
	})
	if err != nil {
		logrus.Errorf("searching for scms: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})

		return
	}

	if summary {
		findSCMSummary(c, rows, totalCount, queryValues.Get("start_time"), queryValues.Get("end_time"), map[string]string{})
		return
	}

	c.JSON(http.StatusOK, ListSCMsResponse{
		SCMs:       rows,
		TotalCount: totalCount,
	})
}

func getSCMRows(c *gin.Context, params SearchSCMsRequest) ([]model.SCM, int, error) {
	limit := params.Limit
	page := params.Page
	if params.Summary {
		limit = 0
		page = 0
	}

	return database.GetSCM(c, params.ScmID, params.URL, params.Branch, limit, page)
}

// FindSCMSummaryResponse represents the response for the FindSCMSummary endpoint.
type FindSCMSummaryResponse struct {
	TotalCount int                                  `json:"total_count"`
	Data       map[string]database.SCMBranchDataset `json:"data"`
}

// findSCMSummary returns a summary of all git repositories detected.
func findSCMSummary(c *gin.Context, scmRows []model.SCM, totalCount int, startTime, endTime string, labels map[string]string) {
	var data map[string]database.SCMBranchDataset

	dataset, err := database.GetSCMSummary(database.GetSCMSummaryParams{
		Ctx:                    c,
		ScmRows:                scmRows,
		TotalCount:             totalCount,
		MonitoringDurationDays: monitoringDurationDays,
		StartTime:              startTime,
		EndTime:                endTime,
		Labels:                 labels,
	})
	if err != nil {
		logrus.Errorf("getting scm summary failed: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	if dataset != nil {
		data = dataset.Data
	}

	c.JSON(http.StatusOK, FindSCMSummaryResponse{
		Data:       data,
		TotalCount: totalCount,
	})
}
