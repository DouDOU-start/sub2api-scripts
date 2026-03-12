package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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
			opts[i] = fmt.Sprintf("[ID:%d] %s (%s)", p.ID, p.Name, p.Address)
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

	fmt.Printf("\n开始处理...\n\n")

	var results []addResult

	for i, line := range lines {
		parts := strings.SplitN(line, "----", 3)
		if len(parts) != 3 {
			fmt.Printf("[%d/%d] %s 格式错误，跳过: %s\n", i+1, len(lines), failIcon, line)
			results = append(results, addResult{email: line, status: addCreateFail, detail: "格式错误"})
			continue
		}
		email := strings.TrimSpace(parts[0])
		sessionKey := strings.TrimSpace(parts[2])

		currentProxy := getProxy(i)

		if proxyName := getProxyName(i); proxyName != "" {
			fmt.Printf("[%d/%d] %s (代理: %s)", i+1, len(lines), email, proxyName)
		} else {
			fmt.Printf("[%d/%d] %s", i+1, len(lines), email)
		}

		// 检查已存在
		if existing, ok := existingAccounts[email]; ok {
			result := handleExistingAccount(client, email, existing, currentProxy, selectedGroupIDs)
			results = append(results, result)
			continue
		}
		fmt.Println()

		// 认证
		fmt.Printf("  认证中...")
		tokenInfo, err := client.CookieAuth(sessionKey, currentProxy, pcfg.accountType)
		if err != nil {
			fmt.Printf(" %s %v\n", failIcon, err)
			results = append(results, addResult{email: email, status: addAuthFail, detail: err.Error()})
			continue
		}
		fmt.Printf(" %s\n", successIcon)

		// 创建
		fmt.Printf("  创建账号...")
		req := api.BuildCreateRequest(email, tokenInfo, currentProxy, selectedGroupIDs, pcfg.platform, pcfg.accountType)
		accountID, err := client.CreateAccount(req)
		if err != nil {
			fmt.Printf(" %s %v\n", failIcon, err)
			results = append(results, addResult{email: email, status: addCreateFail, detail: err.Error()})
			continue
		}
		fmt.Printf(" %s (ID: %d)\n", successIcon, accountID)

		// 测试
		fmt.Printf("  测试连接...")
		testErr := client.TestAccount(accountID, model)
		if testErr != nil {
			fmt.Printf(" %s %v\n", failIcon, testErr)
			fmt.Printf("  关闭调度...")
			if disableErr := client.DisableSchedule(accountID, testErr.Error()); disableErr != nil {
				fmt.Printf(" %s %v\n", failIcon, disableErr)
			} else {
				fmt.Printf(" %s\n", successIcon)
			}
			results = append(results, addResult{email: email, status: addTestFail, detail: testErr.Error(), accountID: accountID})
		} else {
			fmt.Printf(" %s\n", successIcon)
			results = append(results, addResult{email: email, status: addSuccess, detail: "-", accountID: accountID})
		}

		if i < len(lines)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 输出汇总
	printAddResults(results)
}

func handleExistingAccount(client *api.Client, email string, existing *api.Account, selectedProxy *int64, selectedGroupIDs []int64) addResult {
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
