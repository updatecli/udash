package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func FindSCM(c *gin.Context) {

	scmid := c.Request.URL.Query().Get("scmid")
	url := c.Request.URL.Query().Get("url")
	branch := c.Request.URL.Query().Get("branch")

	rows, err := dbGetScm(scmid, url, branch)
	if err != nil {
		logrus.Errorf("searching for scms: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err,
		})

		return
	}

	c.JSON(http.StatusOK, gin.H{
		"scms": rows,
	})
}
