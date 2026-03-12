package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"sub2api-scripts/internal/api"
)

func runAccountList(client *api.Client, _ string) {
	printHeader("查看所有账号")

	// 获取所有账号（不过滤状态）
	fmt.Printf("%s 正在获取账号列表...\n", infoIcon)
	accounts, err := client.FetchAccounts(api.AccountListOptions{
		Platform: "anthropic",
		Type:     "oauth",
	})
	if err != nil {
		fmt.Printf("%s 获取账号列表失败: %v\n", failIcon, err)
		return
	}
	if len(accounts) == 0 {
		fmt.Printf("%s 没有账号\n", warnIcon)
		return
	}

	// 获取代理列表做名称映射
	proxyNameMap := make(map[int64]string)
	proxies, err := client.FetchProxiesPaginated("")
	if err != nil {
		fmt.Printf("%s 获取代理列表失败: %v，代理列将显示 ID\n", warnIcon, err)
	} else {
		for _, p := range proxies {
			proxyNameMap[p.ID] = p.Name
		}
	}

	// 统计
	statusCount := make(map[string]int)
	scheduleCount := map[bool]int{}
	withProxy, withoutProxy := 0, 0
	for _, acc := range accounts {
		statusCount[acc.Status]++
		scheduleCount[acc.Schedulable]++
		if acc.ProxyID != nil {
			withProxy++
		} else {
			withoutProxy++
		}
	}

	// 汇总
	fmt.Printf("\n%s 共 %d 个账号\n", infoIcon, len(accounts))
	fmt.Printf("  状态: ")
	for status, count := range statusCount {
		fmt.Printf("%s=%d ", status, count)
	}
	fmt.Println()
	fmt.Printf("  调度: 开启=%d 关闭=%d\n", scheduleCount[true], scheduleCount[false])
	fmt.Printf("  代理: 有=%d 无=%d\n", withProxy, withoutProxy)
	fmt.Println()

	// 表格
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\t名称\t状态\t调度\t并发\t代理")
	fmt.Fprintln(w, "--\t----\t----\t----\t----\t----")

	for _, acc := range accounts {
		schedule := "开"
		if !acc.Schedulable {
			schedule = "关"
		}

		proxyInfo := "-"
		if acc.ProxyID != nil {
			if name, ok := proxyNameMap[*acc.ProxyID]; ok && name != "" {
				proxyInfo = fmt.Sprintf("%s(ID:%d)", name, *acc.ProxyID)
			} else {
				proxyInfo = fmt.Sprintf("ID:%d", *acc.ProxyID)
			}
		}

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%d\t%s\n",
			acc.ID, acc.Name, acc.Status, schedule, acc.Concurrency, proxyInfo)
	}
	w.Flush()
}
