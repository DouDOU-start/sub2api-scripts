// Claude OAuth 账号批量添加脚本
// 从 stdin 或文件读取账号信息，批量添加到 sub2api 并验证
// 格式: email----password----session_key（每行一个）
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
	"time"
)

// API 响应
type APIResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// cookie-auth 返回的 token 信息（保留原始 JSON 用于 credentials）
type TokenInfo struct {
	Raw          map[string]any // 原始 JSON，直接作为 credentials
	OrgUUID      string
	AccountUUID  string
	EmailAddress string
}

// 创建账号返回
type CreatedAccount struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// SSE 事件
type TestEvent struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Success bool   `json:"success,omitempty"`
	Error   string `json:"error,omitempty"`
}

// 分页数据
type PaginatedData struct {
	Items json.RawMessage `json:"items"`
	Total int             `json:"total"`
	Page  int             `json:"page"`
	Pages int             `json:"pages"`
}

// 已有账号（用于去重和更新检测）
type ExistingAccount struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	ProxyID     *int64  `json:"proxy_id"`
	GroupIDs    []int64 `json:"group_ids"`
	Concurrency int     `json:"concurrency"`
}

// 代理
type Proxy struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

// 分组
type Group struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Status   string `json:"status"`
}

// 添加结果状态
type AddStatus string

const (
	StatusSuccess    AddStatus = "成功"
	StatusSkipped    AddStatus = "已跳过"
	StatusUpdated    AddStatus = "已更新"
	StatusAuthFail   AddStatus = "认证失败"
	StatusAddFail    AddStatus = "创建失败"
	StatusTestFail   AddStatus = "测试失败"
	StatusUpdateFail AddStatus = "更新失败"
)

// 单条结果
type AddResult struct {
	Email     string
	Status    AddStatus
	Detail    string
	AccountID int64
}

func main() {
	loadEnvFile()

	apiURL := flag.String("api-url", "", "sub2api 服务地址")
	apiKey := flag.String("api-key", "", "管理员 API Key")
	model := flag.String("model", "", "测试用模型（留空使用默认）")
	proxyID := flag.Int64("proxy", -1, "绑定代理 ID（-1=交互选择，0=不绑定）")
	input := flag.String("input", "", "账号文件路径（每行一个，格式: email----password----session_key）")
	output := flag.String("output", "batch-add-result.txt", "结果输出文件")
	flag.Parse()

	finalURL := getConfig(*apiURL, "SUB2API_URL", "http://localhost:8080")
	finalKey := getConfig(*apiKey, "SUB2API_KEY", "")
	finalModel := getConfig(*model, "SUB2API_MODEL", "")

	if finalKey == "" {
		fmt.Fprintln(os.Stderr, "错误: 未找到 API Key，请通过 --api-key、环境变量 SUB2API_KEY 或 .env 文件提供")
		os.Exit(1)
	}

	client := &http.Client{Timeout: 120 * time.Second}

	// 代理选择
	var selectedProxy *int64
	if *proxyID == -1 {
		// 交互选择代理
		proxies, err := fetchProxies(client, finalURL, finalKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取代理列表失败: %v\n", err)
			os.Exit(1)
		}
		if len(proxies) == 0 {
			fmt.Println("没有可用的代理，将不绑定代理")
		} else {
			fmt.Println("可用代理列表:")
			fmt.Println("  0. 不绑定代理")
			for i, p := range proxies {
				fmt.Printf("  %d. [ID:%d] %s (%s)\n", i+1, p.ID, p.Name, p.Address)
			}
			fmt.Print("请选择代理编号: ")
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				var choice int
				if _, err := fmt.Sscanf(scanner.Text(), "%d", &choice); err == nil && choice >= 1 && choice <= len(proxies) {
					id := proxies[choice-1].ID
					selectedProxy = &id
					fmt.Printf("已选择代理: %s (ID: %d)\n", proxies[choice-1].Name, id)
				}
			}
		}
	} else if *proxyID > 0 {
		selectedProxy = proxyID
		fmt.Printf("绑定代理 ID: %d\n", *proxyID)
	}

	// 分组选择
	var selectedGroupIDs []int64
	groups, err := fetchGroups(client, finalURL, finalKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取分组列表失败: %v，将不绑定分组\n", err)
	} else if len(groups) > 0 {
		fmt.Println("\n可用分组列表（多选用逗号分隔，如 1,3；直接回车跳过）:")
		for i, g := range groups {
			fmt.Printf("  %d. [ID:%d] %s (%s)\n", i+1, g.ID, g.Name, g.Platform)
		}
		fmt.Print("请选择分组编号: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			if text != "" {
				for _, part := range strings.Split(text, ",") {
					var choice int
					if _, err := fmt.Sscanf(strings.TrimSpace(part), "%d", &choice); err == nil && choice >= 1 && choice <= len(groups) {
						selectedGroupIDs = append(selectedGroupIDs, groups[choice-1].ID)
					}
				}
				if len(selectedGroupIDs) > 0 {
					names := make([]string, len(selectedGroupIDs))
					for i, id := range selectedGroupIDs {
						for _, g := range groups {
							if g.ID == id {
								names[i] = g.Name
								break
							}
						}
					}
					fmt.Printf("已选择分组: %s\n", strings.Join(names, ", "))
				}
			}
		}
	}

	// 读取账号：优先从文件，否则从 stdin
	var lines []string
	if *input != "" {
		f, err := os.Open(*input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "打开文件失败: %v\n", err)
			os.Exit(1)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			lines = append(lines, line)
		}
		f.Close()
		fmt.Printf("从文件 %s 读取到 %d 个账号\n", *input, len(lines))
	} else {
		fmt.Println("请输入账号信息（格式: email----password----session_key，每行一个，空行结束）:")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				break
			}
			lines = append(lines, line)
		}
	}

	if len(lines) == 0 {
		fmt.Println("未读取到任何账号")
		return
	}
	fmt.Printf("\n共读取 %d 个账号\n", len(lines))

	// 获取已有账号，用于跳过重复或补充代理/分组
	fmt.Println("正在查询已有账号...")
	existingAccounts, err := fetchExistingAccounts(client, finalURL, finalKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "查询已有账号失败: %v，将不跳过重复\n", err)
		existingAccounts = make(map[string]*ExistingAccount)
	} else {
		fmt.Printf("已有 %d 个 Claude OAuth 账号\n", len(existingAccounts))
	}

	fmt.Printf("开始处理...\n\n")

	var results []AddResult

	for i, line := range lines {
		parts := strings.SplitN(line, "----", 3)
		if len(parts) != 3 {
			fmt.Printf("[%d/%d] 格式错误，跳过: %s\n", i+1, len(lines), line)
			results = append(results, AddResult{Email: line, Status: StatusAddFail, Detail: "格式错误，需要 email----password----session_key"})
			continue
		}
		email := strings.TrimSpace(parts[0])
		// password 暂未使用（sub2api 通过 session_key 认证）
		sessionKey := strings.TrimSpace(parts[2])

		fmt.Printf("[%d/%d] %s", i+1, len(lines), email)

		// 检查已存在的账号
		if existing, ok := existingAccounts[email]; ok {
			// 检查是否需要补充代理、分组或容量
			needProxy := selectedProxy != nil && existing.ProxyID == nil
			needGroups := len(selectedGroupIDs) > 0 && len(existing.GroupIDs) == 0
			needConcurrency := existing.Concurrency == 0

			if !needProxy && !needGroups && !needConcurrency {
				fmt.Printf(" -> 已存在，跳过\n")
				results = append(results, AddResult{Email: email, Status: StatusSkipped, Detail: "账号已存在", AccountID: existing.ID})
				continue
			}

			// 补充代理/分组/容量
			var updates []string
			var updateProxy *int64
			var updateGroups []int64
			var concurrency *int
			if needProxy {
				updateProxy = selectedProxy
				updates = append(updates, "代理")
			}
			if needGroups {
				updateGroups = selectedGroupIDs
				updates = append(updates, "分组")
			}
			if needConcurrency {
				c := 10
				concurrency = &c
				updates = append(updates, "容量")
			}
			fmt.Printf(" -> 已存在，补充%s...", strings.Join(updates, "+"))
			if err := updateAccount(client, finalURL, finalKey, existing.ID, updateProxy, updateGroups, concurrency); err != nil {
				fmt.Printf(" 失败: %v\n", err)
				results = append(results, AddResult{Email: email, Status: StatusUpdateFail, Detail: err.Error(), AccountID: existing.ID})
			} else {
				fmt.Printf(" OK\n")
				results = append(results, AddResult{Email: email, Status: StatusUpdated, Detail: "补充" + strings.Join(updates, "+"), AccountID: existing.ID})
			}
			continue
		}
		fmt.Println()

		// 步骤1: cookie-auth 换 token
		fmt.Printf("  认证中...")
		tokenInfo, err := cookieAuth(client, finalURL, finalKey, sessionKey, selectedProxy)
		if err != nil {
			fmt.Printf(" 失败: %v\n", err)
			results = append(results, AddResult{Email: email, Status: StatusAuthFail, Detail: err.Error()})
			continue
		}
		fmt.Printf(" OK\n")

		// 步骤2: 创建账号
		fmt.Printf("  创建账号...")
		accountID, err := createAccount(client, finalURL, finalKey, email, tokenInfo, selectedProxy, selectedGroupIDs)
		if err != nil {
			fmt.Printf(" 失败: %v\n", err)
			results = append(results, AddResult{Email: email, Status: StatusAddFail, Detail: err.Error()})
			continue
		}
		fmt.Printf(" OK (ID: %d)\n", accountID)

		// 步骤3: 测试连接
		fmt.Printf("  测试连接...")
		testErr := testAccount(client, finalURL, finalKey, accountID, finalModel)
		if testErr != nil {
			fmt.Printf(" 失败: %v\n", testErr)
			// 禁用账号并标记失败原因
			fmt.Printf("  禁用账号...")
			disableErr := disableAccount(client, finalURL, finalKey, accountID, testErr.Error())
			if disableErr != nil {
				fmt.Printf(" 失败: %v\n", disableErr)
			} else {
				fmt.Printf(" OK\n")
			}
			results = append(results, AddResult{Email: email, Status: StatusTestFail, Detail: testErr.Error(), AccountID: accountID})
		} else {
			fmt.Printf(" OK\n")
			results = append(results, AddResult{Email: email, Status: StatusSuccess, Detail: "-", AccountID: accountID})
		}

		// 间隔
		if i < len(lines)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 输出汇总
	printResults(results)

	// 导出到文件
	if err := exportResults(results, *output); err != nil {
		fmt.Fprintf(os.Stderr, "导出结果失败: %v\n", err)
	} else {
		fmt.Printf("\n结果已导出到: %s\n", *output)
	}
}

// fetchExistingAccounts 获取所有已有 Claude OAuth 账号（含代理和分组信息）
func fetchExistingAccounts(client *http.Client, apiURL, apiKey string) (map[string]*ExistingAccount, error) {
	existing := make(map[string]*ExistingAccount)
	page := 1

	for {
		url := fmt.Sprintf("%s/api/v1/admin/accounts?platform=anthropic&type=oauth&page=%d&page_size=100", apiURL, page)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-api-key", apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var apiResp APIResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return nil, err
		}
		if apiResp.Code != 0 {
			return nil, fmt.Errorf("%s", apiResp.Message)
		}

		var paginated PaginatedData
		if err := json.Unmarshal(apiResp.Data, &paginated); err != nil {
			return nil, err
		}

		var accounts []ExistingAccount
		if err := json.Unmarshal(paginated.Items, &accounts); err != nil {
			return nil, err
		}

		for i := range accounts {
			existing[accounts[i].Name] = &accounts[i]
		}

		if page >= paginated.Pages {
			break
		}
		page++
	}

	return existing, nil
}

// fetchProxies 获取所有代理
func fetchProxies(client *http.Client, apiURL, apiKey string) ([]Proxy, error) {
	url := apiURL + "/api/v1/admin/proxies/all"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("%s", apiResp.Message)
	}

	var proxies []Proxy
	if err := json.Unmarshal(apiResp.Data, &proxies); err != nil {
		return nil, err
	}
	return proxies, nil
}

// fetchGroups 获取所有分组
func fetchGroups(client *http.Client, apiURL, apiKey string) ([]Group, error) {
	url := apiURL + "/api/v1/admin/groups/all"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("%s", apiResp.Message)
	}

	var groups []Group
	if err := json.Unmarshal(apiResp.Data, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// cookieAuth 使用 session_key 换取 token
func cookieAuth(client *http.Client, apiURL, apiKey, sessionKey string, proxyID *int64) (*TokenInfo, error) {
	url := apiURL + "/api/v1/admin/accounts/cookie-auth"
	reqBody := map[string]any{"code": sessionKey}
	if proxyID != nil {
		reqBody["proxy_id"] = *proxyID
	}
	bodyBytes, _ := json.Marshal(reqBody)
	body := string(bodyBytes)

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("%s", apiResp.Message)
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

// createAccount 创建 Claude OAuth 账号
func createAccount(client *http.Client, apiURL, apiKey, email string, token *TokenInfo, proxyID *int64, groupIDs []int64) (int64, error) {
	url := apiURL + "/api/v1/admin/accounts"

	// credentials 直接使用 cookie-auth 返回的完整 token（与 Web 前端一致）
	credentials := token.Raw

	// extra 中放入身份信息（与 Web 前端 buildExtraInfo 一致）
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

	reqBody := map[string]any{
		"name":        email,
		"platform":    "anthropic",
		"type":        "oauth",
		"credentials": credentials,
		"concurrency": 10,
	}
	if len(extra) > 0 {
		reqBody["extra"] = extra
	}
	if proxyID != nil {
		reqBody["proxy_id"] = *proxyID
	}
	if len(groupIDs) > 0 {
		reqBody["group_ids"] = groupIDs
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return 0, fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return 0, fmt.Errorf("%s", apiResp.Message)
	}

	var account CreatedAccount
	if err := json.Unmarshal(apiResp.Data, &account); err != nil {
		return 0, fmt.Errorf("解析账号失败: %w", err)
	}
	return account.ID, nil
}

// updateAccount 更新已有账号的代理、分组和容量
func updateAccount(client *http.Client, apiURL, apiKey string, accountID int64, proxyID *int64, groupIDs []int64, concurrency *int) error {
	url := fmt.Sprintf("%s/api/v1/admin/accounts/%d", apiURL, accountID)

	reqBody := map[string]any{
		"confirm_mixed_channel_risk": true,
	}
	if proxyID != nil {
		reqBody["proxy_id"] = *proxyID
	}
	if groupIDs != nil {
		reqBody["group_ids"] = groupIDs
	}
	if concurrency != nil {
		reqBody["concurrency"] = *concurrency
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("PUT", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return fmt.Errorf("%s", apiResp.Message)
	}
	return nil
}

// disableAccount 禁用账号并记录失败原因
func disableAccount(client *http.Client, apiURL, apiKey string, accountID int64, reason string) error {
	url := fmt.Sprintf("%s/api/v1/admin/accounts/%d", apiURL, accountID)

	reqBody := map[string]any{
		"status": "inactive",
		"notes":  "测试失败: " + reason,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("PUT", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return fmt.Errorf("%s", apiResp.Message)
	}
	return nil
}

// testAccount 测试账号连接
func testAccount(client *http.Client, apiURL, apiKey string, accountID int64, model string) error {
	url := fmt.Sprintf("%s/api/v1/admin/accounts/%d/test", apiURL, accountID)

	body := "{}"
	if model != "" {
		body = fmt.Sprintf(`{"model_id":"%s"}`, model)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

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
				return nil
			}
		case "error":
			lastError = event.Error
		}
	}

	if gotContent {
		return nil
	}
	if lastError != "" {
		return fmt.Errorf("%s", lastError)
	}
	return fmt.Errorf("未收到有效响应")
}

// printResults 打印汇总
func printResults(results []AddResult) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("批量添加结果")
	fmt.Println(strings.Repeat("=", 80))

	success, updated, skipped, authFail, addFail, testFail, updateFail := 0, 0, 0, 0, 0, 0, 0
	for _, r := range results {
		icon := "✗"
		switch r.Status {
		case StatusSuccess:
			icon = "✓"
		case StatusUpdated:
			icon = "↑"
		case StatusSkipped:
			icon = "-"
		}
		fmt.Printf("  %s [%s] %s (ID: %d) %s\n", icon, r.Status, r.Email, r.AccountID, r.Detail)

		switch r.Status {
		case StatusSuccess:
			success++
		case StatusUpdated:
			updated++
		case StatusSkipped:
			skipped++
		case StatusAuthFail:
			authFail++
		case StatusAddFail:
			addFail++
		case StatusTestFail:
			testFail++
		case StatusUpdateFail:
			updateFail++
		}
	}

	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("总计: %d | 成功: %d | 已更新: %d | 跳过: %d | 认证失败: %d | 创建失败: %d | 测试失败: %d | 更新失败: %d\n",
		len(results), success, updated, skipped, authFail, addFail, testFail, updateFail)
}

// exportResults 导出结果到 txt
func exportResults(results []AddResult, filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "批量添加结果 - %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(f, "%s\n\n", strings.Repeat("=", 60))

	for _, r := range results {
		fmt.Fprintf(f, "[%s] %s | ID: %d | %s\n", r.Status, r.Email, r.AccountID, r.Detail)
	}

	success, fail := 0, 0
	for _, r := range results {
		if r.Status == StatusSuccess {
			success++
		} else {
			fail++
		}
	}

	fmt.Fprintf(f, "\n%s\n", strings.Repeat("-", 60))
	fmt.Fprintf(f, "总计: %d | 成功: %d | 失败: %d\n", len(results), success, fail)
	return nil
}

// loadEnvFile 从 .env 文件加载配置
func loadEnvFile() {
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

// getConfig 按优先级获取配置
func getConfig(flagVal, envKey, defaultVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return defaultVal
}
