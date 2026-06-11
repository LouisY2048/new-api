# Open API — Token/APIKey 开放接口 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现两个 Open API 端点（签发 APIKey + 查询使用量）和后台 OpenApp 管理功能。

**Architecture:** 遵循现有 Router → Controller → Service → Model 分层。新增 OpenAuth 签名鉴权中间件、三个 Redis 限流规则、OpenApp 数据模型及缓存、独立返回格式辅助函数。Classic 主题设置页新增"开放应用"标签。

**Tech Stack:** Go 1.22+, Gin, GORM, Redis, React 18 + Semi UI

---

## 文件清单

| 操作 | 文件 | 职责 |
|------|------|------|
| Create | `dto/openapi.go` | 请求/响应结构体 |
| Create | `common/openapi_response.go` | 统一返回格式辅助函数 |
| Create | `model/open_app.go` | OpenApp 模型、缓存、CRUD |
| Modify | `model/user.go` | 新增 ICCID 字段 |
| Modify | `model/main.go` | AutoMigrate 注册 OpenApp |
| Modify | `constant/context_key.go` | 新增 OpenApp 上下文 Key |
| Create | `middleware/open_auth.go` | OpenAuth 签名鉴权 + 三个限流 |
| Create | `service/openapi.go` | 签发 + 查询业务逻辑 |
| Create | `controller/openapi.go` | /open/v1/* 两个接口处理器 |
| Create | `controller/openapp.go` | 后台管理 CRUD |
| Create | `router/openapi-router.go` | /open/v1/* 路由注册 |
| Modify | `router/api-router.go` | 注册 /api/openapp/ 路由 |
| Modify | `router/main.go` | 注册 Open API 路由 |
| Create | `web/classic/src/components/settings/OpenAppSetting.jsx` | 容器组件 |
| Create | `web/classic/src/pages/Setting/OpenApp/OpenAppList.jsx` | 列表 |
| Create | `web/classic/src/pages/Setting/OpenApp/AddEditOpenAppModal.jsx` | 弹窗 |
| Modify | `web/classic/src/pages/Setting/index.jsx` | 添加 Tab |
| Modify | `web/classic/src/App.jsx` | 确保路由正确（不需要额外修改，Settings 已有路由） |

---

### Task 1: DTO 和返回格式辅助函数

**Files:**
- Create: `dto/openapi.go`
- Create: `common/openapi_response.go`

- [ ] **Step 1: 创建 DTO 结构体文件**

```go
// dto/openapi.go
package dto

// 签发 APIKey 请求
type IssueApiKeyRequest struct {
	Iccid  string `json:"iccid"`
	Email  string `json:"email"`
	PlanId int    `json:"planid"`
}

// 签发 APIKey 响应 data
type IssueApiKeyData struct {
	ApiKey       string `json:"apiKey"`
	TokenTotal   int64  `json:"tokenTotal"`
	ValidEndDate string `json:"validEndDate"`
}

// 查询使用量请求
type TokenUsageRequest struct {
	Iccid string `json:"iccid"`
}

// 使用明细
type UsageDetailItem struct {
	UsedAt     string `json:"usedAt"`
	TokenCount int    `json:"tokenCount"`
	Scene      string `json:"scene"`
	RequestId  string `json:"requestId"`
	Remark     string `json:"remark"`
}

// 查询使用量响应 data
type TokenUsageData struct {
	TokenTotal   int64             `json:"tokenTotal"`
	TokenUsed    int64             `json:"tokenUsed"`
	RemainToken  int64             `json:"remainToken"`
	ValidEndDate string            `json:"validEndDate"`
	UsageDetails []UsageDetailItem `json:"usageDetails"`
}

// 后台列表项
type OpenAppItem struct {
	Id                  int    `json:"id"`
	AppId               string `json:"app_id"`
	Name                string `json:"name"`
	Status              int    `json:"status"`
	IpWhitelistEnabled  bool   `json:"ip_whitelist_enabled"`
	AllowIps            string `json:"allow_ips"`
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
}

// 后台新增/编辑请求
type OpenAppRequest struct {
	Id                 int    `json:"id"`
	AppId              string `json:"app_id"`
	Name               string `json:"name"`
	Status             int    `json:"status"`
	IpWhitelistEnabled bool   `json:"ip_whitelist_enabled"`
	AllowIps           string `json:"allow_ips"`
}
```

- [ ] **Step 2: 创建返回格式辅助函数**

```go
// common/openapi_response.go
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
```

- [ ] **Step 3: 提交**

```bash
git add dto/openapi.go common/openapi_response.go
git commit -m "$(cat <<'EOF'
feat(openapi): add DTOs and response helpers for Open API

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: OpenApp 模型、缓存、CRUD

**Files:**
- Create: `model/open_app.go`

- [ ] **Step 1: 创建模型文件**

```go
// model/open_app.go
package model

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
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

// Redis key prefixes
const (
	openAppCacheKey        = "openapp:"
	openAppCacheExpiration = 300 * time.Second
)

// GenerateAppSecret 生成 32 位随机字符串
func GenerateAppSecret() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// cacheSetOpenApp 将 OpenApp 缓存到 Redis
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

// cacheGetOpenApp 从 Redis 获取 OpenApp
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

// cacheDeleteOpenApp 删除 Redis 缓存
func cacheDeleteOpenApp(appId string) {
	if !common.RedisEnabled {
		return
	}
	common.RedisDelete(openAppCacheKey + appId)
}

// GetOpenAppByAppId 根据 AppId 查询，先查 Redis 再查 DB
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

// GetAllOpenApps 获取所有开放应用列表
func GetAllOpenApps(startIdx int, num int) (apps []*OpenApp, total int64, err error) {
	err = DB.Model(&OpenApp{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = DB.Order("id desc").Limit(num).Offset(startIdx).Find(&apps).Error
	return apps, total, err
}

// InsertOpenApp 新增开放应用
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

// UpdateOpenApp 更新开放应用
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

// DeleteOpenAppById 删除开放应用
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

// GetOpenAppById 根据 ID 查询
func GetOpenAppById(id int) (*OpenApp, error) {
	var app OpenApp
	err := DB.Where("id = ?", id).First(&app).Error
	return &app, err
}

// RefreshOpenAppSecret 刷新 AppSecret
func RefreshOpenAppSecret(id int) (string, error) {
	secret, err := GenerateAppSecret()
	if err != nil {
		return "", err
	}
	err = DB.Model(&OpenApp{}).Where("id = ?", id).Update("app_secret", secret).Error
	if err != nil {
		return "", err
	}
	// 刷新后更新缓存
	app, err := GetOpenAppById(id)
	if err != nil {
		return "", err
	}
	cacheSetOpenApp(app)
	return secret, nil
}
```

- [ ] **Step 2: 验证编译**

```bash
cd e:/ExerProjects/shuzihua-api/new-api && go build ./...
```

- [ ] **Step 3: 提交**

```bash
git add model/open_app.go
git commit -m "$(cat <<'EOF'
feat(openapi): add OpenApp model with cache and CRUD

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: User 模型 ICCID 字段 + 迁移注册

**Files:**
- Modify: `model/user.go`
- Modify: `model/main.go`

- [ ] **Step 1: 在 User 结构体中新增 ICCID 字段**

编辑 `model/user.go`，在 `Group` 字段之后（第 43 行附近）添加：

```go
Iccid            string         `json:"iccid" gorm:"type:varchar(32);uniqueIndex;default:''"`
```

同时更新 `ToBaseUser` 方法（第 58-69 行），在 `UserBase` 中添加 `Iccid` 字段。先检查 `UserBase` 结构体定义位置：

```bash
cd e:/ExerProjects/shuzihua-api/new-api && grep -n "type UserBase struct" model/user.go
```

然后在该结构体中添加 `Iccid string` 字段，并在 `ToBaseUser` 方法中添加 `Iccid: user.Iccid`。

- [ ] **Step 2: 在迁移列表中注册 OpenApp**

编辑 `model/main.go`，在 `migrateDB` 函数的 `DB.AutoMigrate` 参数列表中，在现有模型后面添加 `&OpenApp{}`（找到 `&PerfMetric{},` 行，在其后面添加）：

```go
&OpenApp{},
```

- [ ] **Step 3: 验证编译**

```bash
cd e:/ExerProjects/shuzihua-api/new-api && go build ./...
```

- [ ] **Step 4: 提交**

```bash
git add model/user.go model/main.go
git commit -m "$(cat <<'EOF'
feat(openapi): add ICCID field to User model and register OpenApp migration

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Context Key 注册

**Files:**
- Modify: `constant/context_key.go`

- [ ] **Step 1: 添加 OpenApp 上下文 Key**

在 `constant/context_key.go` 的 `ContextKeyUserEmail` 行之后添加：

```go
ContextKeyOpenAppId     ContextKey = "open_app_id"
ContextKeyOpenAppName   ContextKey = "open_app_name"
```

- [ ] **Step 2: 提交**

```bash
git add constant/context_key.go
git commit -m "$(cat <<'EOF'
feat(openapi): add OpenApp context keys

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: OpenAuth 鉴权中间件 + 限流

**Files:**
- Create: `middleware/open_auth.go`

- [ ] **Step 1: 创建中间件文件**

```go
// middleware/open_auth.go
package middleware

import (
	"context"
	"crypto/md5"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

const (
	openapiTimestampMaxDiff = 300 // 秒
	openapiIccidRateTTL     = 3600
	openapiEmailRateTTL     = 86400
	openapiIccidHistoryTTL  = 60
)

// openApiAbort 返回 Open API 格式错误并中止请求
func openApiAbort(c *gin.Context, code string, message string) {
	c.JSON(http.StatusOK, gin.H{
		"code":    code,
		"message": message,
		"data":    nil,
	})
	c.Abort()
}

func OpenAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		appId := c.GetHeader("X-AppId")
		timestamp := c.GetHeader("X-Timestamp")
		sign := c.GetHeader("X-Sign")

		if appId == "" || timestamp == "" || sign == "" {
			openApiAbort(c, "10009", "签名错误、AppId 非法或时间戳过期")
			return
		}

		// 2. 查 OpenApp
		openApp, err := model.GetOpenAppByAppId(appId)
		if err != nil || openApp.Status != 1 {
			openApiAbort(c, "10009", "签名错误、AppId 非法或时间戳过期")
			return
		}

		// 3. IP 白名单校验
		if openApp.IpWhitelistEnabled && openApp.AllowIps != "" {
			clientIP := c.ClientIP()
			allowed := false
			for _, ip := range strings.Split(openApp.AllowIps, ",") {
				if strings.TrimSpace(ip) == clientIP {
					allowed = true
					break
				}
			}
			if !allowed {
				openApiAbort(c, "10009", "签名错误、AppId 非法或时间戳过期")
				return
			}
		}

		// 4. 时间戳校验
		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil || math.Abs(float64(time.Now().Unix()-ts)) > openapiTimestampMaxDiff {
			openApiAbort(c, "10009", "签名错误、AppId 非法或时间戳过期")
			return
		}

		// 5. 签名校验
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			openApiAbort(c, "99999", "平台系统内部异常")
			return
		}
		bodyBytes, err := storage.Bytes()
		if err != nil {
			openApiAbort(c, "99999", "平台系统内部异常")
			return
		}
		// 重置以便后续 Unmarshal
		if _, seekErr := storage.Seek(0, 0); seekErr != nil {
			openApiAbort(c, "99999", "平台系统内部异常")
			return
		}

		expectSign := fmt.Sprintf("%x", md5.Sum([]byte(appId+openApp.AppSecret+timestamp+string(bodyBytes))))
		if expectSign != sign {
			openApiAbort(c, "10009", "签名错误、AppId 非法或时间戳过期")
			return
		}

		// 6. 注入 Context
		common.SetContextKey(c, constant.ContextKeyOpenAppId, openApp.AppId)
		common.SetContextKey(c, constant.ContextKeyOpenAppName, openApp.AppName)

		c.Next()
	}
}

// checkIccidRateLimit ICCID 申领频次限流（1 小时 20 次）
func checkIccidRateLimit(c *gin.Context, iccid string) bool {
	if !common.RedisEnabled {
		openApiAbort(c, "99999", "平台系统内部异常")
		return false
	}
	ctx := context.Background()
	key := "openapi:ratelimit:iccid:" + iccid + ":1h"
	count, err := common.RDB.Incr(ctx, key).Result()
	if err != nil {
		openApiAbort(c, "99999", "平台系统内部异常")
		return false
	}
	if count == 1 {
		common.RDB.Expire(ctx, key, openapiIccidRateTTL*time.Second)
	}
	if count > 20 {
		openApiAbort(c, "10008", "触发接口限流，请求频次超限")
		return false
	}
	return true
}

// checkEmailRateLimit 邮箱每日绑定 ICCID 数限流（单日 5 个不同 ICCID）
func checkEmailRateLimit(c *gin.Context, email string, iccid string) bool {
	if !common.RedisEnabled {
		openApiAbort(c, "99999", "平台系统内部异常")
		return false
	}
	ctx := context.Background()
	key := "openapi:ratelimit:email:" + email + ":1d"
	added, err := common.RDB.SAdd(ctx, key, iccid).Result()
	if err != nil {
		openApiAbort(c, "99999", "平台系统内部异常")
		return false
	}
	common.RDB.Expire(ctx, key, openapiEmailRateTTL*time.Second)
	if added > 0 {
		// 新 ICCID 加入集合，检查集合大小
		size, err := common.RDB.SCard(ctx, key).Result()
		if err != nil {
			openApiAbort(c, "99999", "平台系统内部异常")
			return false
		}
		if size > 5 {
			// 超过限制，回退此次添加
			common.RDB.SRem(ctx, key, iccid)
			openApiAbort(c, "10008", "触发接口限流，请求频次超限")
			return false
		}
	}
	return true
}

// checkConsecutiveIccidRateLimit 连续 ICCID 风控（连续 ≥5 个步进+1 的 ICCID 直接拦截）
func checkConsecutiveIccidRateLimit(c *gin.Context, appId string, iccid string) bool {
	if !common.RedisEnabled {
		openApiAbort(c, "99999", "平台系统内部异常")
		return false
	}
	// 取 ICCID 最后 6 位
	last6 := iccid[len(iccid)-6:]
	lastNum, err := strconv.Atoi(last6)
	if err != nil {
		return true // 非纯数字不触发连续风控
	}

	ctx := context.Background()
	key := "openapi:iccid_history:" + appId

	// 获取已有历史列表
	history, err := common.RDB.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		openApiAbort(c, "99999", "平台系统内部异常")
		return false
	}

	// 判断新 ICCID 是否与已有列表形成连续序列
	if len(history) >= 4 {
		nums := make([]int, 0, len(history)+1)
		nums = append(nums, lastNum)
		for _, h := range history {
			if n, err := strconv.Atoi(h); err == nil {
				nums = append(nums, n)
			}
		}
		// 排序后检查是否有 ≥5 个连续步进+1 的序列
		sortInts(nums)
		consecutive := 1
		maxConsecutive := 1
		for i := 1; i < len(nums); i++ {
			if nums[i] == nums[i-1]+1 {
				consecutive++
				if consecutive > maxConsecutive {
					maxConsecutive = consecutive
				}
			} else {
				consecutive = 1
			}
		}
		if maxConsecutive >= 5 {
			openApiAbort(c, "10010", "疑似爬虫批量遍历连续 ICCID，请求拦截")
			return false
		}
	}

	// 将新 ICCID 最后 6 位加入列表头部
	common.RDB.LPush(ctx, key, last6)
	// 只保留最近 5 个
	common.RDB.LTrim(ctx, key, 0, 4)
	common.RDB.Expire(ctx, key, openapiIccidHistoryTTL*time.Second)

	return true
}

// sortInts 简单的整数排序（避免 import sort）
func sortInts(nums []int) {
	for i := 0; i < len(nums); i++ {
		for j := i + 1; j < len(nums); j++ {
			if nums[i] > nums[j] {
				nums[i], nums[j] = nums[j], nums[i]
			}
		}
	}
}

// IssueRateLimit 签发接口专用限流中间件
func IssueRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		appId := common.GetContextKeyString(c, constant.ContextKeyOpenAppId)
		_ = appId // 预留
		c.Next()
	}
}
```

注意：签发接口的三个限流需要在 Controller 层调用，因为在中间件层还没有解析 Body 拿到 ICCID 和 email。中间件通过 OpenAuth 后，在 Controller 中拿到请求参数后调用 `checkIccidRateLimit`、`checkEmailRateLimit`、`checkConsecutiveIccidRateLimit` 三个函数。

- [ ] **Step 2: 验证编译**

```bash
cd e:/ExerProjects/shuzihua-api/new-api && go build ./...
```

- [ ] **Step 3: 提交**

```bash
git add middleware/open_auth.go
git commit -m "$(cat <<'EOF'
feat(openapi): add OpenAuth signature middleware and rate limiting

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Service 层 — 签发 + 查询业务逻辑

**Files:**
- Create: `service/openapi.go`

- [ ] **Step 1: 创建 Service 文件**

```go
// service/openapi.go
package service

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// IssueApiKey 签发 APIKey
func IssueApiKey(c *gin.Context, req *dto.IssueApiKeyRequest, appId string) (*dto.IssueApiKeyData, string, error) {
	// 1. 查 ICCID 是否已有用户
	user, err := model.GetUserByIccid(req.Iccid)
	if err == nil && user != nil {
		// ICCID 已绑定用户
		if user.Email != req.Email {
			// 邮箱不一致 → 10005 data=null
			return nil, "10005", nil
		}

		// 邮箱一致，检查是否有有效套餐和有效 APIKey
		sub, subErr := model.GetActiveUserSubscription(user.Id)
		if subErr != nil || sub == nil {
			// 无有效套餐 → 10004
			return nil, "10004", nil
		}

		token, tokenErr := model.GetFirstValidTokenByUserId(user.Id)
		if tokenErr != nil || token == nil {
			// 无有效 APIKey → 10004
			return nil, "10004", nil
		}

		// 有有效套餐 + 有效 APIKey → 10005 + 已有数据
		validEndDate := time.Unix(sub.EndTime, 0).Format("2006-01-02")
		return &dto.IssueApiKeyData{
			ApiKey:       "sk-" + token.Key,
			TokenTotal:   sub.AmountTotal,
			ValidEndDate: validEndDate,
		}, "10005", nil
	}

	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, "99999", err
	}

	// 2. ICCID 未绑定用户：创建新用户
	username := "esim_" + common.GetRandomString(16)
	newUser := &model.User{
		Username:    username,
		Password:    common.GetRandomString(24),
		DisplayName: "",
		Role:        1,
		Status:      1,
		Email:       req.Email,
		Iccid:       req.Iccid,
		Group:       "default",
		Quota:       0, // ICCID 用户无钱包额度
	}

	if err := newUser.Insert(); err != nil {
		return nil, "99999", fmt.Errorf("failed to create user: %w", err)
	}

	// 3. 查套餐
	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil || plan == nil || !plan.Enabled {
		return nil, "99999", fmt.Errorf("subscription plan not found or disabled: id=%d", req.PlanId)
	}

	// 4. 创建订阅记录
	now := time.Now().Unix()
	endTime := now + plan.GetDurationSeconds()

	sub := &model.UserSubscription{
		UserId:       newUser.Id,
		PlanId:       plan.Id,
		AmountTotal:  plan.TotalAmount,
		AmountUsed:   0,
		StartTime:    now,
		EndTime:      endTime,
		Status:       "active",
		Source:       "openapi",
		UpgradeGroup: plan.UpgradeGroup,
	}

	if sub.UpgradeGroup == "" {
		sub.UpgradeGroup = "default"
	}

	if err := model.CreateUserSubscription(sub); err != nil {
		return nil, "99999", fmt.Errorf("failed to create subscription: %w", err)
	}

	// 用户升级分组（若 UpgradeGroup 非空且非默认）
	if plan.UpgradeGroup != "" && plan.UpgradeGroup != "default" {
		sub.PrevUserGroup = "default"
		if err := model.UpgradeUserGroup(newUser.Id, plan.UpgradeGroup); err != nil {
			// 订阅已创建，分组升级失败记录日志但不清退
			common.SysLog(fmt.Sprintf("failed to upgrade user group for user %d: %s", newUser.Id, err.Error()))
		}
	}

	// 5. 生成 APIKey
	key, err := common.GenerateKey()
	if err != nil {
		return nil, "99999", fmt.Errorf("failed to generate key: %w", err)
	}

	token := &model.Token{
		UserId:         newUser.Id,
		Key:            key,
		Status:         1,
		Name:           "ICCID-" + req.Iccid,
		ExpiredTime:    endTime,
		UnlimitedQuota: true,
		Group:          sub.UpgradeGroup,
	}

	if err := token.Insert(); err != nil {
		return nil, "99999", fmt.Errorf("failed to insert token: %w", err)
	}

	// 6. 返回
	validEndDate := time.Unix(endTime, 0).Format("2006-01-02")
	return &dto.IssueApiKeyData{
		ApiKey:       "sk-" + key,
		TokenTotal:   plan.TotalAmount,
		ValidEndDate: validEndDate,
	}, "00000", nil
}

// GetTokenUsage 查询使用量
func GetTokenUsage(iccid string) (*dto.TokenUsageData, string, error) {
	// 1. 查用户
	user, err := model.GetUserByIccid(iccid)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, "10001", nil
		}
		return nil, "99999", err
	}

	// 2. 查有效套餐
	sub, err := model.GetActiveUserSubscription(user.Id)
	if err != nil || sub == nil {
		return nil, "10004", nil
	}

	// 3. 查该用户所有消费日志
	logs, err := model.GetUserConsumeLogs(user.Id)
	if err != nil {
		return nil, "99999", err
	}

	// 4. 组装 usageDetails
	usageDetails := make([]dto.UsageDetailItem, 0, len(logs))
	for _, log := range logs {
		usedAt := time.Unix(log.CreatedAt, 0).Format("2006-01-02 15:04:05")
		usageDetails = append(usageDetails, dto.UsageDetailItem{
			UsedAt:     usedAt,
			TokenCount: log.Quota,
			Scene:      "chat",
			RequestId:  log.RequestId,
			Remark:     "",
		})
	}

	validEndDate := time.Unix(sub.EndTime, 0).Format("2006-01-02")

	return &dto.TokenUsageData{
		TokenTotal:   sub.AmountTotal,
		TokenUsed:    sub.AmountUsed,
		RemainToken:  sub.AmountTotal - sub.AmountUsed,
		ValidEndDate: validEndDate,
		UsageDetails: usageDetails,
	}, "00000", nil
}
```

- [ ] **Step 2: 补充 Model 层缺失的查询方法**

需要在 `model/user.go` 中添加：

```go
// GetUserByIccid 根据 ICCID 查询用户
func GetUserByIccid(iccid string) (*User, error) {
	var user User
	err := DB.Where("iccid = ?", iccid).First(&user).Error
	return &user, err
}
```

需要在 `model/token.go` 中添加：

```go
// GetFirstValidTokenByUserId 获取用户第一把有效 APIKey
// 有效条件：Status=1, (ExpiredTime=-1 或 ExpiredTime>now), DeletedAt IS NULL
func GetFirstValidTokenByUserId(userId int) (*Token, error) {
	var token Token
	now := common.GetTimestamp()
	err := DB.Where("user_id = ? AND status = 1 AND deleted_at IS NULL AND (expired_time = -1 OR expired_time > ?)", userId, now).
		Order("id asc").First(&token).Error
	return &token, err
}
```

需要在 `model/subscription.go` 中添加：

```go
// GetActiveUserSubscription 获取用户的活跃订阅
func GetActiveUserSubscription(userId int) (*UserSubscription, error) {
	var sub UserSubscription
	now := time.Now().Unix()
	err := DB.Where("user_id = ? AND status = 'active' AND end_time > ?", userId, now).
		Order("id desc").First(&sub).Error
	return &sub, err
}

// CreateUserSubscription 创建订阅记录（支持跨 DB）
func CreateUserSubscription(sub *UserSubscription) error {
	return DB.Create(sub).Error
}

// UpgradeUserGroup 升级用户分组
func UpgradeUserGroup(userId int, group string) error {
	return DB.Model(&User{}).Where("id = ?", userId).Update("group", group).Error
}

// GetSubscriptionPlanById 根据 ID 查套餐
func GetSubscriptionPlanById(id int) (*SubscriptionPlan, error) {
	var plan SubscriptionPlan
	err := DB.Where("id = ?", id).First(&plan).Error
	return &plan, err
}
```

需要在 `model/log.go` 中添加：

```go
// GetUserConsumeLogs 获取用户所有消费日志
func GetUserConsumeLogs(userId int) ([]*Log, error) {
	var logs []*Log
	err := LOG_DB.Where("user_id = ? AND type = ?", userId, LogTypeConsume).
		Order("created_at desc").Find(&logs).Error
	return logs, err
}
```

检查 `GetSubscriptionPlanById` 和 `GetActiveUserSubscription` 是否已存在，若已存在则复用。

- [ ] **Step 3: 验证编译**

```bash
cd e:/ExerProjects/shuzihua-api/new-api && go build ./...
```

- [ ] **Step 4: 提交**

```bash
git add service/openapi.go model/user.go model/token.go model/subscription.go model/log.go
git commit -m "$(cat <<'EOF'
feat(openapi): add issue/query service logic and model helpers

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Controller 层 — Open API 端点 + 后台管理

**Files:**
- Create: `controller/openapi.go`
- Create: `controller/openapp.go`

- [ ] **Step 1: 创建 Open API Controller**

```go
// controller/openapi.go
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

// IssueApiKey POST /open/v1/apikey/issue
func IssueApiKey(c *gin.Context) {
	var req dto.IssueApiKeyRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.OpenApiError(c, "10009", "签名错误、AppId 非法或时间戳过期")
		return
	}

	// 邮箱长度校验
	if len(req.Email) > 50 {
		common.OpenApiError(c, "10002", "邮箱格式不符合规范")
		return
	}

	appId := common.GetContextKeyString(c, constant.ContextKeyOpenAppId)

	// 限流检查（在 Controller 层执行，因为需要请求参数）
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
		messages := map[string]string{
			"10005": "该ICCID已完成APIKey申领，不可重复签发",
			"10004": "APIKey 无效、已过期或已作废",
		}
		msg, ok := messages[code]
		if !ok {
			msg = "平台系统内部异常"
			code = "99999"
		}
		common.OpenApiError(c, code, msg)
		return
	}

	if code == "10005" {
		common.OpenApiError(c, code, "该ICCID已完成APIKey申领，不可重复签发")
		return
	}

	common.OpenApiSuccess(c, data)
}

// GetTokenUsage POST /open/v1/token/usage
func GetTokenUsage(c *gin.Context) {
	var req dto.TokenUsageRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.OpenApiError(c, "10009", "签名错误、AppId 非法或时间戳过期")
		return
	}

	data, code, err := service.GetTokenUsage(req.Iccid)
	if err != nil {
		messages := map[string]string{
			"10001": "iccid 不存在或未开通",
			"10004": "APIKey 无效、已过期或已作废",
		}
		msg, ok := messages[code]
		if !ok {
			msg = "平台系统内部异常"
			code = "99999"
		}
		common.OpenApiError(c, code, msg)
		return
	}

	common.OpenApiSuccess(c, data)
}
```

**注意：** 签发接口中 `IssueApiKey` service 返回 code="10005" 且 data 非 nil 时表示"已有有效 APIKey 应返回已有数据"。需要在 service 层区分"10005 + 有 data"和"10005 + 无 data"。修正 service 返回值为 `(data, code, error)` 三元组，Controller 据此判断。

- [ ] **Step 2: 创建后台管理 Controller**

```go
// controller/openapp.go
package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetAllOpenApps GET /api/openapp/
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

// GetOpenApp GET /api/openapp/:id
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
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    app,
	})
}

// AddOpenApp POST /api/openapp/
func AddOpenApp(c *gin.Context) {
	var app model.OpenApp
	if err := common.UnmarshalBodyReusable(c, &app); err != nil {
		common.ApiError(c, err)
		return
	}
	// AppId 自动生成
	if app.AppId == "" {
		app.AppId = "app_" + common.GetRandomString(16)
	}
	if err := model.InsertOpenApp(&app); err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回时显式包含 AppSecret（仅创建时返回一次）
	common.ApiSuccess(c, gin.H{
		"id":                  app.Id,
		"app_id":              app.AppId,
		"app_secret":          app.AppSecret,
		"name":                app.Name,
		"status":              app.Status,
		"ip_whitelist_enabled": app.IpWhitelistEnabled,
		"allow_ips":           app.AllowIps,
		"created_at":          app.CreatedAt,
		"updated_at":          app.UpdatedAt,
	})
}

// UpdateOpenApp PUT /api/openapp/
func UpdateOpenApp(c *gin.Context) {
	var req dto.OpenAppRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	app := &model.OpenApp{
		Id:                 req.Id,
		Name:               req.Name,
		Status:             req.Status,
		IpWhitelistEnabled: req.IpWhitelistEnabled,
		AllowIps:           req.AllowIps,
	}
	// 需要先从 DB 读取 AppId 用于缓存失效
	existing, err := model.GetOpenAppById(req.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	app.AppId = existing.AppId
	if err := model.UpdateOpenApp(app); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, app)
}

// DeleteOpenApp DELETE /api/openapp/:id
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

// GetOpenAppKey POST /api/openapp/:id/key
// 查看/刷新 AppSecret，需要 SecureVerification
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
```

- [ ] **Step 3: 修正 Service 返回 triplet 以支持区分 10005 有无 data**

在 `service/openapi.go` 中，`IssueApiKey` 已经返回 `(*dto.IssueApiKeyData, string, error)`，Controller 需要分别处理：
- `code == "10005" && data != nil` → 返回 data 中的已有信息（注意：需要用 `OpenApiSuccess` 而非 `OpenApiError`）
- `code == "10005" && data == nil` → 返回 10005 + data:null（标准 OpenApiError）

更新 `controller/openapi.go` 中 `IssueApiKey` 函数，将 10005 分支改为：

```go
	if code == "10005" {
		if data != nil {
			// 邮箱一致，返回已有 APIKey 信息
			c.JSON(http.StatusOK, gin.H{
				"code":    "10005",
				"message": "该ICCID已完成APIKey申领，不可重复签发",
				"data":    data,
			})
		} else {
			common.OpenApiError(c, "10005", "该ICCID已完成APIKey申领，不可重复签发")
		}
		return
	}
```

- [ ] **Step 4: 验证编译**

```bash
cd e:/ExerProjects/shuzihua-api/new-api && go build ./...
```

- [ ] **Step 5: 提交**

```bash
git add controller/openapi.go controller/openapp.go
git commit -m "$(cat <<'EOF'
feat(openapi): add Open API and admin management controllers

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: 路由注册

**Files:**
- Create: `router/openapi-router.go`
- Modify: `router/main.go`
- Modify: `router/api-router.go`

- [ ] **Step 1: 创建 Open API 路由文件**

```go
// router/openapi-router.go
package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetOpenApiRouter(router *gin.Engine) {
	openRouter := router.Group("/open/v1")
	openRouter.Use(middleware.RouteTag("openapi"))
	openRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	openRouter.Use(middleware.BodyStorageCleanup())
	openRouter.Use(middleware.OpenAuth())
	{
		openRouter.POST("/apikey/issue", controller.IssueApiKey)
		openRouter.POST("/token/usage", controller.GetTokenUsage)
	}
}
```

- [ ] **Step 2: 在 main router 中注册 Open API 路由**

编辑 `router/main.go`，在 `SetRouter` 函数中添加：

```go
SetOpenApiRouter(router)
```

（放在现有 `SetApiRouter`、`SetDashboardRouter` 等调用之间）

- [ ] **Step 3: 在 api-router 中注册后台管理路由**

编辑 `router/api-router.go`，在适当位置（例如 `redemptionRoute` 附近）添加：

```go
openAppRoute := apiRouter.Group("/openapp")
openAppRoute.Use(middleware.AdminAuth())
{
    openAppRoute.GET("/", controller.GetAllOpenApps)
    openAppRoute.GET("/search", controller.GetAllOpenApps)
    openAppRoute.GET("/:id", controller.GetOpenApp)
    openAppRoute.POST("/", controller.AddOpenApp)
    openAppRoute.PUT("/", controller.UpdateOpenApp)
    openAppRoute.DELETE("/:id", controller.DeleteOpenApp)
    openAppRoute.POST("/:id/key",
        middleware.CriticalRateLimit(),
        middleware.SecureVerificationRequired(),
        controller.GetOpenAppKey,
    )
}
```

- [ ] **Step 4: 验证编译**

```bash
cd e:/ExerProjects/shuzihua-api/new-api && go build ./...
```

- [ ] **Step 5: 提交**

```bash
git add router/openapi-router.go router/main.go router/api-router.go
git commit -m "$(cat <<'EOF'
feat(openapi): register Open API and admin management routes

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: 前端 — OpenAppSetting 容器组件

**Files:**
- Create: `web/classic/src/components/settings/OpenAppSetting.jsx`

- [ ] **Step 1: 创建容器组件**

参考现有的 `OperationSetting.jsx` 模式：

```jsx
// web/classic/src/components/settings/OpenAppSetting.jsx
import React, { useEffect, useState } from 'react';
import { Card, Spin } from '@douyinfe/semi-ui';
import OpenAppList from '../../pages/Setting/OpenApp/OpenAppList';
import { API } from '../../helpers';

const OpenAppSetting = () => {
  const [loading, setLoading] = useState(false);
  const [apps, setApps] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const pageSize = 10;

  const fetchApps = async (p = 0) => {
    setLoading(true);
    try {
      const res = await API.get(`/api/openapp/?p=${p}&page_size=${pageSize}`);
      const { success, data, total } = await res.json();
      if (success) {
        setApps(data || []);
        setTotal(total || 0);
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchApps(page);
  }, [page]);

  return (
    <Card>
      <Spin spinning={loading}>
        <OpenAppList
          apps={apps}
          total={total}
          page={page}
          pageSize={pageSize}
          onPageChange={setPage}
          onRefresh={() => fetchApps(page)}
        />
      </Spin>
    </Card>
  );
};

export default OpenAppSetting;
```

- [ ] **Step 2: 提交**

```bash
git add web/classic/src/components/settings/OpenAppSetting.jsx
git commit -m "$(cat <<'EOF'
feat(openapi): add OpenApp settings container component

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 10: 前端 — OpenAppList 列表组件

**Files:**
- Create: `web/classic/src/pages/Setting/OpenApp/OpenAppList.jsx`

- [ ] **Step 1: 创建列表组件**

```jsx
// web/classic/src/pages/Setting/OpenApp/OpenAppList.jsx
import React, { useState } from 'react';
import { Button, Table, Tag, Switch, Popconfirm, Space } from '@douyinfe/semi-ui';
import { IconEdit, IconDelete, IconPlus, IconKey, IconRefresh } from '@douyinfe/semi-icons';
import AddEditOpenAppModal from './AddEditOpenAppModal';
import { API, showError, showSuccess } from '../../../helpers';

const OpenAppList = ({ apps, total, page, pageSize, onPageChange, onRefresh }) => {
  const [modalVisible, setModalVisible] = useState(false);
  const [editingApp, setEditingApp] = useState(null);

  const handleDelete = async (id) => {
    try {
      const res = await API.delete(`/api/openapp/${id}`);
      const { success, message } = await res.json();
      if (success) {
        showSuccess('删除成功');
        onRefresh();
      } else {
        showError(message);
      }
    } catch (e) {
      showError('删除失败');
    }
  };

  const handleViewKey = async (id) => {
    try {
      const res = await API.post(`/api/openapp/${id}/key`);
      const { success, data, message } = await res.json();
      if (success) {
        showSuccess(`AppSecret: ${data.app_secret}`);
      } else {
        showError(message);
      }
    } catch (e) {
      showError('获取失败');
    }
  };

  const handleRefreshKey = async (id) => {
    try {
      const res = await API.post(`/api/openapp/${id}/key?action=refresh`);
      const { success, data, message } = await res.json();
      if (success) {
        showSuccess(`新 AppSecret: ${data.app_secret}`);
        onRefresh();
      } else {
        showError(message);
      }
    } catch (e) {
      showError('刷新失败');
    }
  };

  const columns = [
    { title: 'AppId', dataIndex: 'app_id', key: 'app_id' },
    { title: '名称', dataIndex: 'name', key: 'name' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => (
        <Tag color={status === 1 ? 'green' : 'red'}>
          {status === 1 ? '启用' : '禁用'}
        </Tag>
      ),
    },
    {
      title: 'IP 白名单开关',
      dataIndex: 'ip_whitelist_enabled',
      key: 'ip_whitelist_enabled',
      render: (val) => (val ? '开启' : '关闭'),
    },
    { title: 'IP 白名单', dataIndex: 'allow_ips', key: 'allow_ips' },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (ts) => ts ? new Date(ts * 1000).toLocaleString() : '-',
    },
    {
      title: '操作',
      key: 'actions',
      render: (_, record) => (
        <Space>
          <Button
            icon={<IconEdit />}
            size='small'
            onClick={() => { setEditingApp(record); setModalVisible(true); }}
          />
          <Button
            icon={<IconKey />}
            size='small'
            onClick={() => handleViewKey(record.id)}
          />
          <Button
            icon={<IconRefresh />}
            size='small'
            onClick={() => handleRefreshKey(record.id)}
          />
          <Popconfirm
            title='确定删除该应用？'
            onConfirm={() => handleDelete(record.id)}
          >
            <Button icon={<IconDelete />} size='small' type='danger' />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <h3>开放应用设置</h3>
        <Button
          icon={<IconPlus />}
          theme='solid'
          onClick={() => { setEditingApp(null); setModalVisible(true); }}
        >
          新增应用
        </Button>
      </div>
      <Table
        columns={columns}
        dataSource={apps}
        rowKey='id'
        pagination={{
          currentPage: page + 1,
          pageSize,
          total,
          onChange: (p) => onPageChange(p - 1),
        }}
      />
      <AddEditOpenAppModal
        visible={modalVisible}
        app={editingApp}
        onClose={() => setModalVisible(false)}
        onSuccess={() => { setModalVisible(false); onRefresh(); }}
      />
    </>
  );
};

export default OpenAppList;
```

- [ ] **Step 2: 提交**

```bash
git add web/classic/src/pages/Setting/OpenApp/OpenAppList.jsx
git commit -m "$(cat <<'EOF'
feat(openapi): add OpenApp list component

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 11: 前端 — AddEditOpenAppModal 弹窗

**Files:**
- Create: `web/classic/src/pages/Setting/OpenApp/AddEditOpenAppModal.jsx`

- [ ] **Step 1: 创建弹窗组件**

```jsx
// web/classic/src/pages/Setting/OpenApp/AddEditOpenAppModal.jsx
import React, { useState } from 'react';
import { Modal, Form, Button, Switch as SemiSwitch } from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../../helpers';

const AddEditOpenAppModal = ({ visible, app, onClose, onSuccess }) => {
  const [loading, setLoading] = useState(false);
  const isEdit = !!app;

  const handleSubmit = async (values) => {
    setLoading(true);
    try {
      const payload = {
        id: app?.id || 0,
        app_id: values.app_id || '',
        name: values.name,
        status: values.status ? 1 : 0,
        ip_whitelist_enabled: values.ip_whitelist_enabled,
        allow_ips: values.allow_ips || '',
      };
      const method = isEdit ? API.put : API.post;
      const res = await method('/api/openapp/', payload);
      const { success, data, message } = await res.json();
      if (success) {
        showSuccess(isEdit ? '编辑成功' : '新增成功');
        // 新增成功后展示 AppSecret
        if (!isEdit && data?.app_secret) {
          showSuccess(`AppSecret: ${data.app_secret}（请妥善保管，仅显示一次）`);
        }
        onSuccess();
      } else {
        showError(message);
      }
    } catch (e) {
      showError('操作失败');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      title={isEdit ? '编辑开放应用' : '新增开放应用'}
      visible={visible}
      onCancel={onClose}
      footer={null}
    >
      <Form
        onSubmit={handleSubmit}
        initValues={app ? {
          app_id: app.app_id,
          name: app.name,
          status: app.status === 1,
          ip_whitelist_enabled: app.ip_whitelist_enabled,
          allow_ips: app.allow_ips,
        } : {
          status: true,
          ip_whitelist_enabled: false,
        }}
      >
        {!isEdit && (
          <Form.Input field='app_id' label='AppId' placeholder='留空自动生成' />
        )}
        <Form.Input
          field='name'
          label='名称'
          rules={[{ required: true, message: '请输入应用名称' }]}
        />
        <Form.Slot label='状态' field='status'>
          <SemiSwitch defaultChecked={true} />
        </Form.Slot>
        <Form.Slot label='IP 白名单开关' field='ip_whitelist_enabled'>
          <SemiSwitch />
        </Form.Slot>
        <Form.TextArea
          field='allow_ips'
          label='IP 白名单'
          placeholder='多个 IP 用逗号分隔，如 192.168.1.1,10.0.0.0/24'
        />
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <Button onClick={onClose}>取消</Button>
          <Button type='primary' htmlType='submit' loading={loading}>
            {isEdit ? '保存' : '创建'}
          </Button>
        </div>
      </Form>
    </Modal>
  );
};

export default AddEditOpenAppModal;
```

- [ ] **Step 2: 提交**

```bash
git add web/classic/src/pages/Setting/OpenApp/AddEditOpenAppModal.jsx
git commit -m "$(cat <<'EOF'
feat(openapi): add OpenApp modal component

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 12: 前端 — 集成到设置页

**Files:**
- Modify: `web/classic/src/pages/Setting/index.jsx`

- [ ] **Step 1: 在 Setting 页添加"开放应用"标签**

1. 在文件顶部导入中添加：
```jsx
import OpenAppSetting from '../../components/settings/OpenAppSetting';
import { AppWindow } from 'lucide-react';
```

2. 在 `isRoot()` 条件内，最后一个 pane（`other`）之后，`isRoot()` 闭合大括号之前，添加：
```jsx
panes.push({
  tab: (
    <span style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
      <AppWindow size={18} />
      {t('开放应用设置')}
    </span>
  ),
  content: <OpenAppSetting />,
  itemKey: 'openapp',
});
```

- [ ] **Step 2: 提交**

```bash
git add web/classic/src/pages/Setting/index.jsx
git commit -m "$(cat <<'EOF'
feat(openapi): add OpenApp tab to settings page

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 13: 集成验证与修正

**Files:** 不新增，修复编译/运行时问题。

- [ ] **Step 1: 全量编译验证**

```bash
cd e:/ExerProjects/shuzihua-api/new-api && go build ./...
```

修复所有编译错误。常见的可能问题：
- `common.RedisSet` / `common.RedisGet` / `common.RedisDelete` 函数名是否正确（检查 `common/redis.go`）
- `common.GetTimestamp()` 是否存在（检查 `common/utils.go`）
- `SubscriptionPlan.GetDurationSeconds()` 方法是否存在（可能需要手动计算）
- `model.GetSubscriptionPlanById` 与现有函数名冲突（检查是否已存在）
- `model.GetUserConsumeLogs` 中 `LOG_DB` 是否正确（确认是 `LOG_DB` 还是其他变量名）
- `common.GetRandomString` 是否已导出（检查函数签名）

- [ ] **Step 2: API 测试**

启动服务后测试接口：

```bash
# 先创建一个 OpenApp（通过后台管理页面或直接 DB 插入）
# 记录 AppId 和 AppSecret 用于签名计算

# 签发 APIKey
curl -X POST http://localhost:3000/open/v1/apikey/issue \
  -H "Content-Type: application/json" \
  -H "X-AppId: <app_id>" \
  -H "X-Timestamp: $(date +%s)" \
  -H "X-Sign: <计算得到的签名>" \
  -d '{"iccid":"89860012345678901234","email":"test@example.com","planid":1}'

# 查询使用量
curl -X POST http://localhost:3000/open/v1/token/usage \
  -H "Content-Type: application/json" \
  -H "X-AppId: <app_id>" \
  -H "X-Timestamp: $(date +%s)" \
  -H "X-Sign: <计算得到的签名>" \
  -d '{"iccid":"89860012345678901234"}'
```

- [ ] **Step 3: 前端验证**

```bash
cd web/classic && bun run dev
```

访问 `http://localhost:3000/console/setting?tab=openapp` 确认：
1. "开放应用设置"标签可见
2. 新增应用功能正常
3. 编辑、删除、查看密钥、刷新密钥功能正常
4. IP 白名单开关和输入框正常工作

- [ ] **Step 4: 提交最终修复**

```bash
git add -A
git commit -m "$(cat <<'EOF'
fix(openapi): integration fixes from verification

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Model 辅助方法补充清单

以下方法需要在对应 model 文件中添加（如果不存在），Task 6 Step 2 中已列出代码：

| 方法 | 文件 | 用途 |
|------|------|------|
| `GetUserByIccid(iccid)` | `model/user.go` | 根据 ICCID 查用户 |
| `GetFirstValidTokenByUserId(userId)` | `model/token.go` | 获取用户第一把有效 APIKey |
| `GetActiveUserSubscription(userId)` | `model/subscription.go` | 获取用户活跃订阅 |
| `CreateUserSubscription(sub)` | `model/subscription.go` | 创建订阅记录 |
| `UpgradeUserGroup(userId, group)` | `model/subscription.go` | 升级用户分组 |
| `GetSubscriptionPlanById(id)` | `model/subscription.go` | 查套餐 |
| `GetUserConsumeLogs(userId)` | `model/log.go` | 获取用户所有消费日志 |
| `GetUserQuota(userId, forceDB)` | `model/user.go` | 获取用户额度（已有，确认签名） |
| `HasActiveUserSubscription(userId)` | `model/subscription.go` | 检查是否有活跃订阅（已有，确认签名） |

## 限流函数导出说明

三个限流检查函数（`CheckIccidRateLimit`、`CheckEmailRateLimit`、`CheckConsecutiveIccidRateLimit`）需要从小写改为大写导出，因为 Controller 需要调用它们。更新 `middleware/open_auth.go` 中的函数名为导出形式。

## 签名计算方法

调用方计算签名的示例（用于测试）：

```bash
APP_ID="your_app_id"
APP_SECRET="your_app_secret"
TIMESTAMP=$(date +%s)
BODY='{"iccid":"89860012345678901234","email":"test@example.com","planid":1}'
SIGN=$(echo -n "${APP_ID}${APP_SECRET}${TIMESTAMP}${BODY}" | md5sum | cut -d' ' -f1)
```
