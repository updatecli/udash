package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/model"
)

// ListSCMsResponse represents the response for the ListSCMs endpoint.
type ListSCMsResponse struct {
	// SCMs is a list of SCMs.
	SCMs []model.SCM `json:"scms"`
	// TotalCount is the total number of SCMs matching the query.
	TotalCount int `json:"total_count"`
}

// ListSCMs returns a list of SCMs from the database.
// @Summary List SCMs
// @Description List SCMs data from the database
// @Tags SCMs
// @Param scmid query string false "ID of the SCM"
// @Param url query string false "URL of the SCM"
// @Param branch query string false "Branch of the SCM"
// @Param summary query bool false "Return a summary of the SCMs"
// @Param limit query string false "Limit the number of reports returned, default is 100"
// @Param page query string false "Page number for pagination, default is 1"
// @Success 200 {object} DefaultResponseModel
// @Failure 500 {object} DefaultResponseModel
// @Router /api/pipeline/scms [get]
func ListSCMs(c *gin.Context) {
	scmid := c.Request.URL.Query().Get("scmid")
	url := c.Request.URL.Query().Get("url")
	branch := c.Request.URL.Query().Get("branch")
	summary := c.Request.URL.Query().Get("summary")

	limit, page, err := getPaginationParamFromURLQuery(c)
	if err != nil {
		logrus.Errorf("getting pagination params: %s", err)
		c.JSON(http.StatusBadRequest, DefaultResponseModel{
			Err: "invalid pagination parameters",
		})
		return
	}

	rows, totalCount, err := database.GetSCM(c, scmid, url, branch, limit, page)
	if err != nil {
		logrus.Errorf("searching for scms: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})

		return
	}

	if strings.ToUpper(summary) == "TRUE" {
		findSCMSummary(c, rows, totalCount)
		return
	}

	c.JSON(http.StatusOK, ListSCMsResponse{
		SCMs:       rows,
		TotalCount: totalCount,
	})
}

// FindSCMSummaryResponse represents the response for the FindSCMSummary endpoint.
type FindSCMSummaryResponse struct {
	TotalCount int                                  `json:"total_count"`
	Data       map[string]database.SCMBranchDataset `json:"data"`
}

// findSCMSummary returns a summary of all git repositories detected.
func findSCMSummary(c *gin.Context, scmRows []model.SCM, totalCount int) {

	var data map[string]database.SCMBranchDataset

	dataset, err := database.GetSCMSummary(c, scmRows, totalCount, monitoringDurationDays) // Assuming 30 days as the default monitoring duration
	if err != nil {
		logrus.Errorf("getting scm summary failed: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{
			Err: err.Error(),
		})
		return
	}

	if dataset != nil {
		data = dataset.Data
	}

	c.JSON(http.StatusOK, FindSCMSummaryResponse{
		Data:       data,
		TotalCount: totalCount,
	})
}
