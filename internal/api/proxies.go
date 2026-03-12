package api

import (
	"encoding/json"
	"fmt"
)

// FetchProxies 获取代理列表（通过 /all 接口，仅返回 active，无分页）
func (c *Client) FetchProxies() ([]Proxy, error) {
	apiResp, err := c.Get("/api/v1/admin/proxies/all")
	if err != nil {
		return nil, err
	}
	var proxies []Proxy
	if err := json.Unmarshal(apiResp.Data, &proxies); err != nil {
		return nil, err
	}
	return proxies, nil
}

// FetchProxiesPaginated 通过分页接口获取代理，status 为空时获取所有状态
func (c *Client) FetchProxiesPaginated(status string) ([]Proxy, error) {
	var all []Proxy
	page := 1
	for {
		// 服务端 Limit() 硬限制最大 100，所以用 page_size=100
		path := fmt.Sprintf("/api/v1/admin/proxies?page=%d&page_size=100", page)
		if status != "" {
			path += "&status=" + status
		}
		apiResp, err := c.Get(path)
		if err != nil {
			return nil, err
		}
		var paginated PaginatedData
		if err := json.Unmarshal(apiResp.Data, &paginated); err != nil {
			return nil, fmt.Errorf("解析分页数据失败: %w", err)
		}
		var proxies []Proxy
		if err := json.Unmarshal(paginated.Items, &proxies); err != nil {
			return nil, fmt.Errorf("解析代理列表失败: %w", err)
		}
		all = append(all, proxies...)
		if page >= paginated.Pages {
			break
		}
		page++
	}
	return all, nil
}

// ProxyTestResult 代理测试结果
type ProxyTestResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	IPAddress string `json:"ip_address,omitempty"`
	Country   string `json:"country,omitempty"`
}

// DeleteProxy 删除代理
func (c *Client) DeleteProxy(proxyID int64) error {
	path := fmt.Sprintf("/api/v1/admin/proxies/%d", proxyID)
	_, err := c.Delete(path)
	return err
}

// TestProxy 测试代理连通性
func (c *Client) TestProxy(proxyID int64) (*ProxyTestResult, error) {
	path := fmt.Sprintf("/api/v1/admin/proxies/%d/test", proxyID)
	apiResp, err := c.Post(path, nil)
	if err != nil {
		return nil, err
	}

	var result ProxyTestResult
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("解析测试结果失败: %w", err)
	}
	return &result, nil
}
