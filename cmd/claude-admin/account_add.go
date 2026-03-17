package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/huh"

	"sub2api-scripts/internal/api"
)

// 添加结果状态
type addStatus string

const (
	addSuccess    addStatus = "成功"
	addUpdated    addStatus = "已更新"
	addAuthFail   addStatus = "认证失败"
	addCreateFail addStatus = "创建失败"
	addTestFail   addStatus = "测试失败"
	addUpdateFail addStatus = "更新失败"
)

type addResult struct {
	email     string
	status    addStatus
	detail    string
	accountID int64
}

// 平台和账号类型配置
type platformConfig struct {
	platform    string
	accountType string
	label       string
}

var platformConfigs = []platformConfig{
	{"anthropic", "oauth", "Anthropic (OAuth)"},
	{"anthropic", "setup-token", "Anthropic (Setup Token)"},
	{"openai", "oauth", "OpenAI (OAuth)"},
}

func runAccountAdd(client *api.Client, model string) {
	printHeader("批量添加账号")

	// 选择平台和账号类型
	platformOpts := make([]string, len(platformConfigs))
	for i, c := range platformConfigs {
		platformOpts[i] = c.label
	}
	platformIdx, err := selectOne("选择平台和账号类型", platformOpts)
	if err != nil {
		fmt.Println("已取消")
		return
	}
	pcfg := platformConfigs[platformIdx]
	fmt.Printf("%s 平台: %s, 类型: %s\n", infoIcon, pcfg.platform, pcfg.accountType)

	// 选择输入方式
	inputMode, err := selectOne("账号数据来源", []string{
		"从文件读取",
		"手动输入（每行一个，空行结束）",
	})
	if err != nil {
		fmt.Println("已取消")
		return
	}

	var lines []string
	if inputMode == 0 {
		// 扫描 data/ 目录下的 .txt 文件
		entries, dirErr := os.ReadDir("data")
		if dirErr != nil {
			fmt.Printf("%s 读取 data 目录失败: %v\n", failIcon, dirErr)
			return
		}
		var files []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".txt") {
				files = append(files, e.Name())
			}
		}
		if len(files) == 0 {
			fmt.Printf("%s data 目录下没有 .txt 文件\n", warnIcon)
			return
		}
		fileIdx, err := selectOne("选择账号文件", files)
		if err != nil {
			fmt.Println("已取消")
			return
		}
		filePath := "data/" + files[fileIdx]
		lines = readLinesFromFile(filePath)
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
		fmt.Printf("%s 未读取到任何账号\n", warnIcon)
		return
	}
	fmt.Printf("\n%s 共读取 %d 个账号\n", infoIcon, len(lines))

	// 代理选择
	var selectedProxy *int64
	var proxyAssign []*int64
	var proxyNameMap map[int64]string
	shareSize := 0
	var cachedProxies []api.Proxy

	fetchProxies := func() ([]api.Proxy, error) {
		if cachedProxies != nil {
			return cachedProxies, nil
		}
		proxies, err := client.FetchProxies()
		if err != nil {
			return nil, err
		}
		cachedProxies = proxies
		return proxies, nil
	}

	proxyMode, err := selectOne("代理绑定方式", []string{
		"不绑定代理",
		"选择单个代理（所有账号共用）",
		"自动分配（每 N 个账号共用一条代理）",
	})
	if err != nil {
		fmt.Println("已取消")
		return
	}

	switch proxyMode {
	case 1: // 单代理
		proxies, err := fetchProxies()
		if err != nil {
			fmt.Printf("%s 获取代理列表失败: %v\n", failIcon, err)
			return
		}
		if len(proxies) == 0 {
			fmt.Printf("%s 没有可用的代理\n", warnIcon)
			return
		}
		opts := make([]string, len(proxies))
		for i, p := range proxies {
			opts[i] = fmt.Sprintf("[ID:%d] %s (%s:%d)", p.ID, p.Name, p.Host, p.Port)
		}
		idx, err := selectOne("选择代理", opts)
		if err != nil {
			fmt.Println("已取消")
			return
		}
		id := proxies[idx].ID
		selectedProxy = &id
		fmt.Printf("%s 已选择代理: %s (ID: %d)\n", successIcon, proxies[idx].Name, id)

	case 2: // 自动分配
		shareStr, err := inputText("每条代理共享几个账号", "如 10", "")
		if err != nil || shareStr == "" {
			fmt.Println("已取消")
			return
		}
		shareSize, err = strconv.Atoi(shareStr)
		if err != nil || shareSize <= 0 {
			fmt.Printf("%s 无效的数字\n", failIcon)
			return
		}
	}

	// 分组选择
	var selectedGroupIDs []int64
	groups, err := client.FetchGroups()
	if err != nil {
		fmt.Printf("%s 获取分组列表失败: %v，将不绑定分组\n", warnIcon, err)
	} else if len(groups) > 0 {
		opts := make([]huh.Option[int], len(groups))
		// 默认预选当前平台的分组
		var preselected []int
		for i, g := range groups {
			opts[i] = huh.NewOption(fmt.Sprintf("[ID:%d] %s (%s)", g.ID, g.Name, g.Platform), i)
			if g.Platform == pcfg.platform {
				preselected = append(preselected, i)
			}
		}
		selected := preselected
		err = huh.NewMultiSelect[int]().
			Title("选择分组（空格切换选中，回车确认）").
			Options(opts...).
			Value(&selected).
			Run()
		if err == nil {
			for _, idx := range selected {
				selectedGroupIDs = append(selectedGroupIDs, groups[idx].ID)
			}
		}
	}

	// 配额限速配置（仅 Anthropic OAuth/SetupToken 账号）
	quota := api.DefaultQuotaConfig()
	if pcfg.platform == "anthropic" {
		quota = promptQuotaConfig()
	}

	// 并发量设置
	concurrency := 1
	if len(lines) > 1 {
		concurrencyStr, err := inputText("并发量（同时处理的账号数）", fmt.Sprintf("1~%d，默认 1", len(lines)), "1")
		if err != nil {
			fmt.Println("已取消")
			return
		}
		if c, err := strconv.Atoi(concurrencyStr); err == nil && c > 0 {
			if c > len(lines) {
				c = len(lines)
			}
			concurrency = c
		}
		fmt.Printf("%s 并发量: %d\n", infoIcon, concurrency)
	}

	// 获取已有账号
	fmt.Printf("\n%s 正在查询已有账号...\n", infoIcon)
	existingAccounts, err := client.FetchAccountMap(api.AccountListOptions{
		Platform: pcfg.platform,
		Type:     pcfg.accountType,
	})
	if err != nil {
		fmt.Printf("%s 查询已有账号失败: %v，将不跳过重复\n", warnIcon, err)
		existingAccounts = make(map[string]*api.Account)
	} else {
		fmt.Printf("%s 已有 %d 个 %s 账号\n", infoIcon, len(existingAccounts), pcfg.label)
	}

	// 自动分配模式：分配代理
	if shareSize > 0 {
		proxies, err := fetchProxies()
		if err != nil {
			fmt.Printf("%s 获取代理列表失败: %v\n", failIcon, err)
			return
		}

		proxyUsage := make(map[int64]int)
		for _, acc := range existingAccounts {
			if acc.ProxyID != nil {
				proxyUsage[*acc.ProxyID]++
			}
		}

		proxyNameMap = make(map[int64]string, len(proxies))
		for _, p := range proxies {
			proxyNameMap[p.ID] = p.Name
		}

		type proxySlot struct {
			proxy     api.Proxy
			used      int
			remaining int
		}
		var slots []proxySlot
		for _, p := range proxies {
			used := proxyUsage[p.ID]
			remaining := shareSize - used
			if remaining > 0 {
				slots = append(slots, proxySlot{proxy: p, used: used, remaining: remaining})
			}
		}

		// 测试代理连通性
		if confirm("是否测试代理连通性？") {
			fmt.Printf("\n%s 正在测试代理连通性...\n", infoIcon)
			var reachableSlots []proxySlot
			for _, s := range slots {
				fmt.Printf("  [ID:%d] %s ...", s.proxy.ID, s.proxy.Name)
				result, testErr := client.TestProxy(s.proxy.ID)
				if testErr != nil {
					fmt.Printf(" %s %v（已排除）\n", failIcon, testErr)
					continue
				}
				if !result.Success {
					fmt.Printf(" %s %s（已排除）\n", failIcon, result.Message)
					continue
				}
				fmt.Printf(" %s (%dms, %s)\n", successIcon, result.LatencyMs, result.IPAddress)
				reachableSlots = append(reachableSlots, s)
			}
			slots = reachableSlots
			fmt.Println()
		}

		totalRemaining := 0
		for _, s := range slots {
			totalRemaining += s.remaining
		}

		if totalRemaining < len(lines) {
			fmt.Printf("%s 代理剩余容量不足，需要分配 %d 个账号，但总剩余容量仅 %d（每条上限 %d）\n",
				failIcon, len(lines), totalRemaining, shareSize)
			return
		}

		proxyAssign = make([]*int64, len(lines))
		slotIdx := 0
		assigned := 0
		fmt.Printf("代理自动分配（每条上限 %d 个账号）:\n", shareSize)
		for slotIdx < len(slots) && assigned < len(lines) {
			s := &slots[slotIdx]
			count := s.remaining
			if assigned+count > len(lines) {
				count = len(lines) - assigned
			}
			id := s.proxy.ID
			for j := 0; j < count; j++ {
				proxyAssign[assigned+j] = &id
			}
			fmt.Printf("  [ID:%d] %s: 已有 %d，本次分配 %d（账号 %d~%d）\n",
				s.proxy.ID, s.proxy.Name, s.used, count, assigned+1, assigned+count)
			assigned += count
			slotIdx++
		}
	}

	getProxy := func(index int) *int64 {
		if proxyAssign != nil {
			return proxyAssign[index]
		}
		return selectedProxy
	}

	getProxyName := func(index int) string {
		if proxyAssign != nil && proxyAssign[index] != nil && proxyNameMap != nil {
			return proxyNameMap[*proxyAssign[index]]
		}
		return ""
	}

	fmt.Printf("\n开始处理（并发: %d）...\n\n", concurrency)

	// 按原始顺序存放结果
	results := make([]addResult, len(lines))

	type task struct {
		index int
		line  string
	}
	taskCh := make(chan task, len(lines))
	for i, line := range lines {
		taskCh <- task{index: i, line: line}
	}
	close(taskCh)

	var mu sync.Mutex // 保护终端输出
	var wg sync.WaitGroup
	total := len(lines)

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range taskCh {
				i, line := t.index, t.line
				result := processOneAccount(client, i, total, line, getProxy(i), getProxyName(i), existingAccounts, selectedGroupIDs, pcfg, model, quota.ForIndex(i), &mu)
				results[i] = result
			}
		}()
	}

	wg.Wait()

	// 输出汇总
	printAddResults(results)
}

// processOneAccount 处理单个账号的添加流程（线程安全）
func processOneAccount(
	client *api.Client, i, total int, line string,
	currentProxy *int64, proxyName string,
	existingAccounts map[string]*api.Account,
	selectedGroupIDs []int64, pcfg platformConfig, model string,
	quota api.QuotaConfig, mu *sync.Mutex,
) addResult {
	parts := strings.SplitN(line, "----", 3)
	if len(parts) != 3 {
		mu.Lock()
		fmt.Printf("[%d/%d] %s 格式错误，跳过: %s\n", i+1, total, failIcon, line)
		mu.Unlock()
		return addResult{email: line, status: addCreateFail, detail: "格式错误"}
	}
	email := strings.TrimSpace(parts[0])
	sessionKey := strings.TrimSpace(parts[2])

	prefix := fmt.Sprintf("[%d/%d] %s", i+1, total, email)
	if proxyName != "" {
		prefix += fmt.Sprintf(" (代理: %s)", proxyName)
	}

	// 检查已存在
	if existing, ok := existingAccounts[email]; ok {
		mu.Lock()
		fmt.Printf("%s", prefix)
		result := handleExistingAccount(client, email, existing, currentProxy, selectedGroupIDs, quota)
		mu.Unlock()
		return result
	}

	// 认证
	mu.Lock()
	fmt.Printf("%s\n  认证中...", prefix)
	mu.Unlock()

	tokenInfo, err := client.CookieAuth(sessionKey, currentProxy, pcfg.accountType)
	if err != nil {
		mu.Lock()
		fmt.Printf(" %s %v\n", failIcon, err)
		mu.Unlock()
		return addResult{email: email, status: addAuthFail, detail: err.Error()}
	}
	mu.Lock()
	fmt.Printf(" %s\n", successIcon)
	mu.Unlock()

	// 创建
	mu.Lock()
	fmt.Printf("  [%s] 创建账号...", email)
	mu.Unlock()

	req := api.BuildCreateRequest(email, tokenInfo, currentProxy, selectedGroupIDs, pcfg.platform, pcfg.accountType, quota)
	accountID, err := client.CreateAccount(req)
	if err != nil {
		mu.Lock()
		fmt.Printf(" %s %v\n", failIcon, err)
		mu.Unlock()
		return addResult{email: email, status: addCreateFail, detail: err.Error()}
	}
	mu.Lock()
	fmt.Printf(" %s (ID: %d)\n", successIcon, accountID)
	mu.Unlock()

	// 测试
	mu.Lock()
	fmt.Printf("  [%s] 测试连接...", email)
	mu.Unlock()

	testErr := client.TestAccount(accountID, model)
	if testErr != nil {
		mu.Lock()
		fmt.Printf(" %s %v\n", failIcon, testErr)
		fmt.Printf("  [%s] 关闭调度...", email)
		if disableErr := client.DisableSchedule(accountID, testErr.Error()); disableErr != nil {
			fmt.Printf(" %s %v\n", failIcon, disableErr)
		} else {
			fmt.Printf(" %s\n", successIcon)
		}
		mu.Unlock()
		return addResult{email: email, status: addTestFail, detail: testErr.Error(), accountID: accountID}
	}

	mu.Lock()
	fmt.Printf(" %s\n", successIcon)
	mu.Unlock()
	return addResult{email: email, status: addSuccess, detail: "-", accountID: accountID}
}

func handleExistingAccount(client *api.Client, email string, existing *api.Account, selectedProxy *int64, selectedGroupIDs []int64, quota api.QuotaConfig) addResult {
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

	p := 1
	req.Priority = &p
	req.Extra = api.BuildQuotaExtra(quota)
	if quota.RateMultiplier > 0 && quota.RateMultiplier != 1.0 {
		rm := quota.RateMultiplier
		req.RateMultiplier = &rm
	}
	if quota.LoadFactor > 0 {
		lf := quota.LoadFactor
		req.LoadFactor = &lf
	}
	updates = append(updates, "配置+配额")

	desc := strings.Join(updates, "+")
	fmt.Printf(" -> 已存在，同步%s...", desc)
	if err := client.UpdateAccount(existing.ID, req); err != nil {
		fmt.Printf(" %s %v\n", failIcon, err)
		return addResult{email: email, status: addUpdateFail, detail: err.Error(), accountID: existing.ID}
	}
	fmt.Printf(" %s\n", successIcon)
	return addResult{email: email, status: addUpdated, detail: "同步" + desc, accountID: existing.ID}
}

func readLinesFromFile(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("%s 打开文件失败: %v\n", failIcon, err)
		return nil
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	fmt.Printf("%s 从文件 %s 读取到 %d 个账号\n", successIcon, path, len(lines))
	return lines
}

// promptQuotaConfig 交互式配置配额限速参数
func promptQuotaConfig() api.QuotaConfig {
	quota := api.DefaultQuotaConfig()

	fmt.Printf("\n%s 配额限速配置（基准值: RPM=%d, 最大会话=%d, 超时=%d分钟, 5h费用=$%.0f）\n",
		infoIcon, quota.BaseRPM, quota.MaxSessions, quota.SessionIdleTimeoutMinutes, quota.WindowCostLimit)

	mode, err := selectOne("配额配置方式", []string{
		fmt.Sprintf("使用默认值（RPM=%d, 会话=%d, 5h=$%.0f）", quota.BaseRPM, quota.MaxSessions, quota.WindowCostLimit),
		"自定义配置",
		"不设置配额限制",
	})
	if err != nil || mode == 0 {
		quota = promptQuotaPercentages(quota)
		return quota
	}
	if mode == 2 {
		fmt.Printf("%s 不设置配额限制\n", infoIcon)
		return api.QuotaConfig{}
	}

	// 自定义配置
	if v, err := inputText("RPM 基准值（每分钟请求数，0=不限）", "如 60", strconv.Itoa(quota.BaseRPM)); err == nil {
		if n, e := strconv.Atoi(v); e == nil && n >= 0 {
			quota.BaseRPM = n
		}
	}

	if v, err := inputText("最大并发会话基准值（0=不限）", "如 10", strconv.Itoa(quota.MaxSessions)); err == nil {
		if n, e := strconv.Atoi(v); e == nil && n >= 0 {
			quota.MaxSessions = n
		}
	}

	if quota.MaxSessions > 0 {
		if v, err := inputText("会话空闲超时（分钟）", "如 5", strconv.Itoa(quota.SessionIdleTimeoutMinutes)); err == nil {
			if n, e := strconv.Atoi(v); e == nil && n > 0 {
				quota.SessionIdleTimeoutMinutes = n
			}
		}
	}

	if v, err := inputText("5h 窗口费用基准值（美元，0=不限）", "如 80", fmt.Sprintf("%.0f", quota.WindowCostLimit)); err == nil {
		if f, e := strconv.ParseFloat(v, 64); e == nil && f >= 0 {
			quota.WindowCostLimit = f
		}
	}

	if v, err := inputText("计费倍率（默认 1.0）", "如 1.0", "1.0"); err == nil {
		if f, e := strconv.ParseFloat(v, 64); e == nil && f > 0 {
			quota.RateMultiplier = f
		}
	}

	quota = promptQuotaPercentages(quota)
	return quota
}

// promptQuotaPercentages 配置百分比浮动
func promptQuotaPercentages(quota api.QuotaConfig) api.QuotaConfig {
	fmt.Printf("\n%s 百分比浮动：输入多个百分比值，每个账号按顺序循环应用\n", infoIcon)
	fmt.Printf("  例: 输入 \"90 80 120\" → 账号1=基准×90%%, 账号2=基准×80%%, 账号3=基准×120%%, 账号4=基准×90%%...\n")
	fmt.Printf("  留空或输入 100 = 所有账号使用相同基准值\n")

	v, err := inputText("百分比列表（空格分隔，留空=不浮动）", "如 90 80 120", "")
	if err != nil || strings.TrimSpace(v) == "" {
		printQuotaSummary(quota)
		return quota
	}

	parts := strings.Fields(v)
	var pcts []int
	for _, p := range parts {
		n, e := strconv.Atoi(p)
		if e != nil || n <= 0 {
			fmt.Printf("%s 忽略无效百分比: %s\n", warnIcon, p)
			continue
		}
		pcts = append(pcts, n)
	}

	if len(pcts) > 0 {
		// 检查是否全是 100，等于没浮动
		allSame := true
		for _, p := range pcts {
			if p != 100 {
				allSame = false
				break
			}
		}
		if !allSame {
			quota.Percentages = pcts
			fmt.Printf("%s 百分比浮动: %v（共 %d 档循环）\n", successIcon, pcts, len(pcts))
			// 展示前几个示例
			count := len(pcts)
			if count > 5 {
				count = 5
			}
			for i := 0; i < count; i++ {
				q := quota.ForIndex(i)
				fmt.Printf("  账号%d: RPM=%d, 会话=%d, 5h=$%.1f (%d%%)\n",
					i+1, q.BaseRPM, q.MaxSessions, q.WindowCostLimit, pcts[i])
			}
			if len(pcts) > 5 {
				fmt.Printf("  ... 共 %d 档循环\n", len(pcts))
			}
			return quota
		}
	}

	printQuotaSummary(quota)
	return quota
}

func printQuotaSummary(quota api.QuotaConfig) {
	fmt.Printf("%s 配额配置: RPM=%d, 会话=%d, 超时=%d分钟, 5h=$%.0f, 倍率=%.1f\n",
		successIcon, quota.BaseRPM, quota.MaxSessions, quota.SessionIdleTimeoutMinutes,
		quota.WindowCostLimit, quota.RateMultiplier)
}

func printAddResults(results []addResult) {
	printSeparator(80)
	fmt.Println(headerStyle.Render("批量添加结果"))
	printSeparator(80)

	counts := map[addStatus]int{}
	for _, r := range results {
		icon := failIcon
		switch r.status {
		case addSuccess:
			icon = successIcon
		case addUpdated:
			icon = infoIcon
		}
		fmt.Printf("  %s [%s] %s (ID: %d) %s\n", icon, r.status, r.email, r.accountID, r.detail)
		counts[r.status]++
	}

	printSeparator(80)
	fmt.Printf("总计: %d | 成功: %d | 已更新: %d | 认证失败: %d | 创建失败: %d | 测试失败: %d | 更新失败: %d\n",
		len(results), counts[addSuccess], counts[addUpdated], counts[addAuthFail],
		counts[addCreateFail], counts[addTestFail], counts[addUpdateFail])
}
