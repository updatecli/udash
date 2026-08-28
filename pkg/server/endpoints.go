package server

import (
	"context"
	"fmt"
	"net/http"

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
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Either an Udash API token, created from the tokens page and prefixed with "udash_pat_", or an access token from the configured identity provider. Send it as "Bearer <token>".
func (s *Server) Run() error {
	// Init Server Option
	if err := s.Options.Init(); err != nil {
		return fmt.Errorf("invalid server options: %w", err)
	}

	r, err := newGinEngine(s.Options)
	if err != nil {
		return err
	}

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

func newGinEngine(opts Options) (*gin.Engine, error) {
	r := gin.Default()

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.GET("/api", Landing)
	r.GET("/api/ping", Ping)
	r.GET("/api/about", About)

	apiPipeline := r.Group("/api/pipeline")

	// auth authenticates a request against the identity provider. It stays nil when
	// no authentication is configured.
	var auth gin.HandlerFunc
	// resolver reports the current permission behind an Udash API token.
	var resolver RoleResolver = snapshotResolver{}

	ctx := context.Background()

	switch opts.Auth.Mode {
	case ModeOIDC:
		logrus.Debugf("Using OpenID Connect authentication mode")

		// Built once: the middleware caches the signing keys of the issuer, so building
		// it per request would refetch them on every call.
		checked, err := checkJWT(opts.Auth)
		if err != nil {
			return nil, fmt.Errorf("jwt middleware could not initialize: %w", err)
		}
		auth = checked

		resolver, err = newRoleResolver(opts.Auth, nil)
		if err != nil {
			return nil, fmt.Errorf("role resolver could not initialize: %w", err)
		}

	case ModeZitadel:
		logrus.Debugf("Using ZITADEL authentication mode")

		authZ, err := authorization.New(ctx, zitadel.New(opts.Auth.Zitadel.Domain), oauth.DefaultAuthorization(opts.Auth.Zitadel.KeyFile))
		if err != nil {
			return nil, fmt.Errorf("zitadel sdk could not initialize: %w", err)
		}

		zitadelInterceptor := NewZitadelGin(authZ, opts.Auth.Roles)
		auth = zitadelInterceptor.RequireAuthorization()

		var roles zitadelUserRoles
		if opts.Auth.Roles.Resolver == ResolverZitadel {
			roles, err = newZitadelUserRoles(ctx, opts.Auth.Zitadel)
			if err != nil {
				return nil, fmt.Errorf("zitadel management client could not initialize: %w", err)
			}
		}

		resolver, err = newRoleResolver(opts.Auth, roles)
		if err != nil {
			return nil, fmt.Errorf("role resolver could not initialize: %w", err)
		}

	case ModeNone, "":
		logrus.Warningf("No authentication configured, every API endpoint is open")

	default:
		// Never fail open: an unrecognized mode used to register no middleware at
		// all, silently leaving every write endpoint unauthenticated.
		return nil, fmt.Errorf("unknown authentication mode %q", opts.Auth.Mode)
	}

	if auth != nil {
		// An Udash API token is checked first and independently of the mode: Udash
		// issues and validates those itself, so they work the same whichever
		// identity provider is configured.
		auth = udashTokenAuth(resolver, auth)

		switch opts.Auth.Visibility {
		case VisibilityPublic:
			logrus.Debugf("API visibility set to public, no authentication required for read endpoints")
			apiPipeline.Use(publicReadOnly(auth))
		case VisibilityPrivate:
			logrus.Debugf("API visibility set to private, authentication required for all endpoints")
			apiPipeline.Use(auth)
		}

		registerTokenRoutes(r, auth)
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

	// Writing a report needs more than a valid token: the caller must be allowed to
	// publish, and a token must have been granted the scope to do it.
	write := []gin.HandlerFunc{}
	if auth != nil {
		write = append(write, requireScope(ScopeReportsWrite))
	}

	apiPipeline.POST("/reports", append(write, CreatePipelineReport)...)
	apiPipeline.PUT("/reports/:id", append(write, UpdatePipelineReport)...)
	apiPipeline.DELETE("/reports/:id", append(write, DeletePipelineReport)...)

	return r, nil
}
