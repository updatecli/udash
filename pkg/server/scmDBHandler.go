package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func FindTargetSCM(c *gin.Context) {

	rows, err := dbGetScm("", "", "")
	if err != nil {
		logrus.Errorf("find target scms: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err,
		})

		return
	}

	c.JSON(http.StatusOK, gin.H{
		"scms": rows,
	})
}
