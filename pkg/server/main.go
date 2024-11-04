package server

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/version"
)

type Server struct {
	Options Options
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

func (s *Server) Run() {
	r := gin.Default()

	// Init Server Option
	s.Options.Init()

	r.GET("/api/", Landing)
	r.GET("/api/ping", Ping)
	r.GET("/api/about", About)

	switch strings.ToLower(s.Options.Auth.Mode) {
	case "oauth":
		r.GET("/api/pipeline/scms", checkJWT(), FindSCM)
		r.GET("/api/pipeline/reports", checkJWT(), FindAllPipelineReports)
		r.GET("/api/pipeline/reports/:id", checkJWT(), FindPipelineReportByID)
		if !s.Options.DryRun {
			r.POST("/api/pipeline/reports", checkJWT(), CreatePipelineReport)
			r.PUT("/api/pipeline/reports/:id", checkJWT(), UpdatePipelineReport)
			r.DELETE("/api/pipeline/reports/:id", checkJWT(), DeletePipelineReport)
		}

	case "", "none":
		r.GET("/api/pipeline/scms", FindSCM)
		r.GET("/api/pipeline/reports", FindAllPipelineReports)
		r.GET("/api/pipeline/reports/:id", FindPipelineReportByID)
		if !s.Options.DryRun {
			r.POST("/api/pipeline/reports", CreatePipelineReport)
			r.PUT("/api/pipeline/reports/:id", UpdatePipelineReport)
			r.DELETE("/api/pipeline/reports/:id", DeletePipelineReport)
		}

	default:
		logrus.Errorf("Authentication mode %q not supported", s.Options.Auth.Mode)
		os.Exit(1)
	}

	// listen and server on 0.0.0.0:8080
	if err := r.Run(); err != nil {
		logrus.Errorln(err)
		os.Exit(1)
	}
}
