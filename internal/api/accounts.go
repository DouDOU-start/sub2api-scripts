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
func (c *Client) CookieAuth(sessionKey string, proxyID *int64) (*TokenInfo, error) {
	reqBody := map[string]any{"code": sessionKey}
	if proxyID != nil {
		reqBody["proxy_id"] = *proxyID
	}

	apiResp, err := c.Post("/api/v1/admin/accounts/cookie-auth", reqBody)
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
func BuildCreateRequest(email string, token *TokenInfo, proxyID *int64, groupIDs []int64) CreateAccountRequest {
	extra := map[string]any{}
	if token.OrgUUID != "" {
		extra["org_uuid"] = token.OrgUUID
	}
	if token.AccountUUID != "" {
		extra["account_uuid"] = token.AccountUUID
	}
	if token.EmailAddress != "" {
		extra["email_address"] = token.EmailAddress
	}

	req := CreateAccountRequest{
		Name:        email,
		Platform:    "anthropic",
		Type:        "oauth",
		Credentials: token.Raw,
		Concurrency: 10,
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

// DisableSchedule 关闭账号调度并记录失败原因
func (c *Client) DisableSchedule(accountID int64, reason string) error {
	notes := "测试失败: " + reason
	schedulable := false
	return c.UpdateAccount(accountID, UpdateAccountRequest{
		Schedulable: &schedulable,
		Notes:       &notes,
	})
}
