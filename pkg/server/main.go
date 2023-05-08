package server

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/olblak/udash/pkg/database"
	"github.com/olblak/udash/pkg/version"
	"github.com/sirupsen/logrus"
)

type Options struct {
}

type Server struct {
	Options Options
}

func Create(c *gin.Context) {

	var err error
	var p PipelineReport

	if err := c.BindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err})
		log.Fatal(err)
		return
	}

	query := "INSERT INTO pipelines (data) VALUES ($1)"

	_, err = database.DB.Exec(context.Background(), query, p)
	if err != nil {
		logrus.Errorf("query failed: %w", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Posted successfully"})
}

func Delete(c *gin.Context) {
	var err error

	id := c.Param("id")

	query := "DELETE FROM pipelines WHERE id=$1"

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

	c.JSON(http.StatusCreated, gin.H{"message": "Pipeline deleted successfully"})
}

func FindAll(c *gin.Context) {

	type data struct {
		ID        string
		Name      string
		Result    string
		CreatedAt string
		UpdatedAt string
	}

	query := "SELECT * FROM pipelines ORDER BY updated_at DESC FETCH FIRST 50 ROWS ONLY"

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

func FindByID(c *gin.Context) {
	id := c.Param("id")

	data := PipelineRow{}

	err := database.DB.QueryRow(context.Background(), "select * from pipelines where id=$1", id).Scan(
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

func Landing(c *gin.Context) {
	c.JSON(200, gin.H{"message": "Hey what's up?"})
}

func Ping(c *gin.Context) {
	c.JSON(200, gin.H{"message": "pong"})
}

func About(c *gin.Context) {

	v := struct {
		Golang    string
		Api       string
		BuildTime string
	}{
		Golang:    version.GoVersion,
		Api:       version.Version,
		BuildTime: version.BuildTime,
	}

	c.JSON(200, gin.H{
		"version": v,
	})
}

func Update(c *gin.Context) {

	//ID := c.Param("id")
	// c.JSON(http.StatusCreated, gin.H{"message": "data updated successfully!", "data": res})
}

func (s *Server) Run() {
	r := gin.Default()
	r.GET("/api/", Landing)
	r.GET("/api/ping", Ping)
	r.GET("/api/about", About)
	r.GET("/api/pipelines", FindAll)
	r.GET("/api/pipelines/:id", FindByID)

	r.POST("/api/pipelines", Create)
	r.PUT("/api/pipelines/:id", Update)
	r.DELETE("/api/pipelines/:id", Delete)

	// listen and server on 0.0.0.0:8080
	if err := r.Run(); err != nil {
		logrus.Errorln(err)
		os.Exit(1)
	}
}
