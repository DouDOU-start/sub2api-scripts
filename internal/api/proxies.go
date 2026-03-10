package api

import "encoding/json"

// FetchProxies 获取所有代理
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
