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
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Type        string  `json:"type"`
	ProxyID     *int64  `json:"proxy_id"`
	GroupIDs    []int64 `json:"group_ids"`
	Concurrency int     `json:"concurrency"`
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
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
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

// CreateAccountRequest 创建账号请求
type CreateAccountRequest struct {
	Name        string         `json:"name"`
	Platform    string         `json:"platform"`
	Type        string         `json:"type"`
	Credentials map[string]any `json:"credentials"`
	Concurrency int            `json:"concurrency"`
	Priority    int            `json:"priority"`
	Extra       map[string]any `json:"extra,omitempty"`
	ProxyID     *int64         `json:"proxy_id,omitempty"`
	GroupIDs    []int64        `json:"group_ids,omitempty"`
}

// UpdateAccountRequest 更新账号请求
type UpdateAccountRequest struct {
	ProxyID                 *int64         `json:"proxy_id,omitempty"`
	GroupIDs                []int64        `json:"group_ids,omitempty"`
	Concurrency             *int           `json:"concurrency,omitempty"`
	Priority                *int           `json:"priority,omitempty"`
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
