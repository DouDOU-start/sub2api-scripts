package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"sub2api-scripts/internal/api"
)

// 重授权结果状态
type reauthStatus string

const (
	reauthSuccess   reauthStatus = "成功"
	reauthAuthFail  reauthStatus = "认证失败"
	reauthUpdFail   reauthStatus = "更新失败"
	reauthTestFail  reauthStatus = "测试失败"
	reauthSkipped   reauthStatus = "跳过"
	reauthNotFound  reauthStatus = "未匹配"
)

type reauthResult struct {
	email     string
	status    reauthStatus
	detail    string
	accountID int64
}

func runAccountReauth(client *api.Client, model string) {
	printHeader("批量重新授权（更新已有账号 Token）")

	// 选择账号类型
	platformOpts := []string{"Anthropic (OAuth)", "Anthropic (Setup Token)"}
	platformIdx, err := selectOne("选择账号类型", platformOpts)
	if err != nil {
		fmt.Println("已取消")
		return
	}
	accountType := "oauth"
	if platformIdx == 1 {
		accountType = "setup-token"
	}
	fmt.Printf("%s 类型: %s\n", infoIcon, accountType)

	// 选择重授权范围
	scopeMode, err := selectOne("选择重授权范围", []string{
		"仅错误状态的账号（自动匹配 SK 文件）",
		"所有账号（自动匹配 SK 文件）",
		"指定账号 ID 列表",
	})
	if err != nil {
		fmt.Println("已取消")
		return
	}

	// 获取已有账号
	fmt.Printf("\n%s 正在获取账号列表...\n", infoIcon)
	accounts, err := client.FetchAccounts(api.AccountListOptions{
		Platform: "anthropic",
		Type:     accountType,
	})
	if err != nil {
		fmt.Printf("%s 获取账号列表失败: %v\n", failIcon, err)
		return
	}
	fmt.Printf("%s 共 %d 个账号\n", infoIcon, len(accounts))

	// 构建 email -> account 映射
	accountByEmail := make(map[string]*api.Account, len(accounts))
	accountByID := make(map[int64]*api.Account, len(accounts))
	for i := range accounts {
		accountByEmail[accounts[i].Name] = &accounts[i]
		accountByID[accounts[i].ID] = &accounts[i]
	}

	// 读取 SK 数据文件（email----password----session_key 格式）
	skMap, err := loadSKFile()
	if err != nil {
		fmt.Printf("%s %v\n", failIcon, err)
		return
	}
	fmt.Printf("%s SK 文件包含 %d 个账号\n", infoIcon, len(skMap))

	// 根据范围确定目标账号
	var targets []*api.Account
	switch scopeMode {
	case 0: // 仅错误状态
		for i := range accounts {
			if accounts[i].Status == "error" {
				targets = append(targets, &accounts[i])
			}
		}
	case 1: // 所有账号
		for i := range accounts {
			targets = append(targets, &accounts[i])
		}
	case 2: // 指定 ID
		idsStr, err := inputText("输入账号 ID（逗号分隔）", "如 1,2,3", "")
		if err != nil || idsStr == "" {
			fmt.Println("已取消")
			return
		}
		for _, s := range strings.Split(idsStr, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if err != nil {
				continue
			}
			if acc, ok := accountByID[id]; ok {
				targets = append(targets, acc)
			} else {
				fmt.Printf("%s ID %d 不存在，已跳过\n", warnIcon, id)
			}
		}
	}

	// 过滤出有 SK 的目标
	type reauthTask struct {
		account    *api.Account
		sessionKey string
	}
	var tasks []reauthTask
	var noSK []string

	for _, acc := range targets {
		if sk, ok := skMap[acc.Name]; ok {
			tasks = append(tasks, reauthTask{account: acc, sessionKey: sk})
		} else {
			noSK = append(noSK, acc.Name)
		}
	}

	if len(noSK) > 0 {
		fmt.Printf("%s %d 个账号在 SK 文件中未找到匹配，将跳过\n", warnIcon, len(noSK))
	}
	if len(tasks) == 0 {
		fmt.Printf("%s 没有可重授权的账号\n", warnIcon)
		return
	}

	fmt.Printf("\n%s 将对 %d 个账号重新授权\n", infoIcon, len(tasks))
	if !confirm(fmt.Sprintf("开始重授权这 %d 个账号？", len(tasks))) {
		fmt.Println("已取消")
		return
	}

	// 并发量
	concurrency := 1
	if len(tasks) > 1 {
		concurrencyStr, err := inputText("并发量", fmt.Sprintf("1~%d，默认 1", len(tasks)), "1")
		if err != nil {
			fmt.Println("已取消")
			return
		}
		if c, err := strconv.Atoi(concurrencyStr); err == nil && c > 0 {
			if c > len(tasks) {
				c = len(tasks)
			}
			concurrency = c
		}
	}

	fmt.Printf("\n开始处理（并发: %d）...\n\n", concurrency)

	results := make([]reauthResult, len(tasks))
	taskCh := make(chan int, len(tasks))
	for i := range tasks {
		taskCh <- i
	}
	close(taskCh)

	var mu sync.Mutex
	var wg sync.WaitGroup
	total := len(tasks)

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range taskCh {
				t := tasks[idx]
				results[idx] = processReauth(client, idx, total, t.account, t.sessionKey, accountType, model, &mu)
				if idx < total-1 {
					time.Sleep(300 * time.Millisecond)
				}
			}
		}()
	}
	wg.Wait()

	// 汇总
	printReauthResults(results, noSK)
}

// processReauth 处理单个账号的重授权
func processReauth(
	client *api.Client, idx, total int,
	acc *api.Account, sessionKey, accountType, model string,
	mu *sync.Mutex,
) reauthResult {
	prefix := fmt.Sprintf("[%d/%d] [ID:%d] %s", idx+1, total, acc.ID, acc.Name)

	// 1. 用 SK 重新认证获取新 token
	mu.Lock()
	fmt.Printf("%s\n  认证中...", prefix)
	mu.Unlock()

	tokenInfo, err := client.CookieAuth(sessionKey, nil, accountType)
	if err != nil {
		mu.Lock()
		fmt.Printf(" %s %v\n", failIcon, err)
		mu.Unlock()
		return reauthResult{email: acc.Name, status: reauthAuthFail, detail: err.Error(), accountID: acc.ID}
	}
	mu.Lock()
	fmt.Printf(" %s\n", successIcon)
	mu.Unlock()

	// 2. 更新账号 credentials
	mu.Lock()
	fmt.Printf("  更新凭证...")
	mu.Unlock()

	updateReq := api.UpdateAccountRequest{
		Credentials:             tokenInfo.Raw,
		Status:                  "active",
		ConfirmMixedChannelRisk: true,
	}
	// 同步 extra 中的 org_uuid 等字段
	extra := map[string]any{}
	if tokenInfo.OrgUUID != "" {
		extra["org_uuid"] = tokenInfo.OrgUUID
	}
	if tokenInfo.AccountUUID != "" {
		extra["account_uuid"] = tokenInfo.AccountUUID
	}
	if tokenInfo.EmailAddress != "" {
		extra["email_address"] = tokenInfo.EmailAddress
	}
	if len(extra) > 0 {
		updateReq.Extra = extra
	}

	// 恢复调度
	schedulable := true
	updateReq.Schedulable = &schedulable

	if err := client.UpdateAccount(acc.ID, updateReq); err != nil {
		mu.Lock()
		fmt.Printf(" %s %v\n", failIcon, err)
		mu.Unlock()
		return reauthResult{email: acc.Name, status: reauthUpdFail, detail: err.Error(), accountID: acc.ID}
	}
	mu.Lock()
	fmt.Printf(" %s\n", successIcon)
	mu.Unlock()

	// 3. 测试连接
	mu.Lock()
	fmt.Printf("  测试连接...")
	mu.Unlock()

	testErr := client.TestAccount(acc.ID, model)
	if testErr != nil {
		mu.Lock()
		fmt.Printf(" %s %v\n", failIcon, testErr)
		mu.Unlock()
		return reauthResult{email: acc.Name, status: reauthTestFail, detail: testErr.Error(), accountID: acc.ID}
	}
	mu.Lock()
	fmt.Printf(" %s\n", successIcon)
	mu.Unlock()

	return reauthResult{email: acc.Name, status: reauthSuccess, detail: "-", accountID: acc.ID}
}

// loadSKFile 从 data/ 目录读取 SK 文件，返回 email -> session_key 映射
func loadSKFile() (map[string]string, error) {
	entries, err := os.ReadDir("data")
	if err != nil {
		return nil, fmt.Errorf("读取 data 目录失败: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".txt") {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("data 目录下没有 .txt 文件")
	}

	fileIdx, err := selectOne("选择 SK 文件", files)
	if err != nil {
		return nil, fmt.Errorf("已取消")
	}

	f, err := os.Open("data/" + files[fileIdx])
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	skMap := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "----", 3)
		if len(parts) != 3 {
			continue
		}
		email := strings.TrimSpace(parts[0])
		sk := strings.TrimSpace(parts[2])
		skMap[email] = sk
	}

	fmt.Printf("%s 从文件 %s 读取到 %d 个 SK\n", successIcon, files[fileIdx], len(skMap))
	return skMap, nil
}

func printReauthResults(results []reauthResult, noSK []string) {
	printSeparator(80)
	fmt.Println(headerStyle.Render("重授权结果"))
	printSeparator(80)

	counts := map[reauthStatus]int{}
	for _, r := range results {
		icon := failIcon
		switch r.status {
		case reauthSuccess:
			icon = successIcon
		case reauthSkipped:
			icon = infoIcon
		}
		detail := r.detail
		if len(detail) > 50 {
			detail = detail[:50] + "..."
		}
		fmt.Printf("  %s [%s] [ID:%d] %s - %s\n", icon, r.status, r.accountID, r.email, detail)
		counts[r.status]++
	}

	if len(noSK) > 0 {
		fmt.Printf("\n  未匹配 SK 的账号 (%d 个):\n", len(noSK))
		for _, name := range noSK {
			fmt.Printf("    %s %s\n", warnIcon, name)
		}
	}

	printSeparator(80)
	fmt.Printf("总计: %d | 成功: %d | 认证失败: %d | 更新失败: %d | 测试失败: %d | 未匹配: %d\n",
		len(results)+len(noSK), counts[reauthSuccess], counts[reauthAuthFail],
		counts[reauthUpdFail], counts[reauthTestFail], len(noSK))
}
