package server

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/model"
)

// specOnlyConfig represents a Config object with only the Spec field
type specOnlyConfig struct {
	Spec interface{} `json:"spec"`
}

// transformToSpecOnly transforms a Config object to only include the Spec field
func transformToSpecOnly(configJSON []byte) (json.RawMessage, error) {
	var configMap map[string]interface{}
	if err := json.Unmarshal(configJSON, &configMap); err != nil {
		return nil, err
	}

	// Extract only the Spec field
	specOnly := specOnlyConfig{
		Spec: configMap["spec"],
	}

	result, err := json.Marshal(specOnly)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(result), nil
}

// transformSourceConfigsToSpecOnly transforms a slice of ConfigSource to spec-only format
func transformSourceConfigsToSpecOnly(configs []model.ConfigSource) ([]model.ConfigSource, error) {
	result := make([]model.ConfigSource, len(configs))
	for i, config := range configs {
		result[i] = config
		configJSON, err := json.Marshal(config.Config)
		if err != nil {
			return nil, err
		}
		specOnlyJSON, err := transformToSpecOnly(configJSON)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(specOnlyJSON, &result[i].Config); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// transformConditionConfigsToSpecOnly transforms a slice of ConfigCondition to spec-only format
func transformConditionConfigsToSpecOnly(configs []model.ConfigCondition) ([]model.ConfigCondition, error) {
	result := make([]model.ConfigCondition, len(configs))
	for i, config := range configs {
		result[i] = config
		configJSON, err := json.Marshal(config.Config)
		if err != nil {
			return nil, err
		}
		specOnlyJSON, err := transformToSpecOnly(configJSON)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(specOnlyJSON, &result[i].Config); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// transformTargetConfigsToSpecOnly transforms a slice of ConfigTarget to spec-only format
func transformTargetConfigsToSpecOnly(configs []model.ConfigTarget) ([]model.ConfigTarget, error) {
	result := make([]model.ConfigTarget, len(configs))
	for i, config := range configs {
		result[i] = config
		configJSON, err := json.Marshal(config.Config)
		if err != nil {
			return nil, err
		}
		specOnlyJSON, err := transformToSpecOnly(configJSON)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(specOnlyJSON, &result[i].Config); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// SourceConfigResponse represents a response containing configuration sources.
type SourceConfigResponse struct {
	// Configs is a list of configuration sources.
	Configs []model.ConfigSource `json:"configs"`
	// TotalCount is the total number of sources for pagination.
	TotalCount int `json:"total_count"`
}

// ConditionConfigResponse represents a response containing configuration conditions.
type ConditionConfigResponse struct {
	// Configs is a list of configuration conditions.
	Configs []model.ConfigCondition `json:"configs"`
	// TotalCount is the total number of conditions for pagination.
	TotalCount int `json:"total_count"`
}

// TargetConfigResponse represents a response containing configuration targets.
type TargetConfigResponse struct {
	// Configs is a list of configuration targets.
	Configs []model.ConfigTarget `json:"configs"`
	// TotalCount is the total number of targets for pagination.
	TotalCount int `json:"total_count"`
}

// ConfigKindResponse represents a response containing configuration kinds.
type ConfigKindResponse struct {
	Data []string `json:"data"`
}

// ListConfigSources returns a resource configuration from the database.
// @Summary List all configuration sources
// @Description List all configuration sources from the database
// @Tags Configuration Sources
// @Param id query string false "ID of the configuration source"
// @Param kind query string false "Kind of the configuration source"
// @Param config query string false "Configuration of the source"
// @Param limit query string false "Limit the number of reports returned, default is 100"
// @Param page query string false "Page number for pagination, default is 1"
// @Param spec_only query boolean false "When true, returns only the Spec field from each Config object"
// @Success 200 {object} SourceConfigResponse
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/config/sources [get]
func ListConfigSources(c *gin.Context) {
	id := c.Request.URL.Query().Get("id")
	kind := c.Request.URL.Query().Get("kind")
	config := c.Request.URL.Query().Get("config")
	specOnly := c.Request.URL.Query().Get("spec_only") == "true"

	limit, page, err := getPaginationParamFromURLQuery(c)

	if err != nil {
		logrus.Errorf("invalid pagination parameters: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: "invalid pagination parameters: " + err.Error(),
		})
		return
	}

	rows, totalCount, err := database.GetSourceConfigs(c, kind, id, config, limit, page)
	if err != nil {
		logrus.Errorf("searching for config source: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	if specOnly {
		rows, err = transformSourceConfigsToSpecOnly(rows)
		if err != nil {
			logrus.Errorf("transforming config sources to spec-only: %s", err)
			c.JSON(http.StatusInternalServerError, DefaultResponseModel{
				Err: "failed to transform configs: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, SourceConfigResponse{
		Configs:    rows,
		TotalCount: totalCount,
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
		Limit  int             `json:"limit"`
		Page   int             `json:"page"`
	}

	queryConfig := configResource{}

	if err := c.ShouldBindJSON(&queryConfig); err != nil {
		logrus.Errorf("failed to read json body: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	rows, totalCount, err := database.GetSourceConfigs(c, queryConfig.Kind, queryConfig.ID, string(queryConfig.Config), queryConfig.Limit, queryConfig.Page)
	if err != nil {
		logrus.Errorf("searching for config source: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, SourceConfigResponse{
		Configs:    rows,
		TotalCount: totalCount,
	})
}

// SearchConfigKinds returns a resource configuration from the database.
// @Summary Search configuration by kind
// @Description Search for configuration by kind in the database
// @Tags Configuration
// @Accept json
// @Produce json
// @Success 200 {object} ConfigKindResponse
// @Failure 400 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/config/kinds [post]
func SearchConfigKinds(c *gin.Context) {
	resourceType := c.Request.URL.Query().Get("type")
	if resourceType == "" {
		c.JSON(
			http.StatusBadRequest,
			DefaultResponseModel{
				Err: "no type provided",
			},
		)
		return
	}

	kinds, err := database.GetConfigKind(c, resourceType)
	if err != nil {
		logrus.Errorf("searching for config source kind: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ConfigKindResponse{
		Data: kinds,
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

	err := database.DeleteConfigResource(c, "source", id)
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
// @Param limit query string false "Limit the number of reports returned, default is 100"
// @Param page query string false "Page number for pagination, default is 1"
// @Param spec_only query boolean false "When true, returns only the Spec field from each Config object"
// @Success 200 {object} ConditionConfigResponse
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/config/conditions [get]
func ListConfigConditions(c *gin.Context) {
	id := c.Request.URL.Query().Get("id")
	kind := c.Request.URL.Query().Get("kind")
	config := c.Request.URL.Query().Get("config")
	specOnly := c.Request.URL.Query().Get("spec_only") == "true"

	limit, page, err := getPaginationParamFromURLQuery(c)
	if err != nil {
		logrus.Errorf("invalid pagination parameters: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: "invalid pagination parameters: " + err.Error(),
		})
		return
	}

	rows, totalCount, err := database.GetConditionConfigs(c, kind, id, config, limit, page)
	if err != nil {
		logrus.Errorf("searching for config condition: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Message: err.Error(),
		})
		return
	}

	if specOnly {
		rows, err = transformConditionConfigsToSpecOnly(rows)
		if err != nil {
			logrus.Errorf("transforming config conditions to spec-only: %s", err)
			c.JSON(http.StatusInternalServerError, DefaultResponseModel{
				Err: "failed to transform configs: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, ConditionConfigResponse{
		Configs:    rows,
		TotalCount: totalCount,
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
		// ID is the unique identifier of the configuration condition.
		ID string `json:"id"`
		// Kind is the kind of the configuration condition.
		Kind string `json:"kind"`
		// Config is the configuration of the condition.
		Config json.RawMessage `json:"config"`
		// Limit is the maximum number of results to return.
		Limit int `json:"limit"`
		// Page is the page number for pagination.
		Page int `json:"page"`
	}

	queryConfig := configResource{}

	if err := c.ShouldBindJSON(&queryConfig); err != nil {
		logrus.Errorf("failed to read json body: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	configs, totalCount, err := database.GetConditionConfigs(c, queryConfig.Kind, queryConfig.ID, string(queryConfig.Config), queryConfig.Limit, queryConfig.Page)
	if err != nil {
		logrus.Errorf("searching for config condition: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Message: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, ConditionConfigResponse{
		Configs:    configs,
		TotalCount: totalCount,
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

	err := database.DeleteConfigResource(c, "condition", id)
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
// @Param limit query string false "Limit the number of reports returned, default is 100"
// @Param page query string false "Page number for pagination, default is 1"
// @Param spec_only query boolean false "When true, returns only the Spec field from each Config object"
// @Success 200 {object} TargetConfigResponse
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/config/targets [get]
func ListConfigTargets(c *gin.Context) {
	id := c.Request.URL.Query().Get("id")
	kind := c.Request.URL.Query().Get("kind")
	config := c.Request.URL.Query().Get("config")
	specOnly := c.Request.URL.Query().Get("spec_only") == "true"

	limit, page, err := getPaginationParamFromURLQuery(c)
	if err != nil {
		logrus.Errorf("invalid pagination parameters: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: "invalid pagination parameters: " + err.Error(),
		})
		return
	}

	rows, totalCount, err := database.GetTargetConfigs(c, kind, id, config, limit, page)
	if err != nil {
		logrus.Errorf("searching for config target: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	if specOnly {
		rows, err = transformTargetConfigsToSpecOnly(rows)
		if err != nil {
			logrus.Errorf("transforming config targets to spec-only: %s", err)
			c.JSON(http.StatusInternalServerError, DefaultResponseModel{
				Err: "failed to transform configs: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, TargetConfigResponse{
		Configs:    rows,
		TotalCount: totalCount,
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
		Limit  int             `json:"limit"`
		Page   int             `json:"page"`
	}

	queryConfig := configResource{}

	if err := c.ShouldBindJSON(&queryConfig); err != nil {
		logrus.Errorf("failed to read json body: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	configs, totalCount, err := database.GetTargetConfigs(c, queryConfig.Kind, queryConfig.ID, string(queryConfig.Config), queryConfig.Limit, queryConfig.Page)
	if err != nil {
		logrus.Errorf("searching for config target: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, TargetConfigResponse{
		Configs:    configs,
		TotalCount: totalCount,
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

	err := database.DeleteConfigResource(c, "target", id)
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
