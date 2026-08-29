package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/model"
	"github.com/updatecli/updatecli/pkg/core/reports"
)

type CreatePipelineReportResponse struct {
	Message  string `json:"message"`
	ReportID string `json:"reportid"`
}

// CreatePipelineReport insert a new report into the database
// @Summary Create a new pipeline report
// @Description Create a new pipeline report in the database
// @Tags Pipeline Reports
// @Accept json
// @Produce json
// @Success 201 {object} CreatePipelineReportResponse
// @Failure 400 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/reports [post]
func CreatePipelineReport(c *gin.Context) {
	var p reports.Report

	if err := c.BindJSON(&p); err != nil {
		logrus.Errorf("failed to read json body: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	newReportID, err := database.InsertReport(c, p, publisherFromContext(c))
	if err != nil {
		logrus.Errorf("insert reports: %s", err)
		c.JSON(
			http.StatusInternalServerError,
			DefaultResponseModel{
				Err: err.Error(),
			})
		return
	}

	c.JSON(http.StatusCreated, CreatePipelineReportResponse{
		Message:  "report successfully published",
		ReportID: newReportID,
	})
}

// DeletePipelineReport removes a pipeline report from the database
// @Summary Delete a pipeline report
// @Description Delete a pipeline report from the database
// @Tags Pipeline Reports
// @Param id path string true "Report ID"
// @Success 200 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/reports/{id} [delete]
func DeletePipelineReport(c *gin.Context) {
	id := c.Param("id")

	if err := database.DeleteReport(c, id); err != nil {
		logrus.Errorf("query failed: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, DefaultResponseModel{
		Message: "Pipeline report deleted successfully",
	})
}

type GetPipelineReportsResponse struct {
	Data       []database.SearchLatestReportData `json:"data"`
	TotalCount int                               `json:"total_count"`
}

// SearchPipelineReports returns all pipeline reports from the database using advanced filtering
// @Summary Search pipeline reports
// @Description Search pipeline reports in the database using advanced filtering
// @Param limit query string false "Limit the number of reports returned, default is 100"
// @Param page query string false "Page number for pagination, default is 1"
// @Tags Pipeline Reports
// @Accept json
// @Produce json
// @Success 200 {object} GetPipelineReportsResponse
// @Failure 400 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/reports/search [post]
func SearchPipelineReports(c *gin.Context) {

	type queryData struct {
		// ScmID is the ID of the SCM to filter reports by
		// This is optional and can be used to filter reports by a specific SCM
		ScmID string `json:"scmid"`
		// SourceID is the ID of the source to filter reports by
		// This is optional and can be used to filter reports by a specific source
		SourceID string `json:"sourceid"`
		// ConditionID is the ID of the condition to filter reports by
		// This is optional and can be used to filter reports by a specific condition
		ConditionID string `json:"conditionid"`
		// TargetID is the ID of the target to filter reports by
		// This is optional and can be used to filter reports by a specific target
		TargetID string `json:"targetid"`
		// Limit is the maximum number of reports to return
		// This is optional and can be used to limit the number of reports returned
		Limit int `json:"limit"`
		// Page is the page number for pagination
		// This is optional and can be used to paginate the results
		Page int `json:"page"`
		// StartTime is the start time for the time range filter
		// This is optional and can be used to filter reports by a specific start time
		// Time format is RFC3339: 2006-01-02T15:04:05Z07:00
		StartTime string `json:"start_time"`
		// EndTime is the end time for the time range filter
		// This is optional and can be used to filter reports by a specific end time
		// Time format is RFC3339: 2006-01-02T15:04:05Z07:00
		EndTime string `json:"end_time"`
		// Latest indicates whether to return only the latest report per pipeline ID
		// This is optional and defaults to false
		Latest bool `json:"latest"`
		// Labels is a map of labels to filter reports by
		Labels map[string]string `json:"labels,omitempty"`
		// Results is a list of pipeline results to filter reports by, such as
		// "✔", "✗", "⚠" or "-". A report matches when its result is any of them.
		// This is optional and an empty list does not filter anything out.
		Results []string `json:"results,omitempty"`
		// OpenAction filters reports by whether they carry an action left open, such as a
		// pull request still waiting to be merged. This is optional: unset does not filter
		// anything out, true only keeps the reports with an open action and false only the
		// ones without.
		//
		// Combined with results it isolates the pipelines which succeeded because their
		// change is already waiting in a pull request, which a result alone cannot express:
		// {"results": ["✔"], "open_action": true}.
		OpenAction *bool `json:"open_action,omitempty"`
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

	dataset, totalCount, err := database.SearchLatestReports(
		database.SearchLatestReportsParams{
			Ctx:         c,
			ScmID:       queryParams.ScmID,
			SourceID:    queryParams.SourceID,
			ConditionID: queryParams.ConditionID,
			TargetID:    queryParams.TargetID,
			Options:     database.ReportSearchOptions{Days: monitoringDurationDays},
			StartTime:   queryParams.StartTime,
			EndTime:     queryParams.EndTime,
			Limit:       queryParams.Limit,
			Page:        queryParams.Page,
			Latest:      queryParams.Latest,
			Labels:      queryParams.Labels,
			Results:     queryParams.Results,
			OpenAction:  queryParams.OpenAction,
		},
	)
	if err != nil {
		logrus.Errorf("searching for latest report: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, GetPipelineReportsResponse{
		Data:       dataset,
		TotalCount: totalCount,
	})
}

// SearchPipelineReportsSummaryRequest represents the filters used to summarize
// pipeline reports.
type SearchPipelineReportsSummaryRequest struct {
	// Metric is what the reports are counted by. It defaults to "result", which is
	// the only value supported so far.
	Metric string `json:"metric,omitempty"`
	// Granularity is the size of the time buckets, one of "hour", "day", "week" or
	// "month". It defaults to "day".
	Granularity string `json:"granularity,omitempty"`
	// Days is the number of days to summarize, today included.
	// It defaults to 7 and is ignored when hours, or start_time and end_time, are provided.
	Days int `json:"days,omitempty"`
	// Hours is the number of hours to summarize, the current hour included.
	// It cannot be combined with days and is ignored when start_time and end_time are provided.
	Hours int `json:"hours,omitempty"`
	// ScmID is the ID of the SCM to filter reports by.
	// Use "none" to only count the reports which are not attached to any SCM.
	ScmID string `json:"scmid,omitempty"`
	// Labels is a map of labels to filter reports by.
	Labels map[string]string `json:"labels,omitempty"`
	// Results is a list of pipeline results to filter reports by, such as
	// "✔", "✗", "⚠" or "-". A report is counted when its result is any of them.
	// An empty list does not filter anything out.
	Results []string `json:"results,omitempty"`
	// OpenAction filters reports by whether they carry an action left open, such as a
	// pull request still waiting to be merged. This is optional: unset does not filter
	// anything out, true only counts the reports with an open action and false only the
	// ones without.
	//
	// The same breakdown is reported without filtering anything out under the open_actions
	// key of every bucket.
	OpenAction *bool `json:"open_action,omitempty"`
	// StartTime is the start time for the time range filter.
	// Time format is: 2006-01-02 15:04:05Z07:00
	StartTime string `json:"start_time,omitempty"`
	// EndTime is the end time for the time range filter.
	// Time format is: 2006-01-02 15:04:05Z07:00
	EndTime string `json:"end_time,omitempty"`
}

// SearchPipelineReportsSummaryResponse represents the response for the
// SearchPipelineReportsSummary endpoint.
type SearchPipelineReportsSummaryResponse struct {
	// Metric is the metric the reports were counted by.
	Metric string `json:"metric"`
	// Granularity is the size of the time buckets of the entries.
	Granularity string `json:"granularity"`
	// Data contains one entry per time bucket, ordered from the oldest to the most recent one.
	Data []database.ReportResultSummaryEntry `json:"data"`
	// TotalCount is the total number of reports matching the query.
	TotalCount int `json:"total_count"`
}

// SearchPipelineReportsSummary returns the number of pipeline reports per result, per time bucket.
// @Summary Summarize pipeline reports
// @Description Return the number of pipeline reports per result for each time bucket of the requested time range.
// @Description Buckets are UTC hours, UTC calendar days, ISO weeks or calendar months depending on the
// @Description granularity, and the date of an entry is the start of its bucket, formatted as RFC3339.
// @Description Every report is counted, including several reports of the same pipeline, and buckets without
// @Description any report are returned with a zeroed entry.
// @Tags Pipeline Reports
// @Accept json
// @Produce json
// @Param body body SearchPipelineReportsSummaryRequest true "Summary filters"
// @Success 200 {object} SearchPipelineReportsSummaryResponse
// @Failure 400 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/reports/summary [post]
func SearchPipelineReportsSummary(c *gin.Context) {
	queryParams := SearchPipelineReportsSummaryRequest{}

	if err := c.ShouldBindJSON(&queryParams); err != nil {
		logrus.Errorf("failed to read json body: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	metric := queryParams.Metric
	if metric == "" {
		metric = summaryMetricResult
	}

	if metric != summaryMetricResult {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: ErrInvalidMetricParam,
		})
		return
	}

	granularity := database.SummaryGranularity(queryParams.Granularity)
	if granularity == "" {
		granularity = database.SummaryGranularityDay
	}

	if !granularity.IsValid() {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: ErrInvalidGranularityParam,
		})
		return
	}

	if queryParams.Days != 0 && queryParams.Hours != 0 {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: ErrConflictingWindowParams,
		})
		return
	}

	hours := queryParams.Hours
	if hours < 0 || hours > maxMonitoringDurationDays*24 {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: ErrInvalidHoursParam,
		})
		return
	}

	days := queryParams.Days
	switch {
	case days == 0:
		// Only fall back to the default window when no window was asked for at all,
		// otherwise it would silently override hours.
		if hours == 0 {
			days = monitoringDurationDays
		}
	case days < 0 || days > maxMonitoringDurationDays:
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: ErrInvalidDaysParam,
		})
		return
	}

	if err := validateTimeRangeParams(queryParams.StartTime, queryParams.EndTime); err != nil {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	dataset, totalCount, err := database.SearchReportsSummary(
		database.ReportSummaryParams{
			Ctx:         c,
			Days:        days,
			Hours:       hours,
			Granularity: granularity,
			MaxDays:     maxMonitoringDurationDays,
			MaxBuckets:  maxSummaryBuckets,
			ScmID:       queryParams.ScmID,
			Labels:      queryParams.Labels,
			Results:     queryParams.Results,
			OpenAction:  queryParams.OpenAction,
			StartTime:   queryParams.StartTime,
			EndTime:     queryParams.EndTime,
		},
	)
	if err != nil {
		// An explicit time range bypasses the days validation above, so this is the
		// only place a range wider than the limit can be caught.
		if errors.Is(err, database.ErrSummaryRangeTooWide) {
			c.JSON(http.StatusBadRequest, DefaultResponseModel{
				Err: ErrTimeRangeTooWide,
			})
			return
		}

		// The number of buckets depends on the granularity, which the validation above
		// cannot account for on its own.
		if errors.Is(err, database.ErrSummaryTooManyBuckets) {
			c.JSON(http.StatusBadRequest, DefaultResponseModel{
				Err: ErrTooManyBuckets,
			})
			return
		}

		logrus.Errorf("summarizing reports: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, SearchPipelineReportsSummaryResponse{
		Metric:      metric,
		Granularity: string(granularity),
		Data:        dataset,
		TotalCount:  totalCount,
	})
}

// ListPipelineReports returns all pipeline reports from the database
// @Summary List all pipeline reports
// @Description List all pipeline reports from the database
// @Tags Pipeline Reports
// @Param scmid query string false "SCM ID"
// @Param limit query string false "Limit the number of reports returned, default is 100"
// @Param page query string false "Page number for pagination, default is 1"
// @Param start_time query string false "Start time for filtering reports (RFC3339 format)"
// @Param end_time query string false "End time for filtering reports (RFC3339 format)"
// @Param latest query string false "Only return the latest report per pipeline ID, default is true"
// @Accept json
// @Produce json
// @Success 200 {object} GetPipelineReportsResponse
// @Failure 400 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/reports [get]
func ListPipelineReports(c *gin.Context) {
	queryParams := c.Request.URL.Query()
	scmID := queryParams.Get("scmid")
	startTime := queryParams.Get("start_time")
	endTime := queryParams.Get("end_time")

	// A value which cannot be parsed is rejected rather than ignored: falling through
	// used to leave latest at false, which is the opposite of the documented default.
	latest := true
	if lateststr := queryParams.Get("latest"); lateststr != "" {
		parsedLatest, err := strconv.ParseBool(lateststr)
		if err != nil {
			c.JSON(http.StatusBadRequest, DefaultResponseModel{
				Err: ErrInvalidLatestParam,
			})
			return
		}

		latest = parsedLatest
	}

	if err := validateTimeRangeParams(startTime, endTime); err != nil {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	limit, page, err := getPaginationParamFromURLQuery(c)
	if err != nil {
		logrus.Errorf("getting pagination params: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: ErrInvalidPaginationParams + ": " + err.Error(),
		})
		return
	}

	dataset, totalCount, err := database.SearchLatestReports(
		database.SearchLatestReportsParams{
			Ctx:         c,
			ScmID:       scmID,
			SourceID:    "",
			ConditionID: "",
			TargetID:    "",
			Options:     database.ReportSearchOptions{Days: monitoringDurationDays},
			StartTime:   startTime,
			EndTime:     endTime,
			Limit:       limit,
			Page:        page,
			Latest:      latest,
		},
	)

	if err != nil {
		logrus.Errorf("searching for latest report: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, GetPipelineReportsResponse{
		Data:       dataset,
		TotalCount: totalCount,
	})
}

type GetPipelineReportByIDResponse struct {
	Message          string               `json:"message"`
	Data             model.PipelineReport `json:"data"`
	NBReportsByID    int                  `json:"nbReportsByID"`
	LatestReportByID model.PipelineReport `json:"latestReportByID"`
}

// GetPipelineReportByID returns the latest pipeline report for a specific ID
// @Summary Get a pipeline report by ID
// @Description Get the latest pipeline report for a specific ID
// @Tags Pipeline Reports
// @Param id path string true "Report ID"
// @Success 200 {object} GetPipelineReportByIDResponse
// @Failure 404 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/reports/{id} [get]
func GetPipelineReportByID(c *gin.Context) {
	id := c.Param("id")
	data, err := database.SearchReport(c, id)
	if err != nil {
		logrus.Errorf("parsing result: %s", err)
		statusCode := http.StatusInternalServerError
		if errors.Is(err, pgx.ErrNoRows) {
			statusCode = http.StatusNotFound
		}
		c.JSON(
			statusCode,
			DefaultResponseModel{
				Err: err.Error(),
			})
		return
	}

	nbReportsByID, err := database.SearchNumberOfReportsByPipelineID(c, data.Pipeline.ID)
	if err != nil {
		logrus.Errorf("getting number of reports by name: %s", err)
		c.JSON(
			http.StatusInternalServerError,
			DefaultResponseModel{
				Err: err.Error(),
			})
		return
	}

	latestReportByID, err := database.SearchLatestReportByPipelineID(c, data.Pipeline.ID)
	if err != nil {
		logrus.Errorf("getting latest report by name: %s", err)
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(
				http.StatusNotFound,
				DefaultResponseModel{
					Err: "not found",
				},
			)
			return
		}

		c.JSON(
			http.StatusInternalServerError,
			DefaultResponseModel{
				Err: err.Error(),
			},
		)
		return
	}

	c.JSON(
		http.StatusOK,
		GetPipelineReportByIDResponse{
			Message:          "success!",
			Data:             *data,
			NBReportsByID:    nbReportsByID,
			LatestReportByID: *latestReportByID,
		})
}

// UpdatePipelineReport updates a pipeline report in the database
// Note: This endpoint is not supported yet.
// @Summary Update a pipeline report
// @Description Update a pipeline report in the database. Please note that this endpoint is not supported yet.
// @Tags Pipeline Reports
// @Param id path string true "Report ID"
// @Accept json
// @Produce json
// @Success 200 {object} DefaultResponseModel
// @Router /api/pipeline/reports/{id} [put]
func UpdatePipelineReport(c *gin.Context) {
	c.JSON(
		http.StatusOK,
		DefaultResponseModel{
			Message: "pipeline update is not supported yet!",
		})
}
