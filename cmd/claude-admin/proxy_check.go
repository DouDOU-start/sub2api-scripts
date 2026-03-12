package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/huh"

	"sub2api-scripts/internal/api"
)

func runProxyCheck(client *api.Client, _ string) {
	printHeader("代理连通性检测")

	// 获取代理列表
	fmt.Printf("%s 正在获取代理列表...\n", infoIcon)
	proxies, err := client.FetchProxies()
	if err != nil {
		fmt.Printf("%s 获取代理列表失败: %v\n", failIcon, err)
		return
	}
	if len(proxies) == 0 {
		fmt.Printf("%s 没有代理\n", warnIcon)
		return
	}

	// 获取账号列表统计绑定数
	fmt.Printf("%s 正在获取账号列表...\n", infoIcon)
	accounts, err := client.FetchAccounts(api.AccountListOptions{
		Platform: "anthropic",
		Type:     "oauth",
	})
	if err != nil {
		fmt.Printf("%s 获取账号列表失败: %v\n", failIcon, err)
		return
	}

	accCountMap := make(map[int64]int)
	for _, acc := range accounts {
		if acc.ProxyID != nil {
			accCountMap[*acc.ProxyID]++
		}
	}

	// 测试所有代理
	type proxyResult struct {
		proxy     api.Proxy
		success   bool
		message   string
		latencyMs int64
		ipAddress string
		accCount  int
	}

	fmt.Printf("\n正在测试 %d 条代理的连通性...\n", len(proxies))
	printSeparator(90)

	var results []proxyResult
	for i, p := range proxies {
		fmt.Printf("[%d/%d] [ID:%d] %-25s ...", i+1, len(proxies), p.ID, p.Name)

		r := proxyResult{proxy: p, accCount: accCountMap[p.ID]}

		testResult, err := client.TestProxy(p.ID)
		if err != nil {
			r.message = err.Error()
			fmt.Printf(" %s %v\n", failIcon, err)
		} else if !testResult.Success {
			r.message = testResult.Message
			fmt.Printf(" %s %s\n", failIcon, testResult.Message)
		} else {
			r.success = true
			r.latencyMs = testResult.LatencyMs
			r.ipAddress = testResult.IPAddress
			fmt.Printf(" %s (%dms, %s)\n", successIcon, r.latencyMs, r.ipAddress)
		}

		results = append(results, r)
		if i < len(proxies)-1 {
			time.Sleep(200 * time.Millisecond)
		}
	}

	// 汇总
	var okList, failList []proxyResult
	for _, r := range results {
		if r.success {
			okList = append(okList, r)
		} else {
			failList = append(failList, r)
		}
	}

	fmt.Printf("\n%s 测试完成: 成功 %d，失败 %d，共 %d\n", infoIcon, len(okList), len(failList), len(results))
	printSeparator(90)

	if len(okList) > 0 {
		fmt.Printf("\n正常代理（%d 条）:\n", len(okList))
		printSeparator(90)
		for _, r := range okList {
			fmt.Printf("  %s [ID:%d] %-25s %4dms  %-15s  %d 个账号\n",
				successIcon, r.proxy.ID, r.proxy.Name, r.latencyMs, r.ipAddress, r.accCount)
		}
	}

	if len(failList) > 0 {
		fmt.Printf("\n异常代理（%d 条）:\n", len(failList))
		printSeparator(90)
		for i, r := range failList {
			fmt.Printf("  %d. %s [ID:%d] %-25s %d 个账号  原因: %s\n",
				i+1, failIcon, r.proxy.ID, r.proxy.Name, r.accCount, r.message)
		}
	}
	fmt.Println()

	if len(failList) == 0 {
		fmt.Printf("%s 所有代理均正常，无需处理\n", successIcon)
		return
	}

	// 交互选择删除
	options := make([]huh.Option[int], len(failList))
	for i, r := range failList {
		label := fmt.Sprintf("[ID:%d] %s (%d 个账号)", r.proxy.ID, r.proxy.Name, r.accCount)
		options[i] = huh.NewOption(label, i)
	}

	var selected []int
	err = huh.NewMultiSelect[int]().
		Title("选择要删除的异常代理（空选跳过）").
		Options(options...).
		Value(&selected).
		Run()
	if err != nil || len(selected) == 0 {
		fmt.Println("已跳过")
		return
	}

	// 收集要删除的代理
	var toDelete []proxyResult
	for _, idx := range selected {
		toDelete = append(toDelete, failList[idx])
	}

	totalAccounts := 0
	for _, r := range toDelete {
		totalAccounts += r.accCount
	}

	fmt.Printf("\n将删除以下 %d 条代理", len(toDelete))
	if totalAccounts > 0 {
		fmt.Printf("（需先解绑 %d 个账号）", totalAccounts)
	}
	fmt.Println(":")
	for _, r := range toDelete {
		fmt.Printf("  [ID:%d] %s (%d 个账号)\n", r.proxy.ID, r.proxy.Name, r.accCount)
	}

	if !confirm("确认删除？") {
		fmt.Println("已取消")
		return
	}

	deleteSet := make(map[int64]bool)
	for _, r := range toDelete {
		deleteSet[r.proxy.ID] = true
	}

	// 步骤1: 解绑账号
	if totalAccounts > 0 {
		fmt.Println("\n步骤 1/2: 解绑账号...")
		unbindOK, unbindFail := 0, 0
		for _, acc := range accounts {
			if acc.ProxyID == nil || !deleteSet[*acc.ProxyID] {
				continue
			}
			fmt.Printf("  %s 解绑代理...", acc.Name)
			err := client.UnbindProxy(acc.ID)
			if err != nil {
				fmt.Printf(" %s %v\n", failIcon, err)
				unbindFail++
			} else {
				fmt.Printf(" %s\n", successIcon)
				unbindOK++
			}
			time.Sleep(200 * time.Millisecond)
		}
		fmt.Printf("解绑完成: 成功 %d，失败 %d\n", unbindOK, unbindFail)

		if unbindFail > 0 {
			fmt.Printf("%s 部分账号解绑失败，无法安全删除代理\n", failIcon)
			return
		}
	} else {
		fmt.Println("\n步骤 1/2: 无需解绑账号")
	}

	// 步骤2: 删除代理
	fmt.Println("步骤 2/2: 删除代理...")
	deleteOK, deleteFail := 0, 0
	for _, r := range toDelete {
		fmt.Printf("  删除 [ID:%d] %s ...", r.proxy.ID, r.proxy.Name)
		err := client.DeleteProxy(r.proxy.ID)
		if err != nil {
			fmt.Printf(" %s %v\n", failIcon, err)
			deleteFail++
		} else {
			fmt.Printf(" %s\n", successIcon)
			deleteOK++
		}
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Printf("\n%s 全部完成: 解绑 %d 个账号，删除 %d/%d 个代理\n",
		successIcon, totalAccounts, deleteOK, len(toDelete))
}
