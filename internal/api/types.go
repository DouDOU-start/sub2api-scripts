// sub2api API 公共类型定义
package api

import "encoding/json"

// APIResponse sub2api 统一响应格式
type APIResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// PaginatedData 分页数据
type PaginatedData struct {
	Items json.RawMessage `json:"items"`
	Total int             `json:"total"`
	Page  int             `json:"page"`
	Pages int             `json:"pages"`
}

// Account 账号信息
type Account struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	Platform    string         `json:"platform"`
	Status      string         `json:"status"`
	Type        string         `json:"type"`
	Credentials map[string]any `json:"credentials"`
	ProxyID     *int64         `json:"proxy_id"`
	GroupIDs    []int64        `json:"group_ids"`
	Concurrency int            `json:"concurrency"`
	Schedulable bool           `json:"schedulable"`
}

// TokenInfo cookie-auth 返回的 token 信息
type TokenInfo struct {
	Raw          map[string]any // 原始 JSON，直接作为 credentials
	OrgUUID      string
	AccountUUID  string
	EmailAddress string
}

// Proxy 代理
type Proxy struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
	Status   string `json:"status"`
}

// CreateProxyRequest 创建代理请求
type CreateProxyRequest struct {
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// UpdateProxyRequest 更新代理请求
type UpdateProxyRequest struct {
	Name *string `json:"name,omitempty"`
}

// Group 分组
type Group struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Status   string `json:"status"`
}

// TestEvent SSE 测试事件
type TestEvent struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Model   string `json:"model,omitempty"`
	Success bool   `json:"success,omitempty"`
	Error   string `json:"error,omitempty"`
}

// TestResult 账号测试结果
type TestResult struct {
	Success bool
	Error   string
}

// AccountListOptions 获取账号列表的过滤选项
type AccountListOptions struct {
	Platform string // anthropic
	Type     string // oauth, setup-token
	Status   string // active, inactive（留空不过滤）
}

// QuotaConfig 配额限速配置（用于上号时统一设置）
type QuotaConfig struct {
	BaseRPM                   int     // RPM 基准值（0=不限）
	MaxSessions               int     // 最大并发会话基准值（0=不限）
	SessionIdleTimeoutMinutes int     // 会话空闲超时（分钟）
	WindowCostLimit           float64 // 5h 窗口费用基准值（美元，0=不限）
	RateMultiplier            float64 // 计费倍率（0 视为 1.0）
	LoadFactor                int     // 负载因子（0=使用 concurrency）
	Percentages               []int   // 百分比列表，按账号索引循环应用（如 [90,80] 表示 90%,80%）
}

// DefaultQuotaConfig 返回合理的默认配额配置（适合账号多用户多的场景）
func DefaultQuotaConfig() QuotaConfig {
	return QuotaConfig{
		BaseRPM:                   60,
		MaxSessions:               10,
		SessionIdleTimeoutMinutes: 5,
		WindowCostLimit:           80,
		RateMultiplier:            1.0,
		LoadFactor:                0,
	}
}

// ForIndex 根据账号索引和百分比列表计算实际配额
// 百分比为空时直接返回基准值
func (q QuotaConfig) ForIndex(index int) QuotaConfig {
	if len(q.Percentages) == 0 {
		return q
	}
	pct := float64(q.Percentages[index%len(q.Percentages)]) / 100.0
	result := q
	result.BaseRPM = int(float64(q.BaseRPM)*pct + 0.5)
	result.MaxSessions = int(float64(q.MaxSessions)*pct + 0.5)
	result.WindowCostLimit = float64(int(q.WindowCostLimit*pct*100+0.5)) / 100 // 保留两位小数
	result.Percentages = nil // 实际配额不再携带百分比
	return result
}

// CreateAccountRequest 创建账号请求
type CreateAccountRequest struct {
	Name           string         `json:"name"`
	Platform       string         `json:"platform"`
	Type           string         `json:"type"`
	Credentials    map[string]any `json:"credentials"`
	Concurrency    int            `json:"concurrency"`
	Priority       int            `json:"priority"`
	RateMultiplier *float64       `json:"rate_multiplier,omitempty"`
	LoadFactor     *int           `json:"load_factor,omitempty"`
	Extra          map[string]any `json:"extra,omitempty"`
	ProxyID        *int64         `json:"proxy_id,omitempty"`
	GroupIDs       []int64        `json:"group_ids,omitempty"`
}

// UpdateAccountRequest 更新账号请求
type UpdateAccountRequest struct {
	ProxyID                 *int64         `json:"proxy_id,omitempty"`
	GroupIDs                []int64        `json:"group_ids,omitempty"`
	Concurrency             *int           `json:"concurrency,omitempty"`
	Priority                *int           `json:"priority,omitempty"`
	RateMultiplier          *float64       `json:"rate_multiplier,omitempty"`
	LoadFactor              *int           `json:"load_factor,omitempty"`
	Credentials             map[string]any `json:"credentials,omitempty"`
	Extra                   map[string]any `json:"extra,omitempty"`
	Schedulable             *bool          `json:"schedulable,omitempty"`
	Status                  string         `json:"status,omitempty"`
	Notes                   *string        `json:"notes,omitempty"`
	ConfirmMixedChannelRisk bool           `json:"confirm_mixed_channel_risk,omitempty"`
}

// CreatedAccount 创建账号返回
type CreatedAccount struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
