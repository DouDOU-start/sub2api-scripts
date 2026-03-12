package main

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"sub2api-scripts/internal/api"
)

func runProxyRebalance(client *api.Client, _ string) {
	printHeader("代理均衡分配")

	// 输入每条代理的上限
	limitStr, err := inputText("每条代理最多绑定的账号数", "如 10", "")
	if err != nil || limitStr == "" {
		fmt.Println("已取消")
		return
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		fmt.Printf("%s 无效的数字: %s\n", failIcon, limitStr)
		return
	}

	// 获取所有代理（不过滤状态）
	fmt.Printf("\n%s 正在获取代理列表...\n", infoIcon)
	proxies, err := client.FetchProxiesPaginated("")
	if err != nil {
		fmt.Printf("%s 获取代理列表失败: %v\n", failIcon, err)
		return
	}
	if len(proxies) == 0 {
		fmt.Printf("%s 没有可用的代理\n", warnIcon)
		return
	}

	proxyNameMap := make(map[int64]string, len(proxies))
	activeProxies := make(map[int64]bool)
	for _, p := range proxies {
		proxyNameMap[p.ID] = p.Name
		if p.Status == "active" {
			activeProxies[p.ID] = true
		}
	}

	// 获取所有账号
	fmt.Printf("%s 正在获取账号列表...\n", infoIcon)
	accounts, err := client.FetchAccounts(api.AccountListOptions{
		Platform: "anthropic",
		Type:     "oauth",
	})
	if err != nil {
		fmt.Printf("%s 获取账号列表失败: %v\n", failIcon, err)
		return
	}
	fmt.Printf("%s 共 %d 个账号\n\n", infoIcon, len(accounts))

	// 按代理分组
	proxyAccounts := make(map[int64][]api.Account)
	var noProxy []api.Account
	for _, acc := range accounts {
		if acc.ProxyID != nil {
			proxyAccounts[*acc.ProxyID] = append(proxyAccounts[*acc.ProxyID], acc)
		} else {
			noProxy = append(noProxy, acc)
		}
	}

	// 显示当前状态 & 收集需要处理的账号
	fmt.Printf("当前代理绑定情况（上限: %d）:\n", limit)
	printSeparator(70)

	var overflow []api.Account
	slotRemaining := make(map[int64]int)

	for _, p := range proxies {
		accs := proxyAccounts[p.ID]
		count := len(accs)
		isActive := activeProxies[p.ID]

		var status string
		if !isActive {
			status = fmt.Sprintf("停用，需迁出 %d", count)
			overflow = append(overflow, accs...)
		} else if count > limit {
			status = fmt.Sprintf("超出 %d", count-limit)
			overflow = append(overflow, accs[limit:]...)
		} else if count == limit {
			status = "已满"
		} else {
			status = fmt.Sprintf("剩余 %d", limit-count)
			slotRemaining[p.ID] = limit - count
		}

		stateTag := "启用"
		if !isActive {
			stateTag = "停用"
		}
		fmt.Printf("  [ID:%d] %-25s [%s] %3d 个账号  %s\n", p.ID, p.Name, stateTag, count, status)
	}
	if len(noProxy) > 0 {
		fmt.Printf("  [无代理] %d 个账号\n", len(noProxy))
	}
	fmt.Println()

	if len(overflow) == 0 {
		fmt.Printf("%s 所有代理均正常，无需调整\n", successIcon)
		return
	}

	// 分类：异常解绑，正常迁移
	var unbindAccounts, migrateAccounts []api.Account
	for _, acc := range overflow {
		if acc.Status != "active" {
			unbindAccounts = append(unbindAccounts, acc)
		} else {
			migrateAccounts = append(migrateAccounts, acc)
		}
	}

	// 测试代理连通性
	fmt.Printf("%s 正在测试代理连通性...\n", infoIcon)
	var reachableProxies []int64
	for _, p := range proxies {
		if !activeProxies[p.ID] {
			continue
		}
		if _, ok := slotRemaining[p.ID]; !ok {
			continue
		}
		fmt.Printf("  [ID:%d] %s ...", p.ID, p.Name)
		result, err := client.TestProxy(p.ID)
		if err != nil {
			fmt.Printf(" %s %v（已排除）\n", failIcon, err)
			delete(slotRemaining, p.ID)
			continue
		}
		if !result.Success {
			fmt.Printf(" %s %s（已排除）\n", failIcon, result.Message)
			delete(slotRemaining, p.ID)
			continue
		}
		fmt.Printf(" %s (%dms, %s)\n", successIcon, result.LatencyMs, result.IPAddress)
		reachableProxies = append(reachableProxies, p.ID)
	}
	fmt.Println()

	// 构建可用代理列表
	var availableSlots []int64
	for _, pid := range reachableProxies {
		if rem, ok := slotRemaining[pid]; ok && rem > 0 {
			availableSlots = append(availableSlots, pid)
		}
	}
	sort.Slice(availableSlots, func(i, j int) bool {
		return slotRemaining[availableSlots[i]] > slotRemaining[availableSlots[j]]
	})

	totalRemaining := 0
	for _, pid := range availableSlots {
		totalRemaining += slotRemaining[pid]
	}

	fmt.Printf("需要处理 %d 个账号:\n", len(overflow))
	fmt.Printf("  异常账号（直接解绑代理）: %d 个\n", len(unbindAccounts))
	fmt.Printf("  正常账号（迁移到其他代理）: %d 个\n", len(migrateAccounts))
	fmt.Printf("  启用代理剩余容量: %d\n", totalRemaining)

	if totalRemaining < len(migrateAccounts) {
		fmt.Printf("\n%s 剩余容量不足，需要迁移 %d 个正常账号，但只有 %d 个空位\n",
			failIcon, len(migrateAccounts), totalRemaining)
		for _, pid := range availableSlots {
			fmt.Printf("  [ID:%d] %s: 剩余 %d\n", pid, proxyNameMap[pid], slotRemaining[pid])
		}
		return
	}

	// 生成迁移计划
	type migration struct {
		account   api.Account
		fromProxy int64
		toProxy   int64
	}
	var plan []migration
	slotIdx := 0
	for _, acc := range migrateAccounts {
		for slotRemaining[availableSlots[slotIdx]] <= 0 {
			slotIdx++
		}
		target := availableSlots[slotIdx]
		plan = append(plan, migration{account: acc, fromProxy: *acc.ProxyID, toProxy: target})
		slotRemaining[target]--
	}

	// 显示计划
	if len(unbindAccounts) > 0 {
		fmt.Printf("\n解绑代理（异常账号，共 %d 个）:\n", len(unbindAccounts))
		printSeparator(80)
		for i, acc := range unbindAccounts {
			fmt.Printf("  %d. %-40s [%s] 从 %s 解绑\n", i+1, acc.Name, acc.Status, proxyNameMap[*acc.ProxyID])
		}
	}
	if len(plan) > 0 {
		fmt.Printf("\n迁移计划（正常账号，共 %d 个）:\n", len(plan))
		printSeparator(80)
		for i, m := range plan {
			fmt.Printf("  %d. %-40s %s -> %s\n", i+1, m.account.Name,
				proxyNameMap[m.fromProxy], proxyNameMap[m.toProxy])
		}
	}
	fmt.Println()

	if !confirm("确认执行以上操作？") {
		fmt.Println("已取消")
		return
	}

	// 执行解绑
	unbindOK, unbindFail := 0, 0
	if len(unbindAccounts) > 0 {
		fmt.Println("\n开始解绑异常账号...")
		for i, acc := range unbindAccounts {
			fmt.Printf("[%d/%d] %s 解绑代理...", i+1, len(unbindAccounts), acc.Name)
			err := client.UnbindProxy(acc.ID)
			if err != nil {
				fmt.Printf(" %s %v\n", failIcon, err)
				unbindFail++
			} else {
				fmt.Printf(" %s\n", successIcon)
				unbindOK++
			}
			if i < len(unbindAccounts)-1 {
				time.Sleep(200 * time.Millisecond)
			}
		}
		fmt.Printf("解绑完成: 成功 %d，失败 %d\n\n", unbindOK, unbindFail)
	}

	// 执行迁移
	migrateOK, migrateFail := 0, 0
	if len(plan) > 0 {
		fmt.Println("开始迁移正常账号...")
		for i, m := range plan {
			fmt.Printf("[%d/%d] %s -> %s ...", i+1, len(plan), m.account.Name, proxyNameMap[m.toProxy])
			err := client.UpdateAccount(m.account.ID, api.UpdateAccountRequest{
				ProxyID:                &m.toProxy,
				ConfirmMixedChannelRisk: true,
			})
			if err != nil {
				fmt.Printf(" %s %v\n", failIcon, err)
				migrateFail++
			} else {
				fmt.Printf(" %s\n", successIcon)
				migrateOK++
			}
			if i < len(plan)-1 {
				time.Sleep(200 * time.Millisecond)
			}
		}
		fmt.Printf("迁移完成: 成功 %d，失败 %d\n\n", migrateOK, migrateFail)
	}

	fmt.Printf("%s 全部完成: 解绑 %d/%d，迁移 %d/%d\n",
		successIcon, unbindOK, len(unbindAccounts), migrateOK, len(plan))
}
