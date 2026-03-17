// 账号相关 API
package api

import (
	"encoding/json"
	"fmt"
)

// FetchAccounts 分页获取所有符合条件的账号
func (c *Client) FetchAccounts(opts AccountListOptions) ([]Account, error) {
	var all []Account
	page := 1

	for {
		path := fmt.Sprintf("/api/v1/admin/accounts?platform=%s&type=%s&page=%d&page_size=100",
			opts.Platform, opts.Type, page)
		if opts.Status != "" {
			path += "&status=" + opts.Status
		}

		apiResp, err := c.Get(path)
		if err != nil {
			return nil, err
		}

		var paginated PaginatedData
		if err := json.Unmarshal(apiResp.Data, &paginated); err != nil {
			return nil, fmt.Errorf("解析分页数据失败: %w", err)
		}

		var accounts []Account
		if err := json.Unmarshal(paginated.Items, &accounts); err != nil {
			return nil, fmt.Errorf("解析账号列表失败: %w", err)
		}

		all = append(all, accounts...)

		if page >= paginated.Pages {
			break
		}
		page++
	}

	return all, nil
}

// FetchAccountMap 获取账号列表并按名称索引（用于去重检测）
func (c *Client) FetchAccountMap(opts AccountListOptions) (map[string]*Account, error) {
	accounts, err := c.FetchAccounts(opts)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*Account, len(accounts))
	for i := range accounts {
		m[accounts[i].Name] = &accounts[i]
	}
	return m, nil
}

// CookieAuth 使用 session_key 换取 token
// accountType: "oauth" 走 /cookie-auth，"setup-token" 走 /setup-token-cookie-auth
func (c *Client) CookieAuth(sessionKey string, proxyID *int64, accountType string) (*TokenInfo, error) {
	reqBody := map[string]any{"code": sessionKey}
	if proxyID != nil {
		reqBody["proxy_id"] = *proxyID
	}

	path := "/api/v1/admin/accounts/cookie-auth"
	if accountType == "setup-token" {
		path = "/api/v1/admin/accounts/setup-token-cookie-auth"
	}

	apiResp, err := c.Post(path, reqBody)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := json.Unmarshal(apiResp.Data, &raw); err != nil {
		return nil, fmt.Errorf("解析 token 失败: %w", err)
	}

	token := &TokenInfo{Raw: raw}
	if v, ok := raw["org_uuid"].(string); ok {
		token.OrgUUID = v
	}
	if v, ok := raw["account_uuid"].(string); ok {
		token.AccountUUID = v
	}
	if v, ok := raw["email_address"].(string); ok {
		token.EmailAddress = v
	}
	return token, nil
}

// BuildCreateRequest 根据 token 信息构建创建账号请求
func BuildCreateRequest(email string, token *TokenInfo, proxyID *int64, groupIDs []int64, platform, accountType string, quota QuotaConfig) CreateAccountRequest {
	extra := map[string]any{
		"enable_tls_fingerprint":     true,
		"session_id_masking_enabled": true,
		"cache_ttl_override_enabled": true,
		"cache_ttl_override_target":  "5m",
	}
	if token.OrgUUID != "" {
		extra["org_uuid"] = token.OrgUUID
	}
	if token.AccountUUID != "" {
		extra["account_uuid"] = token.AccountUUID
	}
	if token.EmailAddress != "" {
		extra["email_address"] = token.EmailAddress
	}

	// 配额限速配置
	if quota.BaseRPM > 0 {
		extra["base_rpm"] = quota.BaseRPM
	}
	if quota.MaxSessions > 0 {
		extra["max_sessions"] = quota.MaxSessions
		extra["session_idle_timeout_minutes"] = quota.SessionIdleTimeoutMinutes
	}
	if quota.WindowCostLimit > 0 {
		extra["window_cost_limit"] = quota.WindowCostLimit
	}

	req := CreateAccountRequest{
		Name:        email,
		Platform:    platform,
		Type:        accountType,
		Credentials: token.Raw,
		Concurrency: 10,
		Priority:    1,
	}
	if quota.RateMultiplier > 0 && quota.RateMultiplier != 1.0 {
		rm := quota.RateMultiplier
		req.RateMultiplier = &rm
	}
	if quota.LoadFactor > 0 {
		lf := quota.LoadFactor
		req.LoadFactor = &lf
	}
	if len(extra) > 0 {
		req.Extra = extra
	}
	if proxyID != nil {
		req.ProxyID = proxyID
	}
	if len(groupIDs) > 0 {
		req.GroupIDs = groupIDs
	}
	return req
}

// BuildQuotaExtra 根据配额配置构建 Extra 字段（用于更新已有账号）
func BuildQuotaExtra(quota QuotaConfig) map[string]any {
	extra := map[string]any{
		"enable_tls_fingerprint":     true,
		"session_id_masking_enabled": true,
		"cache_ttl_override_enabled": true,
		"cache_ttl_override_target":  "5m",
	}
	if quota.BaseRPM > 0 {
		extra["base_rpm"] = quota.BaseRPM
	}
	if quota.MaxSessions > 0 {
		extra["max_sessions"] = quota.MaxSessions
		extra["session_idle_timeout_minutes"] = quota.SessionIdleTimeoutMinutes
	}
	if quota.WindowCostLimit > 0 {
		extra["window_cost_limit"] = quota.WindowCostLimit
	}
	return extra
}

// CreateAccount 创建账号
func (c *Client) CreateAccount(req CreateAccountRequest) (int64, error) {
	apiResp, err := c.Post("/api/v1/admin/accounts", req)
	if err != nil {
		return 0, err
	}

	var account CreatedAccount
	if err := json.Unmarshal(apiResp.Data, &account); err != nil {
		return 0, fmt.Errorf("解析账号失败: %w", err)
	}
	return account.ID, nil
}

// UpdateAccount 更新账号
func (c *Client) UpdateAccount(accountID int64, req UpdateAccountRequest) error {
	path := fmt.Sprintf("/api/v1/admin/accounts/%d", accountID)
	_, err := c.Put(path, req)
	return err
}

// UnbindProxy 解绑账号的代理（proxy_id 设为 0）
func (c *Client) UnbindProxy(accountID int64) error {
	path := fmt.Sprintf("/api/v1/admin/accounts/%d", accountID)
	body := map[string]any{
		"proxy_id":                   0,
		"confirm_mixed_channel_risk": true,
	}
	_, err := c.Put(path, body)
	return err
}

// DisableSchedule 关闭账号调度并记录失败原因
func (c *Client) DisableSchedule(accountID int64, reason string) error {
	notes := "测试失败: " + reason
	schedulable := false
	return c.UpdateAccount(accountID, UpdateAccountRequest{
		Schedulable: &schedulable,
		Notes:       &notes,
	})
}

// RefreshToken 刷新账号令牌
func (c *Client) RefreshToken(accountID int64) error {
	path := fmt.Sprintf("/api/v1/admin/accounts/%d/refresh", accountID)
	_, err := c.Post(path, nil)
	return err
}

// DeleteAccount 删除账号
func (c *Client) DeleteAccount(accountID int64) error {
	path := fmt.Sprintf("/api/v1/admin/accounts/%d", accountID)
	_, err := c.Delete(path)
	return err
}

// EnableSchedule 开启账号调度并清除错误状态
func (c *Client) EnableSchedule(accountID int64) error {
	schedulable := true
	notes := ""
	return c.UpdateAccount(accountID, UpdateAccountRequest{
		Status:      "active",
		Schedulable: &schedulable,
		Notes:       &notes,
	})
}
