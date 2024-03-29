package server

import (
	"context"
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

	newReportID, err := dbInsertReport(p)
	if err != nil {
		logrus.Errorf("query failed: %s", err)
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

	query := "SELECT * FROM pipelineReports ORDER BY updated_at DESC FETCH FIRST 1000 ROWS ONLY"

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
		p := PipelineRow{}

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
