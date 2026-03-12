// Claude OAuth 账号批量添加脚本
// 从文件或 stdin 读取账号信息，批量添加到 sub2api 并验证
// 格式: email----password----session_key（每行一个）
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"sub2api-scripts/internal/api"
	"sub2api-scripts/internal/config"
	"sub2api-scripts/internal/interactive"
)

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

type AddResult struct {
	Email     string
	Status    AddStatus
	Detail    string
	AccountID int64
}

func main() {
	config.LoadEnvFile()

	apiURL := flag.String("api-url", "", "sub2api 服务地址")
	apiKey := flag.String("api-key", "", "管理员 API Key")
	model := flag.String("model", "", "测试用模型（留空使用默认）")
	proxyID := flag.Int64("proxy", -1, "绑定代理 ID（-1=交互选择，0=不绑定）")
	input := flag.String("input", "", "账号文件路径（每行一个，格式: email----password----session_key）")
	output := flag.String("output", "batch-add-result.txt", "结果输出文件")
	flag.Parse()

	finalURL := config.Get(*apiURL, "SUB2API_URL", "http://localhost:8080")
	finalKey := config.Get(*apiKey, "SUB2API_KEY", "")
	finalModel := config.Get(*model, "SUB2API_MODEL", "")

	if finalKey == "" {
		fmt.Fprintln(os.Stderr, "错误: 未找到 API Key，请通过 --api-key、环境变量 SUB2API_KEY 或 .env 文件提供")
		os.Exit(1)
	}

	client := api.NewClient(finalURL, finalKey)

	// 代理选择
	var selectedProxy *int64
	if *proxyID == -1 {
		proxies, err := client.FetchProxies()
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取代理列表失败: %v\n", err)
			os.Exit(1)
		}
		selectedProxy = interactive.SelectProxy(proxies)
	} else if *proxyID > 0 {
		selectedProxy = proxyID
		fmt.Printf("绑定代理 ID: %d\n", *proxyID)
	}

	// 分组选择
	var selectedGroupIDs []int64
	groups, err := client.FetchGroups()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取分组列表失败: %v，将不绑定分组\n", err)
	} else {
		selectedGroupIDs = interactive.SelectGroups(groups)
	}

	// 读取账号
	lines := readAccountLines(*input)
	if len(lines) == 0 {
		fmt.Println("未读取到任何账号")
		return
	}
	fmt.Printf("\n共读取 %d 个账号\n", len(lines))

	// 获取已有账号
	fmt.Println("正在查询已有账号...")
	existingAccounts, err := client.FetchAccountMap(api.AccountListOptions{
		Platform: "anthropic",
		Type:     "oauth",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "查询已有账号失败: %v，将不跳过重复\n", err)
		existingAccounts = make(map[string]*api.Account)
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
		sessionKey := strings.TrimSpace(parts[2])

		fmt.Printf("[%d/%d] %s", i+1, len(lines), email)

		// 检查已存在的账号
		if existing, ok := existingAccounts[email]; ok {
			result := handleExisting(client, email, existing, selectedProxy, selectedGroupIDs)
			results = append(results, result)
			continue
		}
		fmt.Println()

		// 步骤1: cookie-auth 换 token
		fmt.Printf("  认证中...")
		tokenInfo, err := client.CookieAuth(sessionKey, selectedProxy)
		if err != nil {
			fmt.Printf(" 失败: %v\n", err)
			results = append(results, AddResult{Email: email, Status: StatusAuthFail, Detail: err.Error()})
			continue
		}
		fmt.Printf(" OK\n")

		// 步骤2: 创建账号
		fmt.Printf("  创建账号...")
		req := api.BuildCreateRequest(email, tokenInfo, selectedProxy, selectedGroupIDs)
		accountID, err := client.CreateAccount(req)
		if err != nil {
			fmt.Printf(" 失败: %v\n", err)
			results = append(results, AddResult{Email: email, Status: StatusAddFail, Detail: err.Error()})
			continue
		}
		fmt.Printf(" OK (ID: %d)\n", accountID)

		// 步骤3: 测试连接
		fmt.Printf("  测试连接...")
		testErr := client.TestAccount(accountID, finalModel)
		if testErr != nil {
			fmt.Printf(" 失败: %v\n", testErr)
			fmt.Printf("  关闭调度...")
			if disableErr := client.DisableSchedule(accountID, testErr.Error()); disableErr != nil {
				fmt.Printf(" 失败: %v\n", disableErr)
			} else {
				fmt.Printf(" OK\n")
			}
			results = append(results, AddResult{Email: email, Status: StatusTestFail, Detail: testErr.Error(), AccountID: accountID})
		} else {
			fmt.Printf(" OK\n")
			results = append(results, AddResult{Email: email, Status: StatusSuccess, Detail: "-", AccountID: accountID})
		}

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

// handleExisting 处理已存在的账号：同步代理、分组、容量、优先级、extra 配置
func handleExisting(client *api.Client, email string, existing *api.Account, selectedProxy *int64, selectedGroupIDs []int64) AddResult {
	var updates []string
	req := api.UpdateAccountRequest{ConfirmMixedChannelRisk: true}

	if selectedProxy != nil && existing.ProxyID == nil {
		req.ProxyID = selectedProxy
		updates = append(updates, "代理")
	}
	if len(selectedGroupIDs) > 0 && len(existing.GroupIDs) == 0 {
		req.GroupIDs = selectedGroupIDs
		updates = append(updates, "分组")
	}
	if existing.Concurrency == 0 {
		c := 10
		req.Concurrency = &c
		updates = append(updates, "容量")
	}

	// 始终同步优先级和 extra 配置
	p := 1
	req.Priority = &p
	req.Extra = map[string]any{
		"enable_tls_fingerprint":     true,
		"session_id_masking_enabled": true,
		"cache_ttl_override_enabled": true,
		"cache_ttl_override_target":  "5m",
	}
	updates = append(updates, "配置")

	desc := strings.Join(updates, "+")
	fmt.Printf(" -> 已存在，同步%s...", desc)
	if err := client.UpdateAccount(existing.ID, req); err != nil {
		fmt.Printf(" 失败: %v\n", err)
		return AddResult{Email: email, Status: StatusUpdateFail, Detail: err.Error(), AccountID: existing.ID}
	}
	fmt.Printf(" OK\n")
	return AddResult{Email: email, Status: StatusUpdated, Detail: "同步" + desc, AccountID: existing.ID}
}

// readAccountLines 从文件或 stdin 读取账号行
func readAccountLines(inputFile string) []string {
	var lines []string
	if inputFile != "" {
		f, err := os.Open(inputFile)
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
		fmt.Printf("从文件 %s 读取到 %d 个账号\n", inputFile, len(lines))
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
	return lines
}

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
