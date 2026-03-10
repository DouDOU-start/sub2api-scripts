// sub2api API 客户端封装
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client sub2api API 客户端
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewClient 创建 API 客户端
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// doRequest 执行 HTTP 请求并解析响应
func (c *Client) doRequest(method, path string, body any) (*APIResponse, *http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("序列化请求失败: %w", err)
		}
		bodyReader = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("x-api-key", c.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("请求失败: %w", err)
	}

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		resp.Body.Close()
		return nil, nil, fmt.Errorf("解析响应失败: %w", err)
	}
	resp.Body.Close()

	if apiResp.Code != 0 {
		return nil, nil, fmt.Errorf("%s", apiResp.Message)
	}
	return &apiResp, resp, nil
}

// doRawRequest 执行 HTTP 请求，返回原始 response（用于 SSE 流）
func (c *Client) doRawRequest(method, path string, body any, extraHeaders map[string]string) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求失败: %w", err)
		}
		bodyReader = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	return resp, nil
}

// Get 发送 GET 请求
func (c *Client) Get(path string) (*APIResponse, error) {
	apiResp, _, err := c.doRequest("GET", path, nil)
	return apiResp, err
}

// Post 发送 POST 请求
func (c *Client) Post(path string, body any) (*APIResponse, error) {
	apiResp, _, err := c.doRequest("POST", path, body)
	return apiResp, err
}

// Put 发送 PUT 请求
func (c *Client) Put(path string, body any) (*APIResponse, error) {
	apiResp, _, err := c.doRequest("PUT", path, body)
	return apiResp, err
}
