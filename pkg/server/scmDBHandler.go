package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
)

func FindSCM(c *gin.Context) {

	scmid := c.Request.URL.Query().Get("scmid")
	url := c.Request.URL.Query().Get("url")
	branch := c.Request.URL.Query().Get("branch")
	summary := c.Request.URL.Query().Get("summary")

	rows, err := dbGetScm(scmid, url, branch)
	if err != nil {
		logrus.Errorf("searching for scms: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err,
		})

		return
	}

	if strings.ToUpper(summary) == "TRUE" {
		FindSCMSummary(c, rows)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"scms": rows,
	})
}

// FindSCMSummary returns a summary of all git repositories detected.
func FindSCMSummary(c *gin.Context, scmRows []DatabaseSCMRow) {

	type scmSummaryData struct {
		ID                string         `json:"id"`
		TotalResultByType map[string]int `json:"total_result_by_type"`
		TotalResult       int            `json:"total_result"`
	}

	type scmBranchData map[string]scmSummaryData

	type data struct {
		// URLS is a map of scmURLS where the key is the scm URL.
		Data map[string]scmBranchData
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
	FROM pipelinereports
	WHERE jsonb_path_exists(data::jsonb, '$.Targets[*].* ? (@.Scm.URL  == "%s" && @.Scm.Branch.Target == "%s")') AND updated_at >  current_date - interval '%d day')
SELECT DISTINCT ON (data ->> 'Name')
	id,
	(data ->> 'Result')

FROM filtered_reports
ORDER BY (data ->> 'Name'), updated_at DESC;
`

		query = fmt.Sprintf(query, scmURL, scmBranch, monitoringDurationDays)

		rows, err := database.DB.Query(context.Background(), query)
		if err != nil {
			logrus.Errorf("query failed: %s", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": err,
			})
			return
		}

		if dataset.Data == nil {
			dataset.Data = make(map[string]scmBranchData)
		}

		if dataset.Data[scmURL] == nil {
			dataset.Data[scmURL] = make(map[string]scmSummaryData)
		}

		d := scmSummaryData{
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
				c.JSON(http.StatusInternalServerError, gin.H{
					"message": err,
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

	c.JSON(http.StatusOK, gin.H{
		"data": dataset.Data,
	})
}
