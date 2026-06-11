package common

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// OpenApiSuccess 成功响应，code="00000"，data 携带业务数据
func OpenApiSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code":    "00000",
		"message": "success",
		"data":    data,
	})
}

// OpenApiError 失败响应，code 为非 00000，data 固定为 nil
func OpenApiError(c *gin.Context, code string, message string) {
	c.JSON(http.StatusOK, gin.H{
		"code":    code,
		"message": message,
		"data":    nil,
	})
}
