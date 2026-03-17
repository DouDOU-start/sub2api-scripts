package main

import (
	"fmt"
	"time"

	"sub2api-scripts/internal/api"
)

func runAccountUnbindAndDeleteProxies(client *api.Client, _ string) {
	printHeader("解绑所有代理并删除")

	// 获取账号列表
	fmt.Printf("%s 正在获取账号列表...\n", infoIcon)
	accounts, err := client.FetchAccounts(api.AccountListOptions{
		Platform: "anthropic",
		Type:     "oauth",
	})
	if err != nil {
		fmt.Printf("%s 获取账号列表失败: %v\n", failIcon, err)
		return
	}

	// 获取代理列表
	fmt.Printf("%s 正在获取代理列表...\n", infoIcon)
	proxies, err := client.FetchProxiesPaginated("")
	if err != nil {
		fmt.Printf("%s 获取代理列表失败: %v\n", failIcon, err)
		return
	}

	// 统计绑定了代理的账号
	var boundAccounts []api.Account
	for _, acc := range accounts {
		if acc.ProxyID != nil {
			boundAccounts = append(boundAccounts, acc)
		}
	}

	fmt.Printf("%s 绑定代理的账号: %d 个，代理总数: %d 个\n", infoIcon, len(boundAccounts), len(proxies))

	if len(boundAccounts) == 0 && len(proxies) == 0 {
		fmt.Printf("%s 没有需要处理的数据\n", successIcon)
		return
	}

	if !confirm(fmt.Sprintf("确认解绑 %d 个账号的代理并删除全部 %d 个代理？", len(boundAccounts), len(proxies))) {
		fmt.Println("已取消")
		return
	}
	fmt.Println()

	// 步骤1: 解绑所有账号代理
	if len(boundAccounts) > 0 {
		fmt.Printf("步骤 1/2: 解绑 %d 个账号的代理...\n", len(boundAccounts))
		unbindOK, unbindFail := 0, 0
		for i, acc := range boundAccounts {
			fmt.Printf("[%d/%d] %-35s ...", i+1, len(boundAccounts), acc.Name)
			if err := client.UnbindProxy(acc.ID); err != nil {
				fmt.Printf(" %s %v\n", failIcon, err)
				unbindFail++
			} else {
				fmt.Printf(" %s\n", successIcon)
				unbindOK++
			}
			if i < len(boundAccounts)-1 {
				time.Sleep(100 * time.Millisecond)
			}
		}
		fmt.Printf("%s 解绑完成: 成功 %d，失败 %d\n\n", infoIcon, unbindOK, unbindFail)

		if unbindFail > 0 && !confirm("部分账号解绑失败，是否继续删除代理？") {
			fmt.Println("已取消")
			return
		}
	} else {
		fmt.Println("步骤 1/2: 无需解绑账号")
	}

	// 步骤2: 删除所有代理
	if len(proxies) > 0 {
		fmt.Printf("步骤 2/2: 删除 %d 个代理...\n", len(proxies))
		deleteOK, deleteFail := 0, 0
		for i, p := range proxies {
			fmt.Printf("[%d/%d] [ID:%d] %-25s ...", i+1, len(proxies), p.ID, p.Name)
			if err := client.DeleteProxy(p.ID); err != nil {
				fmt.Printf(" %s %v\n", failIcon, err)
				deleteFail++
			} else {
				fmt.Printf(" %s\n", successIcon)
				deleteOK++
			}
			if i < len(proxies)-1 {
				time.Sleep(100 * time.Millisecond)
			}
		}
		fmt.Printf("\n%s 全部完成: 解绑 %d 个账号，删除 %d/%d 个代理\n",
			successIcon, len(boundAccounts), deleteOK, len(proxies))
	} else {
		fmt.Println("步骤 2/2: 无代理需要删除")
	}
}
