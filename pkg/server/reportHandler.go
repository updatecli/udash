package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/updatecli/pkg/core/reports"
)

// CreatePipelineReport insert a new report into the database
func CreatePipelineReport(c *gin.Context) {

	var err error
	var p reports.Report

	if err := c.BindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err})
		log.Fatal(err)
		return
	}

	// Init scms table if needed
	for i := range p.Targets {
		branch := p.Targets[i].Scm.Branch.Target
		url := p.Targets[i].Scm.URL

		got, err := dbGetScm("", url, branch)
		if err != nil {
			logrus.Errorf("get scm data: %s", err)
			continue
		}

		if len(got) == 0 && url != "" && branch != "" {
			_, err := dbInsertSCM(url, branch)
			if err != nil {
				logrus.Errorf("insert scm data: %s", err)
				continue
			}
		}
	}

	newReportID, err := dbInsertReport(p)
	if err != nil {
		logrus.Errorf("insert reports: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "report successfully published",
		"reportid": newReportID,
	})
}

// DeletePipelineReport removes a pipeline report from the database
func DeletePipelineReport(c *gin.Context) {
	id := c.Param("id")

	if err := dbDeleteReport(id); err != nil {
		logrus.Errorf("query failed: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Pipeline report deleted successfully",
	})
}

// FindAllPipelineReports returns all pipeline reports from the database
func FindAllPipelineReports(c *gin.Context) {

	type data struct {
		ID        string
		Name      string
		Result    string
		CreatedAt string
		UpdatedAt string
	}

	queryParams := c.Request.URL.Query()

	scmid := queryParams.Get("scmid")
	query := ""

	switch scmid {
	case "":
		query = `
WITH filtered_reports AS (
	SELECT id, data,created_at, updated_at
	FROM pipelinereports
	WHERE
	  updated_at >  current_date - interval '%d day'
)
SELECT id, data, created_at, updated_at
FROM filtered_reports
ORDER BY updated_at DESC`
		query = fmt.Sprintf(query, monitoringDurationDays)

	case "none", "null", "nil":
		query = `
WITH filtered_reports AS (
	SELECT id, data, created_at, updated_at
	FROM pipelinereports
	WHERE
	  NOT jsonb_path_exists(data::jsonb, '$.Targets[*].* ? (@.Scm.URL  != "" && @.Scm.Branch.Target != "")') AND
      updated_at >  current_date - interval '%d day'
)
SELECT DISTINCT ON (data ->> 'Name')
	id,
	data,
	created_at,
	updated_at
FROM filtered_reports
ORDER BY (data ->> 'Name'), updated_at DESC;`

		query = fmt.Sprintf(query, monitoringDurationDays)

	default:
		scm, err := dbGetScm(scmid, "", "")
		if err != nil {
			logrus.Errorf("get scm data: %s", err)
			return
		}

		switch len(scm) {
		case 0:
			logrus.Errorf("scm data not found")

		case 1:
			query = `
WITH filtered_reports AS (
	SELECT id, data, created_at, updated_at
	FROM pipelinereports
	WHERE
		jsonb_path_exists(data::jsonb, '$.Targets[*].* ? (@.Scm.URL  == "%s" && @.Scm.Branch.Target == "%s")') AND
		updated_at >  current_date - interval '%d day'
)

SELECT DISTINCT ON (data ->> 'Name')
	id,
	data,
	created_at,
	updated_at
FROM filtered_reports
ORDER BY (data ->> 'Name'), updated_at DESC;
`

			query = fmt.Sprintf(query, scm[0].URL, scm[0].Branch, monitoringDurationDays)

		default:
			// Normally we should never have multiple scms with the same id
			// so we should never reach this point.
			logrus.Errorf("multiple scms found")
		}
	}

	rows, err := database.DB.Query(context.Background(), query)
	if err != nil {
		logrus.Errorf("query failed: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err,
		})
		return
	}

	dataset := []data{}

	for rows.Next() {
		p := PipelineReportRow{}

		err = rows.Scan(&p.ID, &p.Pipeline, &p.Created_at, &p.Updated_at)
		if err != nil {
			logrus.Errorf("parsing result: %s", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": err,
			})
			return
		}

		data := data{
			ID:        p.ID.String(),
			Name:      p.Pipeline.Name,
			Result:    p.Pipeline.Result,
			CreatedAt: p.Created_at.String(),
			UpdatedAt: p.Created_at.String(),
		}

		dataset = append(dataset, data)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": dataset,
	})
}

// FindPipelineReportByID returns the latest pipeline report for a specific ID
func FindPipelineReportByID(c *gin.Context) {
	id := c.Param("id")

	data, err := dbSearchReport(id)
	if err != nil {
		logrus.Errorf("parsing result: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err})
		return
	}

	nbReportsByID, err := dbSearchNumberOfReportsByID(data.Pipeline.ID)
	if err != nil {
		logrus.Errorf("getting number of reports by name: %s", err)
	}

	latestReportByID, err := dbSearchLatestReportByID(data.Pipeline.ID)
	if err != nil {
		logrus.Errorf("getting latest report by name: %s", err)
	}

	switch err {
	case nil:
		c.JSON(http.StatusCreated, gin.H{
			"message":          "success!",
			"data":             *data,
			"nbReportsByID":    nbReportsByID,
			"latestReportByID": latestReportByID,
		})
	case pgx.ErrNoRows:
		c.JSON(http.StatusNotFound, gin.H{})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"message": err})
		return
	}
}

func UpdatePipelineReport(c *gin.Context) {
	c.JSON(http.StatusCreated, gin.H{"message": "pipeline update is not supported yet!"})
}
