package utils

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

func GetIDParam(c *gin.Context, paramName string) (int64, error) {
	idStr := c.Param(paramName)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return 0, err
	}
	return id, nil
}