package main

import (
	"fmt"
	"time"

	"sub2api-scripts/internal/api"
)

func runAccountDeleteError(client *api.Client, _ string) {
	printHeader("批量删除异常账号")

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

	// 筛选异常账号：error 状态 或 调度关闭
	var targets []api.Account
	for _, acc := range accounts {
		if acc.Status == "error" || !acc.Schedulable {
			targets = append(targets, acc)
		}
	}

	if len(targets) == 0 {
		fmt.Printf("%s 没有异常账号\n", successIcon)
		return
	}

	// 统计并展示
	errorCount, disabledCount := 0, 0
	for _, acc := range targets {
		if acc.Status == "error" {
			errorCount++
		}
		if !acc.Schedulable {
			disabledCount++
		}
	}

	fmt.Printf("%s 发现 %d 个异常账号（error: %d，调度关闭: %d）\n\n",
		warnIcon, len(targets), errorCount, disabledCount)

	printSeparator(70)
	for i, acc := range targets {
		tag := acc.Status
		if !acc.Schedulable {
			tag += "/调度关闭"
		}
		fmt.Printf("  %d. [ID:%d] %-35s [%s]\n", i+1, acc.ID, acc.Name, tag)
	}
	printSeparator(70)
	fmt.Println()

	if !confirm(fmt.Sprintf("确认删除这 %d 个异常账号？此操作不可恢复！", len(targets))) {
		fmt.Println("已取消")
		return
	}
	fmt.Println()

	// 执行删除
	ok, fail := 0, 0
	for i, acc := range targets {
		fmt.Printf("[%d/%d] [ID:%d] %-35s ...", i+1, len(targets), acc.ID, acc.Name)

		if err := client.DeleteAccount(acc.ID); err != nil {
			fmt.Printf(" %s %v\n", failIcon, err)
			fail++
		} else {
			fmt.Printf(" %s\n", successIcon)
			ok++
		}

		if i < len(targets)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	fmt.Printf("\n%s 删除完成: 成功 %d，失败 %d\n", infoIcon, ok, fail)
}
