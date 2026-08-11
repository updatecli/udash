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

// publicReadOnly returns a middleware leaving the read endpoints open while requiring the
// provided authentication for anything which may change the stored data.
//
// The read methods are the ones listed, and every other one requires authentication. It is
// deliberately written that way around: enumerating the write methods instead left PUT
// unauthenticated, and would leave out any method added later.
func publicReadOnly(auth gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
		default:
			auth(c)
		}
	}
}

// zitadelAuthorization requires a valid token, and the configured role when there is one.
//
// An empty role must not be passed to authorization.WithRole: it checks the token against
// a role which is granted to nobody, so it rejects every request instead of accepting any
// authenticated one.
func zitadelAuthorization[T authorization.Ctx](interceptor *Interceptor[T], role string) gin.HandlerFunc {
	if role == "" {
		logrus.Debugf("No role required to access the API")
		return interceptor.RequireAuthorization()
	}

	logrus.Debugf("Requiring role %q to access the API", role)
	return interceptor.RequireAuthorization(authorization.WithRole(role))
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

		// Built once: the middleware caches the signing keys of the issuer, so building
		// it per request would refetch them on every call.
		auth, err := checkJWT()
		if err != nil {
			slog.Error("jwt middleware could not initialize", "error", err)
			os.Exit(1)
		}

		switch opts.Auth.Visibility {
		case VisibilityPublic:
			logrus.Debugf("API visibility set to public, no authentication required for read endpoints")
			apiPipeline.Use(publicReadOnly(auth))
		case VisibilityPrivate:
			logrus.Debugf("API visibility set to private, authentication required for all endpoints")
			apiPipeline.Use(auth)
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
		auth := zitadelAuthorization(zitadelInterceptor, opts.Auth.Zitadel.Role)

		switch opts.Auth.Visibility {
		case VisibilityPublic:
			logrus.Debugf("API visibility set to public, no authentication required for read endpoints")
			apiPipeline.Use(publicReadOnly(auth))
		case VisibilityPrivate:
			logrus.Debugf("API visibility set to private, authentication required for all endpoints")
			apiPipeline.Use(auth)
		}
	}

	apiPipeline.GET("/labels", ListLabels)
	apiPipeline.GET("/scms", ListSCMs)
	apiPipeline.GET("/reports", ListPipelineReports)
	apiPipeline.GET("/reports/:id", GetPipelineReportByID)
	apiPipeline.GET("/config/kinds", SearchConfigKinds)
	apiPipeline.GET("/config/sources", ListConfigSources)
	apiPipeline.GET("/config/conditions", ListConfigConditions)
	apiPipeline.GET("/config/targets", ListConfigTargets)

	// Public endpoints when API visibility is set to public
	if opts.Auth.Mode != "" && opts.Auth.Visibility == VisibilityPublic {
		r.POST("/api/pipeline/config/sources/search", SearchConfigSources)
		r.POST("/api/pipeline/config/conditions/search", SearchConfigConditions)
		r.POST("/api/pipeline/config/targets/search", SearchConfigTargets)
		r.POST("/api/pipeline/labels/search", SearchLabels)
		r.POST("/api/pipeline/reports/search", SearchPipelineReports)
		r.POST("/api/pipeline/reports/summary", SearchPipelineReportsSummary)
		r.POST("/api/pipeline/scms/search", SearchSCMs)
	} else {
		apiPipeline.POST("/config/sources/search", SearchConfigSources)
		apiPipeline.POST("/config/conditions/search", SearchConfigConditions)
		apiPipeline.POST("/config/targets/search", SearchConfigTargets)
		apiPipeline.POST("/labels/search", SearchLabels)
		apiPipeline.POST("/reports/search", SearchPipelineReports)
		apiPipeline.POST("/reports/summary", SearchPipelineReportsSummary)
		apiPipeline.POST("/scms/search", SearchSCMs)
	}

	apiPipeline.POST("/reports", CreatePipelineReport)
	apiPipeline.PUT("/reports/:id", UpdatePipelineReport)
	apiPipeline.DELETE("/reports/:id", DeletePipelineReport)

	return r
}
