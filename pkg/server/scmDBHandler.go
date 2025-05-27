package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/model"
)

type ListSCMsResponse struct {
	// SCMs is a list of SCMs.
	SCMs []model.SCM `json:"scms"`
}

// ListSCMs returns a list of SCMs from the database.
// @Summary List SCMs
// @Description List SCMs data from the database
// @Tags SCMs
// @Param scmid query string false "ID of the SCM"
// @Param url query string false "URL of the SCM"
// @Param branch query string false "Branch of the SCM"
// @Param summary query bool false "Return a summary of the SCMs"
// @Success 200 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/scms [get]
func ListSCMs(c *gin.Context) {

	scmid := c.Request.URL.Query().Get("scmid")
	url := c.Request.URL.Query().Get("url")
	branch := c.Request.URL.Query().Get("branch")
	summary := c.Request.URL.Query().Get("summary")

	rows, err := dbGetScm(scmid, url, branch)
	if err != nil {
		logrus.Errorf("searching for scms: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})

		return
	}

	if strings.ToUpper(summary) == "TRUE" {
		FindSCMSummary(c, rows)
		return
	}

	c.JSON(http.StatusOK, ListSCMsResponse{
		SCMs: rows,
	})
}

// ScmSummaryData represents the summary data for a single SCM.
type ScmSummaryData struct {
	// ID is the unique identifier of the SCM.
	ID string `json:"id"`
	// TotalResultByType is a map of result types and their counts.
	TotalResultByType map[string]int `json:"total_result_by_type"`
	// TotalResult is the total number of results for this SCM.
	TotalResult int `json:"total_result"`
}

// ScmBranchData represents a map of branches and their summary data for a single SCM URL.
type ScmBranchData map[string]ScmSummaryData

// FindSCMSummaryResponse represents the response for the FindSCMSummary endpoint.
type FindSCMSummaryResponse struct {
	Data map[string]ScmBranchData `json:"data"`
}

// FindSCMSummary returns a summary of all git repositories detected.
// @Summary Find SCM Summary
// @Description Find SCM Summary of all git repositories detected
// @Tags SCMs
// @Param scmid query string false "ID of the SCM"
// @Param url query string false "URL of the SCM"
// @Param branch query string false "Branch of the SCM"
// @Success 200 {object} FindSCMSummaryResponse
// @Failure 500 {object} DefaultResponseModel
// @Router /api/scms/summary [get]
func FindSCMSummary(c *gin.Context, scmRows []model.SCM) {

	type data struct {
		// URLS is a map of scmURLS where the key is the scm URL.
		Data map[string]ScmBranchData
	}

	dataset := data{}

	query := ""
	for _, row := range scmRows {

		scmID := row.ID
		scmURL := row.URL
		scmBranch := row.Branch

		if scmBranch == "" || scmURL == "" {
			logrus.Debugf("skipping scm %s, missing branch or url", row.ID)
			continue
		}

		query = `
WITH filtered_reports AS (
	SELECT id, data, updated_at
	FROM pipelineReports
	WHERE 
		( target_db_scm_ids && '{ %q }' ) AND 
		( updated_at >  current_date - interval '%d day' )
)
SELECT DISTINCT ON (data ->> 'Name')
	id,
	(data ->> 'Result')

FROM filtered_reports
ORDER BY (data ->> 'Name'), updated_at DESC;
`

		query = fmt.Sprintf(query, scmID, monitoringDurationDays)

		rows, err := database.DB.Query(context.Background(), query)
		if err != nil {
			logrus.Errorf("query failed: %s", err)
			c.JSON(http.StatusInternalServerError, DefaultResponseModel{
				Err: err.Error(),
			})
			return
		}

		if dataset.Data == nil {
			dataset.Data = make(map[string]ScmBranchData)
		}

		if dataset.Data[scmURL] == nil {
			dataset.Data[scmURL] = make(map[string]ScmSummaryData)
		}

		d := ScmSummaryData{
			ID:                scmID.String(),
			TotalResultByType: make(map[string]int),
		}

		dataset.Data[scmURL][scmBranch] = d

		for rows.Next() {

			id := ""
			result := ""

			err = rows.Scan(&id, &result)
			if err != nil {
				logrus.Errorf("parsing result: %s", err)
				c.JSON(http.StatusInternalServerError, DefaultResponseModel{
					Err: err.Error(),
				})
				return
			}

			resultFound := false
			for r := range dataset.Data[scmURL][scmBranch].TotalResultByType {
				if r == result {
					dataset.Data[scmURL][scmBranch].TotalResultByType[r]++
					resultFound = true
				}
			}

			if !resultFound {
				dataset.Data[scmURL][scmBranch].TotalResultByType[result] = 1
			}
		}

		scmData := dataset.Data[scmURL][scmBranch]
		for r := range scmData.TotalResultByType {
			scmData.TotalResult += scmData.TotalResultByType[r]
		}
		dataset.Data[scmURL][scmBranch] = scmData
	}

	c.JSON(http.StatusOK, FindSCMSummaryResponse{
		Data: dataset.Data,
	})
}
