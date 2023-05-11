package server

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/olblak/udash/pkg/version"
	"github.com/sirupsen/logrus"
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

	switch s.Options.Auth.Mode {
	case "jwt":
		r.GET("/api/pipelines", checkJWT(), FindAllPipelines)
		r.GET("/api/pipelines/:id", checkJWT(), FindPipelineByID)

		r.POST("/api/pipelines", checkJWT(), CreatePipeline)
		r.PUT("/api/pipelines/:id", checkJWT(), UpdatePipeline)
		r.DELETE("/api/pipelines/:id", checkJWT(), DeletePipeline)

	case "", "none":
		r.GET("/api/pipelines", FindAllPipelines)
		r.GET("/api/pipelines/:id", FindPipelineByID)

		r.POST("/api/pipelines", CreatePipeline)
		r.PUT("/api/pipelines/:id", UpdatePipeline)
		r.DELETE("/api/pipelines/:id", DeletePipeline)

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
