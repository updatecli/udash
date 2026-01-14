package server

import (
	"database/sql"
	"errors"
	"log"
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
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/reports [post]
func CreatePipelineReport(c *gin.Context) {
	var p reports.Report

	if err := c.BindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err})
		log.Fatal(err)
		return
	}

	newReportID, err := database.InsertReport(c, p)
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
// @Success 201 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/reports/{id} [delete]
func DeletePipelineReport(c *gin.Context) {
	id := c.Param("id")

	if err := database.DeleteReport(c, id); err != nil {
		logrus.Errorf("query failed: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err})
		return
	}

	c.JSON(http.StatusCreated, DefaultResponseModel{
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
	}

	queryParams := queryData{}

	if err := c.ShouldBindJSON(&queryParams); err != nil {
		logrus.Errorf("failed to read json body: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	dataset, totalCount, err := database.SearchLatestReports(
		c, queryParams.ScmID, queryParams.SourceID, queryParams.ConditionID,
		queryParams.TargetID, database.ReportSearchOptions{Days: monitoringDurationDays},
		queryParams.Limit, queryParams.Page,
		queryParams.StartTime, queryParams.EndTime,
		queryParams.Latest,
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

// ListPipelineReports returns all pipeline reports from the database
// @Summary List all pipeline reports
// @Description List all pipeline reports from the database
// @Tags Pipeline Reports
// @Param scmid query string false "SCM ID"
// @Param limit query string false "Limit the number of reports returned, default is 100"
// @Param page query string false "Page number for pagination, default is 1"
// @Param start_time query string false "Start time for filtering reports (RFC3339 format)"
// @Param end_time query string false "End time for filtering reports (RFC3339 format)"
// @Accept json
// @Produce json
// @Success 200 {object} GetPipelineReportsResponse
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/reports [get]
func ListPipelineReports(c *gin.Context) {
	queryParams := c.Request.URL.Query()
	scmID := queryParams.Get("scmid")
	startTime := queryParams.Get("start_time")
	endTime := queryParams.Get("end_time")
	lateststr := queryParams.Get("latest")

	latest, err := strconv.ParseBool(lateststr)
	if err != nil {
		logrus.Warningf("ignoring latest param due to: %s", err)
	}

	limit, page, err := getPaginationParamFromURLQuery(c)
	if err != nil {
		logrus.Errorf("getting pagination params: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: "invalid pagination parameters",
		})
		return
	}

	dataset, totalCount, err := database.SearchLatestReports(
		c, scmID, "", "", "",
		database.ReportSearchOptions{Days: monitoringDurationDays},
		limit,
		page,
		startTime, endTime,
		latest,
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
// @Success 201 {object} GetPipelineReportByIDResponse
// @Failure 404 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/reports/{id} [get]
func GetPipelineReportByID(c *gin.Context) {
	id := c.Param("id")
	data, err := database.SearchReport(c, id)
	if err != nil {
		logrus.Errorf("parsing result: %s", err)
		statusCode := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
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
	}

	latestReportByID, err := database.SearchLatestReportByPipelineID(c, data.Pipeline.ID)
	if err != nil {
		logrus.Errorf("getting latest report by name: %s", err)
	}

	switch err {
	case nil:
		c.JSON(
			http.StatusOK,
			GetPipelineReportByIDResponse{
				Message:          "success!",
				Data:             *data,
				NBReportsByID:    nbReportsByID,
				LatestReportByID: *latestReportByID,
			})
	case pgx.ErrNoRows:
		c.JSON(
			http.StatusNotFound,
			DefaultResponseModel{
				Err: "not found",
			},
		)
	default:
		c.JSON(
			http.StatusInternalServerError,
			DefaultResponseModel{
				Message: err.Error(),
			})
		return
	}
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
