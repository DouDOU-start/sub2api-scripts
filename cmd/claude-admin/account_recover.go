package main

import (
	"fmt"
	"time"

	"sub2api-scripts/internal/api"
)

func runAccountRecover(client *api.Client, model string) {
	printHeader("批量恢复错误账号")

	// 获取所有账号（不过滤状态，找出 error 和调度关闭的）
	fmt.Printf("%s 正在获取账号列表...\n", infoIcon)
	accounts, err := client.FetchAccounts(api.AccountListOptions{
		Platform: "anthropic",
		Type:     "oauth",
	})
	if err != nil {
		fmt.Printf("%s 获取账号列表失败: %v\n", failIcon, err)
		return
	}

	// 筛选：error 状态 或 调度已关闭
	var targets []api.Account
	for _, acc := range accounts {
		if acc.Status == "error" || !acc.Schedulable {
			targets = append(targets, acc)
		}
	}

	if len(targets) == 0 {
		fmt.Printf("%s 没有需要恢复的账号\n", successIcon)
		return
	}

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

	if !confirm(fmt.Sprintf("开始逐个测试并恢复这 %d 个账号？", len(targets))) {
		fmt.Println("已取消")
		return
	}
	fmt.Println()

	// 逐个测试
	type recoverResult struct {
		account api.Account
		tag     string // 原始状态标签
		tested  bool
		ok      bool
		detail  string
	}
	var results []recoverResult
	recovered, failed := 0, 0

	for i, acc := range targets {
		tag := acc.Status
		if !acc.Schedulable {
			tag += "/调度关闭"
		}
		fmt.Printf("[%d/%d] [ID:%d] %-35s [%s]\n", i+1, len(targets), acc.ID, acc.Name, tag)

		// 刷新令牌
		fmt.Printf("  刷新令牌...")
		if err := client.RefreshToken(acc.ID); err != nil {
			fmt.Printf(" %s %v\n", warnIcon, err)
			// 刷新失败不阻断，继续测试（可能 token 本身还有效）
		} else {
			fmt.Printf(" %s\n", successIcon)
		}

		// 测试连接
		fmt.Printf("  测试连接...")
		testResult := client.TestAccountDetail(acc.ID, model)
		if !testResult.Success {
			fmt.Printf(" %s %s\n", failIcon, testResult.Error)
			results = append(results, recoverResult{acc, tag, true, false, testResult.Error})
			failed++

			if i < len(targets)-1 {
				time.Sleep(300 * time.Millisecond)
			}
			continue
		}
		fmt.Printf(" %s\n", successIcon)

		// 测试通过，恢复状态
		fmt.Printf("  恢复调度...")
		if err := client.EnableSchedule(acc.ID); err != nil {
			fmt.Printf(" %s %v\n", failIcon, err)
			results = append(results, recoverResult{acc, tag, true, true, "测试通过但恢复失败: " + err.Error()})
			failed++
		} else {
			fmt.Printf(" %s\n", successIcon)
			results = append(results, recoverResult{acc, tag, true, true, "已恢复"})
			recovered++
		}

		if i < len(targets)-1 {
			time.Sleep(300 * time.Millisecond)
		}
	}

	// 汇总
	printSeparator(80)
	fmt.Printf("恢复结果\n")
	printSeparator(80)

	for _, r := range results {
		icon := successIcon
		status := "已恢复"
		if !r.ok {
			icon = failIcon
			status = "失败"
		}
		detail := r.detail
		if len(detail) > 50 {
			detail = detail[:50] + "..."
		}
		fmt.Printf("  %s [ID:%d] %-35s [%s] %s - %s\n",
			icon, r.account.ID, r.account.Name, r.tag, status, detail)
	}

	printSeparator(80)
	fmt.Printf("总计: %d | 已恢复: %d | 失败: %d\n", len(results), recovered, failed)
}
