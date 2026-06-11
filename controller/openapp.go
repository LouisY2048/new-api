package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetAllOpenApps handles GET /api/openapp/
func GetAllOpenApps(c *gin.Context) {
	startIdx, _ := strconv.Atoi(c.Query("p"))
	num, _ := strconv.Atoi(c.Query("page_size"))
	if num <= 0 {
		num = 10
	}
	startIdx = startIdx * num

	apps, total, err := model.GetAllOpenApps(startIdx, num)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	items := make([]dto.OpenAppItem, 0, len(apps))
	for _, app := range apps {
		items = append(items, dto.OpenAppItem{
			Id:                 app.Id,
			AppId:              app.AppId,
			Name:               app.Name,
			Status:             app.Status,
			IpWhitelistEnabled: app.IpWhitelistEnabled,
			AllowIps:           app.AllowIps,
			CreatedAt:          app.CreatedAt,
			UpdatedAt:          app.UpdatedAt,
		})
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    items,
		"total":   total,
	})
}

// GetOpenApp handles GET /api/openapp/:id
func GetOpenApp(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	app, err := model.GetOpenAppById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, app)
}

// AddOpenApp handles POST /api/openapp/
func AddOpenApp(c *gin.Context) {
	var app model.OpenApp
	if err := common.UnmarshalBodyReusable(c, &app); err != nil {
		common.ApiError(c, err)
		return
	}
	if app.AppId == "" {
		app.AppId = "app_" + common.GetRandomString(16)
	}
	if err := model.InsertOpenApp(&app); err != nil {
		common.ApiError(c, err)
		return
	}
	// Explicitly include AppSecret in response (only shown once on creation)
	common.ApiSuccess(c, gin.H{
		"id":                   app.Id,
		"app_id":               app.AppId,
		"app_secret":           app.AppSecret,
		"name":                 app.Name,
		"status":               app.Status,
		"ip_whitelist_enabled": app.IpWhitelistEnabled,
		"allow_ips":            app.AllowIps,
		"created_at":           app.CreatedAt,
		"updated_at":           app.UpdatedAt,
	})
}

// UpdateOpenApp handles PUT /api/openapp/
func UpdateOpenApp(c *gin.Context) {
	var req dto.OpenAppRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	// Read existing app for AppId (needed for cache invalidation)
	existing, err := model.GetOpenAppById(req.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	app := &model.OpenApp{
		Id:                 req.Id,
		AppId:              existing.AppId,
		Name:               req.Name,
		Status:             req.Status,
		IpWhitelistEnabled: req.IpWhitelistEnabled,
		AllowIps:           req.AllowIps,
	}
	if err := model.UpdateOpenApp(app); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, app)
}

// DeleteOpenApp handles DELETE /api/openapp/:id
func DeleteOpenApp(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	appId, err := model.DeleteOpenAppById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"app_id": appId})
}

// GetOpenAppKey handles POST /api/openapp/:id/key
// View or refresh AppSecret (requires SecureVerification via middleware)
func GetOpenAppKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	action := c.Query("action")
	if action == "refresh" {
		secret, err := model.RefreshOpenAppSecret(id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, gin.H{"app_secret": secret})
		return
	}
	app, err := model.GetOpenAppById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"app_secret": app.AppSecret})
}
