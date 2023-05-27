package server

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/olblak/udash/pkg/database"
	"github.com/sirupsen/logrus"
)

func CreatePipelineReport(c *gin.Context) {

	var err error
	var p PipelineReport

	if err := c.BindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err})
		log.Fatal(err)
		return
	}

	query := "INSERT INTO pipelineReports (data) VALUES ($1)"

	_, err = database.DB.Exec(context.Background(), query, p)
	if err != nil {
		logrus.Errorf("query failed: %w", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Posted successfully"})
}

func DeletePipelineReport(c *gin.Context) {
	var err error

	id := c.Param("id")

	query := "DELETE FROM pipelineReports WHERE id=$1"

	_, err = database.DB.Exec(context.Background(), query, id)
	if err != nil {
		logrus.Errorf("query failed: %w", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Pipeline report deleted successfully"})
}

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
		logrus.Errorf("query failed: %w", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err})
		return
	}

	dataset := []data{}

	for rows.Next() {
		p := PipelineRow{}

		err = rows.Scan(&p.ID, &p.Pipeline, &p.Created_at, &p.Updated_at)
		if err != nil {
			logrus.Errorf("parsing result: %w", err)
			c.JSON(http.StatusInternalServerError, gin.H{"message": err})
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

	c.JSON(http.StatusOK, gin.H{"data": dataset})
}

func FindPipelineReportByID(c *gin.Context) {
	id := c.Param("id")

	data := PipelineRow{}

	err := database.DB.QueryRow(context.Background(), "select * from pipelineReports where id=$1", id).Scan(
		&data.ID,
		&data.Pipeline,
		&data.Created_at,
		&data.Updated_at,
	)

	if err != nil {
		logrus.Errorf("parsing result: %w", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err})
		return
	}

	switch err {
	case nil:
		c.JSON(http.StatusCreated, gin.H{
			"message": "success!",
			"data":    data,
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
