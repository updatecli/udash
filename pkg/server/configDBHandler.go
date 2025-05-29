package server

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/model"
)

// SourceConfigResponse represents a response containing configuration sources.
type SourceConfigResponse struct {
	// Configs is a list of configuration sources.
	Configs []model.ConfigSource `json:"configs"`
}

// ConditionConfigResponse represents a response containing configuration conditions.
type ConditionConfigResponse struct {
	// Configs is a list of configuration conditions.
	Configs []model.ConfigCondition `json:"configs"`
}

// TargetConfigResponse represents a response containing configuration targets.
type TargetConfigResponse struct {
	// Configs is a list of configuration targets.
	Configs []model.ConfigTarget `json:"configs"`
}

// ListConfigSources returns a resource configuration from the database.
// @Summary List all configuration sources
// @Description List all configuration sources from the database
// @Tags Configuration Sources
// @Param id query string false "ID of the configuration source"
// @Param kind query string false "Kind of the configuration source"
// @Param config query string false "Configuration of the source"
// @Success 200 {object} SourceConfigResponse
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/config/sources [get]
func ListConfigSources(c *gin.Context) {
	id := c.Request.URL.Query().Get("id")
	kind := c.Request.URL.Query().Get("kind")
	config := c.Request.URL.Query().Get("config")

	rows, err := dbGetConfigSource(kind, id, config)
	if err != nil {
		logrus.Errorf("searching for config source: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, SourceConfigResponse{
		Configs: rows,
	})
}

// SearchConfigSources returns a resource configuration from the database.
// @Summary Search configuration sources
// @Description Search for configuration sources in the database
// @Tags Configuration Sources
// @Accept json
// @Produce json
// @Success 200 {object} SourceConfigResponse
// @Failure 400 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/config/sources/search [post]
func SearchConfigSources(c *gin.Context) {
	type configResource struct {
		ID     string          `json:"id"`
		Kind   string          `json:"kind"`
		Config json.RawMessage `json:"config"`
	}

	queryConfig := configResource{}

	if err := c.ShouldBindJSON(&queryConfig); err != nil {
		logrus.Errorf("failed to read json body: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	rows, err := dbGetConfigSource(queryConfig.Kind, queryConfig.ID, string(queryConfig.Config))
	if err != nil {
		logrus.Errorf("searching for config source: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, SourceConfigResponse{
		Configs: rows,
	})
}

// DeleteConfigSource deletes a resource configuration from the database.
// @Summary Delete a configuration source
// @Description Delete a configuration source from the database
// @Tags Configuration Sources
// @Param id query string true "ID of the configuration source to delete"
// @Success 200 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/config/sources [delete]
func DeleteConfigSource(c *gin.Context) {
	id := c.Request.URL.Query().Get("id")

	err := dbDeleteConfigResource("source", id)
	if err != nil {
		logrus.Errorf("deleting config source: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, DefaultResponseModel{
		Message: "success",
	})
}

// ListConfigConditions returns condition configurations from the database.
// @Summary List all configuration conditions
// @Description List all configuration conditions from the database
// @Tags Configuration Conditions
// @Param id query string false "ID of the configuration condition"
// @Param kind query string false "Kind of the configuration condition"
// @Param config query string false "Configuration of the condition"
// @Success 200 {object} ConditionConfigResponse
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/config/conditions [get]
func ListConfigConditions(c *gin.Context) {
	id := c.Request.URL.Query().Get("id")
	kind := c.Request.URL.Query().Get("kind")
	config := c.Request.URL.Query().Get("config")

	rows, err := dbGetConfigCondition(kind, id, config)
	if err != nil {
		logrus.Errorf("searching for config condition: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Message: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, ConditionConfigResponse{
		Configs: rows,
	})
}

// SearchConfigConditions returns condition configurations from the database.
// @Summary Search configuration conditions
// @Description Search for configuration conditions in the database
// @Tags Configuration Conditions
// @Accept json
// @Produce json
// @Success 200 {object} ConditionConfigResponse
// @Failure 400 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/config/conditions/search [post]
func SearchConfigConditions(c *gin.Context) {
	type configResource struct {
		ID     string          `json:"id"`
		Kind   string          `json:"kind"`
		Config json.RawMessage `json:"config"`
	}

	queryConfig := configResource{}

	if err := c.ShouldBindJSON(&queryConfig); err != nil {
		logrus.Errorf("failed to read json body: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	rows, err := dbGetConfigCondition(queryConfig.Kind, queryConfig.ID, string(queryConfig.Config))
	if err != nil {
		logrus.Errorf("searching for config condition: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Message: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, ConditionConfigResponse{
		Configs: rows,
	})
}

// DeleteConfigCondition deletes a resource configuration from the database.
// @Summary Delete a configuration condition
// @Description Delete a configuration condition from the database
// @Tags Configuration Conditions
// @Param id query string true "ID of the configuration condition to delete"
// @Success 200 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/config/conditions [delete]
func DeleteConfigCondition(c *gin.Context) {
	id := c.Request.URL.Query().Get("id")

	err := dbDeleteConfigResource("condition", id)
	if err != nil {
		logrus.Errorf("deleting config condition: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Message: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, DefaultResponseModel{
		Message: "success",
	})
}

// ListConfigTargets returns target configurations from the database.
// @Summary List all target configurations.
// @Description List all configuration targets from the database
// @Tags Configuration Targets
// @Param id query string false "ID of the configuration target"
// @Param kind query string false "Kind of the configuration target"
// @Param config query string false "Configuration of the target"
// @Success 200 {object} TargetConfigResponse
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/config/targets [get]
func ListConfigTargets(c *gin.Context) {
	id := c.Request.URL.Query().Get("id")
	kind := c.Request.URL.Query().Get("kind")
	config := c.Request.URL.Query().Get("config")

	rows, err := dbGetConfigTarget(kind, id, config)
	if err != nil {
		logrus.Errorf("searching for config target: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, TargetConfigResponse{
		Configs: rows,
	})
}

// SearchConfigTargets returns target configurations from the database.
// @Summary Search configuration targets
// @Description Search for configuration targets in the database
// @Tags Configuration Targets
// @Accept json
// @Produce json
// @Success 200 {object} TargetConfigResponse
// @Failure 400 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/config/targets/search [post]
func SearchConfigTargets(c *gin.Context) {
	type configResource struct {
		ID     string          `json:"id"`
		Kind   string          `json:"kind"`
		Config json.RawMessage `json:"config"`
	}

	queryConfig := configResource{}

	if err := c.ShouldBindJSON(&queryConfig); err != nil {
		logrus.Errorf("failed to read json body: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	rows, err := dbGetConfigTarget(queryConfig.Kind, queryConfig.ID, string(queryConfig.Config))
	if err != nil {
		logrus.Errorf("searching for config target: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, TargetConfigResponse{
		Configs: rows,
	})
}

// DeleteConfigTarget deletes a resource configuration from the database.
// @Summary Delete a configuration target
// @Description Delete a configuration target from the database
// @Tags Configuration Targets
// @Param id query string true "ID of the configuration target to delete"
// @Success 200 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/config/targets [delete]
func DeleteConfigTarget(c *gin.Context) {
	id := c.Request.URL.Query().Get("id")

	err := dbDeleteConfigResource("target", id)
	if err != nil {
		logrus.Errorf("deleting config target: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, DefaultResponseModel{
		Message: "success",
	})
}
