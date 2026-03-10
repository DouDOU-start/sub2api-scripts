// SSE 流解析：账号测试连接
package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
)

// TestAccount 测试账号连接（简单版，只返回 error）
func (c *Client) TestAccount(accountID int64, model string) error {
	result := c.TestAccountDetail(accountID, model)
	if !result.Success {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

// TestAccountDetail 测试账号连接，返回详细结果
func (c *Client) TestAccountDetail(accountID int64, model string) TestResult {
	path := fmt.Sprintf("/api/v1/admin/accounts/%d/test", accountID)

	var body any
	if model != "" {
		body = map[string]string{"model_id": model}
	} else {
		body = map[string]any{}
	}

	resp, err := c.doRawRequest("POST", path, body, map[string]string{
		"Accept": "text/event-stream",
	})
	if err != nil {
		return TestResult{Error: err.Error()}
	}
	defer resp.Body.Close()

	return parseSSEStream(resp.Body)
}

// parseSSEStream 解析 SSE 流
func parseSSEStream(body interface{ Read([]byte) (int, error) }) TestResult {
	scanner := bufio.NewScanner(body)
	var lastError string
	gotContent := false

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var event TestEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content":
			gotContent = true
		case "test_complete":
			if event.Success || gotContent {
				return TestResult{Success: true}
			}
		case "error":
			lastError = event.Error
		}
	}

	if gotContent {
		return TestResult{Success: true}
	}
	if lastError != "" {
		return TestResult{Error: lastError}
	}
	return TestResult{Error: "未收到有效响应"}
}

// ClassifyError 根据错误信息分类（用于 terms-scan 等需要细分错误类型的场景）
func ClassifyError(errMsg string) string {
	lower := strings.ToLower(errMsg)
	switch {
	case strings.Contains(lower, "consumer terms") ||
		strings.Contains(lower, "privacy policy") ||
		strings.Contains(lower, "accept them in claude.ai") ||
		strings.Contains(lower, "/status to continue"):
		return "need_terms"
	case strings.Contains(errMsg, "401") || strings.Contains(errMsg, "403") || strings.Contains(errMsg, "Unauthorized"):
		return "auth_failed"
	case strings.Contains(errMsg, "429") || strings.Contains(errMsg, "rate_limit"):
		return "rate_limited"
	case strings.Contains(errMsg, "529") || strings.Contains(errMsg, "overloaded"):
		return "overloaded"
	default:
		return "error"
	}
}
