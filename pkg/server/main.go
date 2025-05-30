package server

import (
	"net/http"
	"os"
	"strings"

	_ "github.com/updatecli/udash/docs"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/version"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger" // swagger middleware
)

type Server struct {
	Options Options
}

type DefaultResponseModel struct {
	Message string `json:"message,omitempty"`
	Err     string `json:"error,omitempty"`
}

// Landing is the landing page handler.
// @Summary Landing page
// @Description Landing page of the API
// @Tags Landing
// @Success 200
// @Router /api/ [get]
func Landing(c *gin.Context) {
	c.JSON(http.StatusOK, DefaultResponseModel{
		Message: "Welcome to the Udash API",
	})
}

// Ping is a simple endpoint to check if the server is running.
// @Summary Ping the API
// @Description Ping the API to check if it's running
// @Tags Ping
// @Success 200 {object} DefaultResponseModel
// @Router /api/ping [get]
func Ping(c *gin.Context) {
	c.JSON(http.StatusOK, DefaultResponseModel{
		Message: "pong",
	})
}

type AboutResponseModel struct {
	Version struct {
		Golang    string `json:"golang,omitempty"`
		API       string `json:"api,omitempty"`
		BuildTime string `json:"buildTime,omitempty"`
	} `json:"version,omitempty"`
}

// About returns the version information of the API.
// @Summary About the API
// @Description Get version information of the API
// @Tags About
// @Success 200 {object} AboutResponseModel
func About(c *gin.Context) {
	resp := AboutResponseModel{}
	resp.Version.API = version.Version
	resp.Version.Golang = version.GoVersion
	resp.Version.BuildTime = version.BuildTime

	c.JSON(http.StatusOK, resp)
}

// @title Udash API
// @version 1.0
// @description API for managing Updatecli pipeline reports.
// @BasePath /api/
func (s *Server) Run() {
	r := gin.Default()

	// Init Server Option
	s.Options.Init()

	r.GET("/api/", Landing)
	r.GET("/api/ping", Ping)
	r.GET("/api/about", About)

	r.GET("/api/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	switch strings.ToLower(s.Options.Auth.Mode) {
	case "oauth":
		r.GET("/api/pipeline/scms", checkJWT(), ListSCMs)
		r.GET("/api/pipeline/reports", checkJWT(), ListPipelineReports)
		r.POST("/api/pipeline/reports/search", checkJWT(), SearchPipelineReports)
		r.GET("/api/pipeline/reports/:id", checkJWT(), GetPipelineReportByID)
		r.GET("/api/pipeline/config/kinds", checkJWT(), SearchConfigKinds)
		r.GET("/api/pipeline/config/sources", checkJWT(), ListConfigSources)
		r.POST("/api/pipeline/config/sources/search", checkJWT(), SearchConfigSources)
		r.GET("/api/pipeline/config/conditions", checkJWT(), ListConfigConditions)
		r.POST("/api/pipeline/config/conditions/search", checkJWT(), SearchConfigConditions)
		r.GET("/api/pipeline/config/targets", checkJWT(), ListConfigTargets)
		r.POST("/api/pipeline/config/targets/search", checkJWT(), SearchConfigTargets)
		if !s.Options.DryRun {
			r.POST("/api/pipeline/reports", checkJWT(), CreatePipelineReport)
			r.PUT("/api/pipeline/reports/:id", checkJWT(), UpdatePipelineReport)
			r.DELETE("/api/pipeline/reports/:id", checkJWT(), DeletePipelineReport)
		}

	case "", "none":
		r.GET("/api/pipeline/scms", ListSCMs)
		r.GET("/api/pipeline/reports", ListPipelineReports)
		r.POST("/api/pipeline/reports/search", SearchPipelineReports)
		r.GET("/api/pipeline/reports/:id", GetPipelineReportByID)
		r.GET("/api/pipeline/config/kinds", SearchConfigKinds)
		r.GET("/api/pipeline/config/sources", ListConfigSources)
		r.POST("/api/pipeline/config/sources/search", SearchConfigSources)
		r.GET("/api/pipeline/config/conditions", ListConfigConditions)
		r.POST("/api/pipeline/config/conditions/search", SearchConfigConditions)
		r.GET("/api/pipeline/config/targets", ListConfigTargets)
		r.POST("/api/pipeline/config/targets/search", SearchConfigTargets)
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
