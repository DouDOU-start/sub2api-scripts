// Claude OAuth 账号协议状态批量扫描脚本
// 通过 sub2api 管理员 API 扫描所有 Claude OAuth 账号，识别需要接受协议的账号
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"
)

// 账号状态
type ScanStatus string

const (
	StatusOK          ScanStatus = "正常"
	StatusNeedTerms   ScanStatus = "需要接受协议"
	StatusAuthFailed  ScanStatus = "认证失败"
	StatusError       ScanStatus = "其他错误"
	StatusRateLimited ScanStatus = "速率限制"
	StatusOverloaded  ScanStatus = "服务过载"
)

// sub2api 响应结构
type APIResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type PaginatedData struct {
	Items json.RawMessage `json:"items"`
	Total int             `json:"total"`
	Page  int             `json:"page"`
	Pages int             `json:"pages"`
}

type Account struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Type   string `json:"type"`
}

// SSE 事件
type TestEvent struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Model   string `json:"model,omitempty"`
	Success bool   `json:"success,omitempty"`
	Error   string `json:"error,omitempty"`
}

// 扫描结果
type ScanResult struct {
	Account Account
	Status  ScanStatus
	Detail  string
}

// loadEnvFile 从 .env 文件加载配置到环境变量（不覆盖已有环境变量）
func loadEnvFile() {
	// 从可执行文件所在目录向上查找 .env
	dir, _ := os.Getwd()
	for {
		envPath := filepath.Join(dir, ".env")
		if f, err := os.Open(envPath); err == nil {
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if k, v, ok := strings.Cut(line, "="); ok {
					k = strings.TrimSpace(k)
					v = strings.TrimSpace(v)
					// 不覆盖已有环境变量
					if os.Getenv(k) == "" {
						os.Setenv(k, v)
					}
				}
			}
			f.Close()
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

// getConfig 按优先级获取配置: 命令行参数 > 环境变量 > 默认值
func getConfig(flagVal, envKey, defaultVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return defaultVal
}

func main() {
	// 先加载 .env 文件
	loadEnvFile()

	apiURL := flag.String("api-url", "", "sub2api 服务地址")
	apiKey := flag.String("api-key", "", "管理员 API Key")
	model := flag.String("model", "", "测试用模型（留空使用默认）")
	flag.Parse()

	// 按优先级解析配置
	finalURL := getConfig(*apiURL, "SUB2API_URL", "http://localhost:8080")
	finalKey := getConfig(*apiKey, "SUB2API_KEY", "")
	finalModel := getConfig(*model, "SUB2API_MODEL", "")

	if finalKey == "" {
		fmt.Fprintln(os.Stderr, "错误: 未找到 API Key，请通过以下方式之一提供:")
		fmt.Fprintln(os.Stderr, "  1. 命令行参数: --api-key=xxx")
		fmt.Fprintln(os.Stderr, "  2. 环境变量:   export SUB2API_KEY=xxx")
		fmt.Fprintln(os.Stderr, "  3. 配置文件:   在项目根目录创建 .env 文件（参考 .env.example）")
		os.Exit(1)
	}

	client := &http.Client{Timeout: 120 * time.Second}

	// 1. 获取所有 Claude OAuth 账号
	fmt.Printf("服务地址: %s\n", finalURL)
	fmt.Println("正在获取 Claude OAuth 账号列表...")
	accounts, err := fetchAccounts(client, finalURL, finalKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取账号列表失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("共找到 %d 个 Claude OAuth 账号\n\n", len(accounts))

	if len(accounts) == 0 {
		fmt.Println("没有找到需要扫描的账号")
		return
	}

	// 2. 逐个测试连接
	var results []ScanResult
	for i, acc := range accounts {
		fmt.Printf("[%d/%d] 测试 %s (ID: %d)...", i+1, len(accounts), acc.Name, acc.ID)
		result := testAccount(client, finalURL, finalKey, acc, finalModel)
		results = append(results, result)
		fmt.Printf(" %s\n", result.Status)

		// 避免过于频繁请求
		if i < len(accounts)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 3. 输出汇总
	printResults(results)
}

// fetchAccounts 获取所有 Claude OAuth 活跃账号
func fetchAccounts(client *http.Client, apiURL, apiKey string) ([]Account, error) {
	var allAccounts []Account
	page := 1

	for {
		url := fmt.Sprintf("%s/api/v1/admin/accounts?platform=anthropic&type=oauth&status=active&page=%d&page_size=100", apiURL, page)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-api-key", apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("请求失败: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("API 返回 %d", resp.StatusCode)
		}

		var apiResp APIResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return nil, fmt.Errorf("解析响应失败: %w", err)
		}
		if apiResp.Code != 0 {
			return nil, fmt.Errorf("API 错误: %s", apiResp.Message)
		}

		var paginated PaginatedData
		if err := json.Unmarshal(apiResp.Data, &paginated); err != nil {
			return nil, fmt.Errorf("解析分页数据失败: %w", err)
		}

		var accounts []Account
		if err := json.Unmarshal(paginated.Items, &accounts); err != nil {
			return nil, fmt.Errorf("解析账号列表失败: %w", err)
		}

		allAccounts = append(allAccounts, accounts...)

		if page >= paginated.Pages {
			break
		}
		page++
	}

	return allAccounts, nil
}

// testAccount 测试单个账号连接，解析 SSE 流
func testAccount(client *http.Client, apiURL, apiKey string, acc Account, model string) ScanResult {
	url := fmt.Sprintf("%s/api/v1/admin/accounts/%d/test", apiURL, acc.ID)

	body := "{}"
	if model != "" {
		body = fmt.Sprintf(`{"model_id":"%s"}`, model)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return ScanResult{Account: acc, Status: StatusError, Detail: err.Error()}
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		return ScanResult{Account: acc, Status: StatusError, Detail: err.Error()}
	}
	defer resp.Body.Close()

	// 解析 SSE 流
	scanner := bufio.NewScanner(resp.Body)
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
				return ScanResult{Account: acc, Status: StatusOK, Detail: "-"}
			}
		case "error":
			lastError = event.Error
			// 识别协议问题
			if containsTermsError(event.Error) {
				return ScanResult{Account: acc, Status: StatusNeedTerms, Detail: "需要在 claude.ai 接受 Consumer Terms and Privacy Policy"}
			}
			// 识别认证失败
			if strings.Contains(event.Error, "401") || strings.Contains(event.Error, "403") || strings.Contains(event.Error, "Unauthorized") {
				return ScanResult{Account: acc, Status: StatusAuthFailed, Detail: event.Error}
			}
			// 识别速率限制
			if strings.Contains(event.Error, "429") || strings.Contains(event.Error, "rate_limit") {
				return ScanResult{Account: acc, Status: StatusRateLimited, Detail: event.Error}
			}
			// 识别过载
			if strings.Contains(event.Error, "529") || strings.Contains(event.Error, "overloaded") {
				return ScanResult{Account: acc, Status: StatusOverloaded, Detail: event.Error}
			}
		}
	}

	if lastError != "" {
		return ScanResult{Account: acc, Status: StatusError, Detail: lastError}
	}
	if gotContent {
		return ScanResult{Account: acc, Status: StatusOK, Detail: "-"}
	}
	return ScanResult{Account: acc, Status: StatusError, Detail: "未收到有效响应"}
}

// containsTermsError 检测是否为协议未接受错误
func containsTermsError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return strings.Contains(lower, "consumer terms") ||
		strings.Contains(lower, "privacy policy") ||
		strings.Contains(lower, "accept them in claude.ai") ||
		strings.Contains(lower, "/status to continue")
}

// printResults 打印汇总结果
func printResults(results []ScanResult) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("扫描结果")
	fmt.Println(strings.Repeat("=", 80))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\t名称\t状态\t详情")
	fmt.Fprintln(w, "--\t----\t----\t----")

	countOK, countTerms, countAuth, countRate, countOverload, countErr := 0, 0, 0, 0, 0, 0
	for _, r := range results {
		detail := r.Detail
		if len(detail) > 60 {
			detail = detail[:60] + "..."
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", r.Account.ID, r.Account.Name, r.Status, detail)

		switch r.Status {
		case StatusOK:
			countOK++
		case StatusNeedTerms:
			countTerms++
		case StatusAuthFailed:
			countAuth++
		case StatusRateLimited:
			countRate++
		case StatusOverloaded:
			countOverload++
		case StatusError:
			countErr++
		}
	}
	w.Flush()

	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("总计: %d 个账号\n", len(results))
	fmt.Printf("  正常: %d | 需要接受协议: %d | 认证失败: %d | 速率限制: %d | 过载: %d | 其他错误: %d\n",
		countOK, countTerms, countAuth, countRate, countOverload, countErr)
}
