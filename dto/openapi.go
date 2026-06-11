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
