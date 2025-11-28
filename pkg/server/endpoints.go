package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	_ "github.com/updatecli/udash/docs"
	"github.com/zitadel/zitadel-go/v3/pkg/authorization"
	"github.com/zitadel/zitadel-go/v3/pkg/authorization/oauth"
	"github.com/zitadel/zitadel-go/v3/pkg/zitadel"

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
func (s *Server) Run() error {
	// Init Server Option
	s.Options.Init()

	r := newGinEngine(s.Options)

	// listen and server on 0.0.0.0:8080
	return r.Run()
}

func newGinEngine(opts Options) *gin.Engine {
	r := gin.Default()

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.GET("/api", Landing)
	r.GET("/api/ping", Ping)
	r.GET("/api/about", About)

	apiPipeline := r.Group("/api/pipeline")

	switch strings.ToLower(opts.Auth.Mode) {
	case "oauth":
		logrus.Debugf("Using OAuth authentication mode: %s", opts.Auth.Mode)

		switch opts.Auth.Visibility {
		case VisibilityPublic:
			logrus.Debugf("API visibility set to public, no authentication required for read endpoints")

			apiPipeline.Use(func(c *gin.Context) {
				switch c.Request.Method {
				case http.MethodPost, http.MethodPatch, http.MethodDelete:
					auth := checkJWT()
					auth(c)
					// If the auth middleware aborted the request, stop processing.
					if c.IsAborted() {
						return
					}
					return
				default:
					c.Next()
				}
			})
		case VisibilityPrivate:
			logrus.Debugf("API visibility set to private, authentication required for all endpoints")
			apiPipeline.Use(checkJWT())
		}

	case "zitadel":
		logrus.Debugf("Using ZITADEL authentication mode: %s", opts.Auth.Mode)
		ctx := context.Background()

		authZ, err := authorization.New(ctx, zitadel.New(opts.Auth.Zitadel.Domain), oauth.DefaultAuthorization(opts.Auth.Zitadel.KeyFile))
		if err != nil {
			slog.Error("zitadel sdk could not initialize", "error", err)
			os.Exit(1)
		}

		zitadelInterceptor := NewZitadelGin(authZ)

		switch opts.Auth.Visibility {
		case VisibilityPublic:
			logrus.Debugf("API visibility set to public, no authentication required for read endpoints")
			apiPipeline.Use(func(c *gin.Context) {
				switch c.Request.Method {
				case http.MethodPost, http.MethodPatch, http.MethodDelete:
					var auth gin.HandlerFunc

					switch opts.Auth.Zitadel.Role {
					case "":
						logrus.Debugf("Requiring role %q to access the API", opts.Auth.Zitadel.Role)
						auth = zitadelInterceptor.RequireAuthorization(
							authorization.WithRole(opts.Auth.Zitadel.Role))
					default:
						auth = zitadelInterceptor.RequireAuthorization()
					}
					auth(c)
					// If the auth middleware aborted the request, stop processing.
					if c.IsAborted() {
						return
					}
					return
				default:
					c.Next()
				}
			})
		case VisibilityPrivate:
			logrus.Debugf("API visibility set to private, authentication required for all endpoints")
			switch opts.Auth.Zitadel.Role {
			case "":
				logrus.Debugf("Requiring role %q to access the API", opts.Auth.Zitadel.Role)
				apiPipeline.Use(zitadelInterceptor.RequireAuthorization(authorization.WithRole(opts.Auth.Zitadel.Role)))
			default:
				apiPipeline.Use(zitadelInterceptor.RequireAuthorization(authorization.WithRole(opts.Auth.Zitadel.Role)))
			}
		}
	}

	apiPipeline.GET("/scms", ListSCMs)
	apiPipeline.GET("/reports", ListPipelineReports)
	apiPipeline.GET("/reports/:id", GetPipelineReportByID)
	apiPipeline.GET("/config/kinds", SearchConfigKinds)
	apiPipeline.GET("/config/sources", ListConfigSources)
	apiPipeline.GET("/config/conditions", ListConfigConditions)
	apiPipeline.GET("/config/targets", ListConfigTargets)

	// Public endpoints when API visibility is set to public
	if opts.Auth.Mode != "" && opts.Auth.Visibility == VisibilityPublic {
		r.POST("/api/pipeline/reports/search", SearchPipelineReports)
		r.POST("/api/pipeline/config/sources/search", SearchConfigSources)
		r.POST("/api/pipeline/config/conditions/search", SearchConfigConditions)
		r.POST("/api/pipeline/config/targets/search", SearchConfigTargets)
	} else {
		apiPipeline.POST("/reports/search", SearchPipelineReports)
		apiPipeline.POST("/config/sources/search", SearchConfigSources)
		apiPipeline.POST("/config/conditions/search", SearchConfigConditions)
		apiPipeline.POST("/config/targets/search", SearchConfigTargets)
	}

	apiPipeline.POST("/reports", CreatePipelineReport)
	apiPipeline.PUT("/reports/:id", UpdatePipelineReport)
	apiPipeline.DELETE("/reports/:id", DeletePipelineReport)

	return r
}
