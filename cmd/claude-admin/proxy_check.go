package main

import (
	"fmt"
	"time"

	"sub2api-scripts/internal/api"
)

func runProxyCheck(client *api.Client, _ string) {
	printHeader("代理连通性检测")

	// 获取代理列表
	fmt.Printf("%s 正在获取代理列表...\n", infoIcon)
	proxies, err := client.FetchProxiesPaginated("")
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

	if len(failList) > 0 {
		fmt.Printf("\n%s 有 %d 条异常代理，可通过「删除代理」功能处理\n", warnIcon, len(failList))
	}
}
