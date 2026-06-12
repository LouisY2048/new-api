package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// IssueApiKey handles POST /open/v1/apikey/issue
func IssueApiKey(c *gin.Context) {
	var req dto.IssueApiKeyRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.OpenApiError(c, "99999", "平台系统内部异常")
		return
	}

	// Email validation: empty or too long
	if len(req.Email) == 0 || len(req.Email) > 50 {
		common.OpenApiError(c, "10002", "邮箱格式不符合规范")
		return
	}

	// ICCID validation: must be exactly 20 digits
	if len(req.Iccid) != 20 {
		common.OpenApiError(c, "10001", "ICCID 不存在、未入库或未开户")
		return
	}

	appId := common.GetContextKeyString(c, constant.ContextKeyOpenAppId)

	// Rate limit checks (must run in controller since they need request params)
	if !middleware.CheckIccidRateLimit(c, req.Iccid) {
		return
	}
	if !middleware.CheckEmailRateLimit(c, req.Email, req.Iccid) {
		return
	}
	if !middleware.CheckConsecutiveIccidRateLimit(c, appId, req.Iccid) {
		return
	}

	data, code, err := service.IssueApiKey(c, &req, appId)
	if err != nil {
		common.OpenApiError(c, "99999", "平台系统内部异常")
		return
	}

	// code "10005" with non-nil data means existing valid APIKey with matching email
	// code "10005" with nil data means email mismatch
	if code == "10005" {
		if data != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    "10005",
				"message": "该 ICCID 已申领过 APIKey，禁止重复签发",
				"data":    data,
			})
		} else {
			common.OpenApiError(c, "10005", "该 ICCID 已申领过 APIKey，禁止重复签发")
		}
		return
	}

	if code != "00000" {
		messages := map[string]string{
			"10001": "ICCID 不存在、未入库或未开户",
			"10004": "APIKey 无效、已过期或已作废",
		}
		msg, ok := messages[code]
		if !ok {
			code = "99999"
			msg = "平台系统内部异常"
		}
		common.OpenApiError(c, code, msg)
		return
	}

	common.OpenApiSuccess(c, data)
}

// OpenGetTokenUsage handles POST /open/v1/token/usage
func OpenGetTokenUsage(c *gin.Context) {
	var req dto.TokenUsageRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.OpenApiError(c, "99999", "平台系统内部异常")
		return
	}

	// ICCID validation: must not be empty
	if len(req.Iccid) == 0 {
		common.OpenApiError(c, "10001", "ICCID 不存在、未入库或未开户")
		return
	}

	data, code, err := service.GetTokenUsage(req.Iccid)
	if err != nil || code != "00000" {
		messages := map[string]string{
			"10001": "ICCID 不存在、未入库或未开户",
			"10004": "APIKey 无效、已过期或已作废",
		}
		msg, ok := messages[code]
		if !ok {
			code = "99999"
			msg = "平台系统内部异常"
		}
		common.OpenApiError(c, code, msg)
		return
	}

	common.OpenApiSuccess(c, data)
}
