package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/huh"

	"sub2api-scripts/internal/api"
)

func runProxyDelete(client *api.Client, _ string) {
	printHeader("删除代理")

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

	// 列出所有代理供选择
	options := make([]huh.Option[int], len(proxies))
	for i, p := range proxies {
		label := fmt.Sprintf("[ID:%d] %s (%s) - %d 个账号", p.ID, p.Name, p.Address, accCountMap[p.ID])
		options[i] = huh.NewOption(label, i)
	}

	var selected []int
	err = huh.NewMultiSelect[int]().
		Title("选择要删除的代理（空格切换选中，回车确认）").
		Options(options...).
		Value(&selected).
		Run()
	if err != nil || len(selected) == 0 {
		fmt.Println("已取消")
		return
	}

	// 汇总
	type deleteItem struct {
		proxy    api.Proxy
		accCount int
	}
	var toDelete []deleteItem
	totalAccounts := 0
	for _, idx := range selected {
		p := proxies[idx]
		cnt := accCountMap[p.ID]
		toDelete = append(toDelete, deleteItem{proxy: p, accCount: cnt})
		totalAccounts += cnt
	}

	fmt.Printf("\n将删除以下 %d 条代理", len(toDelete))
	if totalAccounts > 0 {
		fmt.Printf("（需先解绑 %d 个账号）", totalAccounts)
	}
	fmt.Println(":")
	for _, d := range toDelete {
		fmt.Printf("  [ID:%d] %s (%d 个账号)\n", d.proxy.ID, d.proxy.Name, d.accCount)
	}

	if !confirm("确认删除？") {
		fmt.Println("已取消")
		return
	}

	deleteSet := make(map[int64]bool)
	for _, d := range toDelete {
		deleteSet[d.proxy.ID] = true
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
	for _, d := range toDelete {
		fmt.Printf("  删除 [ID:%d] %s ...", d.proxy.ID, d.proxy.Name)
		err := client.DeleteProxy(d.proxy.ID)
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
