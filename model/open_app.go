package model

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type OpenApp struct {
	Id                 int    `json:"id" gorm:"primaryKey;autoIncrement"`
	AppId              string `json:"app_id" gorm:"type:varchar(64);uniqueIndex;not null"`
	AppSecret          string `json:"-" gorm:"type:varchar(128);not null"`
	Name               string `json:"name" gorm:"type:varchar(128);default:''"`
	Status             int    `json:"status" gorm:"type:int;default:1"`
	IpWhitelistEnabled bool   `json:"ip_whitelist_enabled" gorm:"default:false"`
	AllowIps           string `json:"allow_ips" gorm:"type:text;default:''"`
	CreatedAt          int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

const (
	openAppCacheKey        = "openapp:"
	openAppCacheExpiration = 300 * time.Second
)

// GenerateAppSecret generates a 32-char random hex string
func GenerateAppSecret() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// cacheSetOpenApp stores OpenApp in Redis
func cacheSetOpenApp(app *OpenApp) {
	if !common.RedisEnabled {
		return
	}
	key := openAppCacheKey + app.AppId
	data, err := common.Marshal(app)
	if err != nil {
		return
	}
	common.RedisSet(key, string(data), openAppCacheExpiration)
}

// cacheGetOpenApp retrieves OpenApp from Redis cache
func cacheGetOpenApp(appId string) (*OpenApp, error) {
	if !common.RedisEnabled {
		return nil, nil
	}
	key := openAppCacheKey + appId
	data, err := common.RedisGet(key)
	if err != nil || data == "" {
		return nil, err
	}
	var app OpenApp
	if err := common.Unmarshal([]byte(data), &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// cacheDeleteOpenApp removes OpenApp from Redis cache
func cacheDeleteOpenApp(appId string) {
	if !common.RedisEnabled {
		return
	}
	common.RedisDel(openAppCacheKey + appId)
}

// GetOpenAppByAppId looks up OpenApp by AppId (cache then DB)
func GetOpenAppByAppId(appId string) (*OpenApp, error) {
	if cached, err := cacheGetOpenApp(appId); err == nil && cached != nil {
		return cached, nil
	}
	var app OpenApp
	err := DB.Where("app_id = ?", appId).First(&app).Error
	if err != nil {
		return nil, err
	}
	cacheSetOpenApp(&app)
	return &app, nil
}

// GetAllOpenApps returns paginated list of all OpenApps
func GetAllOpenApps(startIdx int, num int) (apps []*OpenApp, total int64, err error) {
	err = DB.Model(&OpenApp{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = DB.Order("id desc").Limit(num).Offset(startIdx).Find(&apps).Error
	return apps, total, err
}

// InsertOpenApp creates a new OpenApp, auto-generating AppSecret if empty
func InsertOpenApp(app *OpenApp) error {
	if app.AppSecret == "" {
		secret, err := GenerateAppSecret()
		if err != nil {
			return err
		}
		app.AppSecret = secret
	}
	err := DB.Create(app).Error
	if err != nil {
		return err
	}
	cacheSetOpenApp(app)
	return nil
}

// UpdateOpenApp updates OpenApp fields (excludes AppId and AppSecret)
func UpdateOpenApp(app *OpenApp) error {
	err := DB.Model(&OpenApp{}).Where("id = ?", app.Id).Updates(map[string]interface{}{
		"name":                 app.Name,
		"status":               app.Status,
		"ip_whitelist_enabled": app.IpWhitelistEnabled,
		"allow_ips":            app.AllowIps,
	}).Error
	if err != nil {
		return err
	}
	cacheDeleteOpenApp(app.AppId)
	return nil
}

// DeleteOpenAppById deletes an OpenApp by ID, returns the app_id for cache cleanup
func DeleteOpenAppById(id int) (string, error) {
	var app OpenApp
	err := DB.Where("id = ?", id).First(&app).Error
	if err != nil {
		return "", err
	}
	err = DB.Delete(&app).Error
	if err != nil {
		return "", err
	}
	cacheDeleteOpenApp(app.AppId)
	return app.AppId, nil
}

// GetOpenAppById looks up OpenApp by primary key
func GetOpenAppById(id int) (*OpenApp, error) {
	var app OpenApp
	err := DB.Where("id = ?", id).First(&app).Error
	return &app, err
}

// RefreshOpenAppSecret generates a new AppSecret for the given OpenApp
func RefreshOpenAppSecret(id int) (string, error) {
	secret, err := GenerateAppSecret()
	if err != nil {
		return "", err
	}
	err = DB.Model(&OpenApp{}).Where("id = ?", id).Update("app_secret", secret).Error
	if err != nil {
		return "", err
	}
	app, err := GetOpenAppById(id)
	if err != nil {
		return "", err
	}
	cacheSetOpenApp(app)
	return secret, nil
}
