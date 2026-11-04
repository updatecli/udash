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
// @Router /api/about [get]
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

// APIPrefix is where the versioned API is served.
const APIPrefix = "/api/v1beta1"

// registerAPIRoutes registers the versioned API endpoints on api. A nil auth leaves
// every endpoint open; otherwise reads are open only when visibility is public.
func registerAPIRoutes(api *gin.RouterGroup, auth gin.HandlerFunc, visibility string) {
	read := []gin.HandlerFunc{}
	ack := []gin.HandlerFunc{}
	write := []gin.HandlerFunc{}

	if auth != nil {
		if visibility != VisibilityPublic {
			read = append(read, auth)
		}
		ack = append(ack, auth, requirePermission(PermissionViewer))
		write = append(write, auth, requireScope(ScopeReportsWrite))

		registerTokenRoutes(api, auth)
	}

	with := func(guard []gin.HandlerFunc, handler gin.HandlerFunc) []gin.HandlerFunc {
		return append(append([]gin.HandlerFunc{}, guard...), handler)
	}

	pipeline := api.Group("/pipeline")

	pipeline.GET("/labels", with(read, ListLabels)...)
	pipeline.GET("/scms", with(read, ListSCMs)...)
	pipeline.GET("/reports", with(read, ListPipelineReports)...)
	pipeline.GET("/reports/:id", with(read, GetPipelineReportByID)...)
	pipeline.GET("/config/kinds", with(read, SearchConfigKinds)...)
	pipeline.GET("/config/sources", with(read, ListConfigSources)...)
	pipeline.GET("/config/conditions", with(read, ListConfigConditions)...)
	pipeline.GET("/config/targets", with(read, ListConfigTargets)...)

	pipeline.POST("/config/sources/search", with(read, SearchConfigSources)...)
	pipeline.POST("/config/conditions/search", with(read, SearchConfigConditions)...)
	pipeline.POST("/config/targets/search", with(read, SearchConfigTargets)...)
	pipeline.POST("/labels/search", with(read, SearchLabels)...)
	pipeline.POST("/reports/search", with(read, SearchPipelineReports)...)
	pipeline.POST("/reports/summary", with(read, SearchPipelineReportsSummary)...)
	pipeline.POST("/actions/search", with(read, SearchOpenActions)...)
	pipeline.POST("/scms/search", with(read, SearchSCMs)...)

	pipeline.PUT("/actions/ack", with(ack, AcknowledgeAction)...)
	pipeline.DELETE("/actions/ack", with(ack, UnacknowledgeAction)...)

	pipeline.POST("/reports", with(write, CreatePipelineReport)...)
	pipeline.PUT("/reports/:id", with(write, UpdatePipelineReport)...)
	pipeline.DELETE("/reports/:id", with(write, DeletePipelineReport)...)
}

func newGinEngine(opts Options) (*gin.Engine, error) {
	r := gin.Default()

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.GET("/api", Landing)
	r.GET("/api/ping", Ping)
	r.GET("/api/about", About)

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
		logrus.Debugf("API visibility set to %q", opts.Auth.Visibility)
	}

	registerAPIRoutes(r.Group(APIPrefix), auth, opts.Auth.Visibility)

	return r, nil
}
