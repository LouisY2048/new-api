package service

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// IssueApiKey handles the Open API key issuance business logic.
// Returns (data, code, error):
//   - data: response payload on success, nil on error
//   - code: "00000" for success, otherwise an error code string
//   - err: underlying Go error (nil for business-logic rejections like email mismatch)
func IssueApiKey(c *gin.Context, req *dto.IssueApiKeyRequest, appId string) (*dto.IssueApiKeyData, string, error) {
	// 1. Check if user exists by ICCID
	user, err := model.GetUserByIccid(req.Iccid)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// Real database error
		return nil, "99999", err
	}

	if err == nil {
		// User exists
		if user.Email != req.Email {
			// Email mismatch - data should be null
			return nil, "10005", nil
		}

		// Check for active subscription
		hasActiveSub, subErr := model.HasActiveUserSubscription(user.Id)
		if subErr != nil {
			return nil, "99999", subErr
		}

		// Check for valid API key
		token, tokenErr := model.GetFirstValidTokenByUserId(user.Id)

		if hasActiveSub && tokenErr == nil {
			// Existing valid binding: return existing info
			sub, subErr := model.GetActiveUserSubscription(user.Id)
			if subErr != nil {
				return nil, "99999", subErr
			}
			return &dto.IssueApiKeyData{
				ApiKey:       "sk-" + token.Key,
				TokenTotal:   sub.AmountTotal,
				ValidEndDate: time.Unix(sub.EndTime, 0).Format("2006-01-02"),
			}, "10005", nil
		}

		// Has no active subscription or no valid API key
		return nil, "10004", nil
	}

	// 2. ICCID not bound: create new user
	newUser := &model.User{
		Username:    "esim_" + common.GetRandomString(16),
		Password:    common.GetRandomString(24),
		Email:       req.Email,
		Iccid:       req.Iccid,
		Group:       "default",
		Status:      1,
		Role:        1,
		Quota:       0,
		DisplayName: "eSIM User",
	}
	if err := newUser.Insert(0); err != nil {
		return nil, "99999", err
	}

	// 3. Look up plan
	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		return nil, "99999", err
	}
	if !plan.Enabled {
		return nil, "99999", errors.New("plan is disabled")
	}

	// 4. Generate API key and create subscription + token in a transaction
	tokenKey, err := common.GenerateKey()
	if err != nil {
		return nil, "99999", err
	}

	var sub *model.UserSubscription
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var txErr error
		sub, txErr = model.CreateUserSubscriptionFromPlanTx(tx, newUser.Id, plan, "openapi")
		if txErr != nil {
			return txErr
		}

		// 5. Create API key token within the same transaction
		tokenGroup := sub.UpgradeGroup
		if tokenGroup == "" {
			tokenGroup = "default"
		}
		token := &model.Token{
			UserId:         newUser.Id,
			Key:            tokenKey,
			Status:         1,
			Name:           "ICCID-" + req.Iccid,
			CreatedTime:    common.GetTimestamp(),
			ExpiredTime:    sub.EndTime,
			UnlimitedQuota: true,
			Group:          tokenGroup,
		}
		return tx.Create(token).Error
	})
	if err != nil {
		return nil, "99999", err
	}

	// 6. Return data
	return &dto.IssueApiKeyData{
		ApiKey:       "sk-" + tokenKey,
		TokenTotal:   plan.TotalAmount,
		ValidEndDate: time.Unix(sub.EndTime, 0).Format("2006-01-02"),
	}, "00000", nil
}

// GetTokenUsage handles the Open API token usage query business logic.
// Returns (data, code, error):
//   - data: usage response payload on success, nil on error
//   - code: "00000" for success, otherwise an error code string
//   - err: underlying Go error (nil for business-logic rejections like user not found)
func GetTokenUsage(iccid string) (*dto.TokenUsageData, string, error) {
	// 1. Get user by ICCID
	user, err := model.GetUserByIccid(iccid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "10001", nil
		}
		return nil, "99999", err
	}

	// 2. Get active subscription
	sub, err := model.GetActiveUserSubscription(user.Id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "10004", nil
		}
		return nil, "99999", err
	}

	// 3. Get consume logs
	logs, err := model.GetUserConsumeLogs(user.Id)
	if err != nil {
		return nil, "99999", err
	}

	// 4. Map logs to UsageDetailItem slice
	details := make([]dto.UsageDetailItem, 0, len(logs))
	for _, log := range logs {
		details = append(details, dto.UsageDetailItem{
			UsedAt:     time.Unix(log.CreatedAt, 0).Format("2006-01-02 15:04:05"),
			TokenCount: log.Quota,
			Scene:      "chat",
			RequestId:  log.RequestId,
			Remark:     "",
		})
	}

	// 5. Return data
	return &dto.TokenUsageData{
		TokenTotal:   sub.AmountTotal,
		TokenUsed:    sub.AmountUsed,
		RemainToken:  sub.AmountTotal - sub.AmountUsed,
		ValidEndDate: time.Unix(sub.EndTime, 0).Format("2006-01-02"),
		UsageDetails: details,
	}, "00000", nil
}

// getPlanDurationSeconds computes the duration in seconds for a subscription plan.
func getPlanDurationSeconds(plan *model.SubscriptionPlan) int64 {
	switch plan.DurationUnit {
	case "day":
		return int64(plan.DurationValue) * 86400
	case "month":
		return int64(plan.DurationValue) * 30 * 86400
	case "year":
		return int64(plan.DurationValue) * 365 * 86400
	case "hour":
		return int64(plan.DurationValue) * 3600
	case "custom":
		return plan.CustomSeconds
	default:
		return 180 * 86400 // default 180 days
	}
}
