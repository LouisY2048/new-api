package middleware

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

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
			openApiAbort(c, "10009", "Missing required headers: X-AppId, X-Timestamp, X-Sign")
			return
		}

		app, err := model.GetOpenAppByAppId(appId)
		if err != nil || app.Status != 1 {
			openApiAbort(c, "10009", "Invalid AppId or app is disabled")
			return
		}

		if app.IpWhitelistEnabled && app.AllowIps != "" {
			clientIP := c.ClientIP()
			allowed := false
			for _, ip := range strings.Split(app.AllowIps, ",") {
				if strings.TrimSpace(ip) == clientIP {
					allowed = true
					break
				}
			}
			if !allowed {
				openApiAbort(c, "10009", "IP not in whitelist")
				return
			}
		}

		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			openApiAbort(c, "10009", "Invalid timestamp format")
			return
		}
		now := time.Now().Unix()
		diff := now - ts
		if diff < 0 {
			diff = -diff
		}
		if diff > 300 {
			openApiAbort(c, "10009", "Timestamp expired")
			return
		}

		storage, err := common.GetBodyStorage(c)
		if err != nil {
			openApiAbort(c, "10009", "Failed to read request body")
			return
		}
		body, err := storage.Bytes()
		if err != nil {
			openApiAbort(c, "10009", "Failed to read request body")
			return
		}

		signStr := appId + app.AppSecret + timestamp + string(body)
		hash := md5.Sum([]byte(signStr))
		expectedSign := hex.EncodeToString(hash[:])
		if expectedSign != sign {
			openApiAbort(c, "10009", "Invalid signature")
			return
		}

		if _, err := storage.Seek(0, 0); err != nil {
			openApiAbort(c, "10009", "Failed to reset request body")
			return
		}

		common.SetContextKey(c, constant.ContextKeyOpenAppId, appId)
		common.SetContextKey(c, constant.ContextKeyOpenAppName, app.Name)

		c.Next()
	}
}

// CheckIccidRateLimit checks ICCID-based rate limiting (20 per hour per ICCID).
// Returns true if the request passes, false if it is rate limited.
func CheckIccidRateLimit(c *gin.Context, iccid string) bool {
	if !common.RedisEnabled {
		return true
	}

	ctx := context.Background()
	key := fmt.Sprintf("openapi:ratelimit:iccid:%s:1h", iccid)

	count, err := common.RDB.Incr(ctx, key).Result()
	if err != nil {
		return true
	}

	if count == 1 {
		common.RDB.Expire(ctx, key, 3600*time.Second)
	}

	if count > 20 {
		openApiAbort(c, "10008", "ICCID rate limit exceeded")
		return false
	}

	return true
}

// CheckEmailRateLimit checks email-based rate limiting (5 unique ICCIDs per day per email).
// Returns true if the request passes, false if it is rate limited.
func CheckEmailRateLimit(c *gin.Context, email string, iccid string) bool {
	if !common.RedisEnabled {
		return true
	}

	ctx := context.Background()
	key := fmt.Sprintf("openapi:ratelimit:email:%s:1d", email)

	added, err := common.RDB.SAdd(ctx, key, iccid).Result()
	if err != nil {
		return true
	}

	common.RDB.Expire(ctx, key, 86400*time.Second)

	if added > 0 {
		card, err := common.RDB.SCard(ctx, key).Result()
		if err != nil {
			return true
		}
		if card > 5 {
			common.RDB.SRem(ctx, key, iccid)
			openApiAbort(c, "10008", "Email rate limit exceeded")
			return false
		}
	}

	return true
}

// CheckConsecutiveIccidRateLimit checks for 5 or more consecutive ICCID values
// within the last 5 requests for the same AppId (window: 60 seconds).
// Returns true if the request passes, false if it is rate limited.
func CheckConsecutiveIccidRateLimit(c *gin.Context, appId string, iccid string) bool {
	if !common.RedisEnabled {
		return true
	}

	if len(iccid) < 6 {
		return true
	}

	last6Str := iccid[len(iccid)-6:]
	newVal, err := strconv.Atoi(last6Str)
	if err != nil {
		return true
	}

	ctx := context.Background()
	key := fmt.Sprintf("openapi:iccid_history:%s", appId)

	existing, err := common.RDB.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return true
	}

	vals := []int{newVal}
	for _, s := range existing {
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			continue
		}
		vals = append(vals, v)
	}

	if len(vals) >= 5 {
		sort.Ints(vals)
		for i := 0; i <= len(vals)-5; i++ {
			consecutive := true
			for j := 1; j < 5; j++ {
				if vals[i+j] != vals[i+j-1]+1 {
					consecutive = false
					break
				}
			}
			if consecutive {
				openApiAbort(c, "10010", "Consecutive ICCID rate limit exceeded")
				return false
			}
		}
	}

	common.RDB.LPush(ctx, key, last6Str)
	common.RDB.LTrim(ctx, key, 0, 4)
	common.RDB.Expire(ctx, key, 60*time.Second)

	return true
}
