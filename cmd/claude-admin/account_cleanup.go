package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/huh"

	"sub2api-scripts/internal/api"
)

func runAccountCleanup(client *api.Client, model string) {
	printHeader("清理异常账号代理")

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

	// 筛选：绑了代理 + (非 active 或 调度已关闭)
	var suspects []api.Account
	for _, acc := range accounts {
		if acc.ProxyID == nil {
			continue
		}
		if acc.Status != "active" || !acc.Schedulable {
			suspects = append(suspects, acc)
		}
	}

	if len(suspects) == 0 {
		fmt.Printf("%s 没有异常账号占用代理\n", successIcon)
		return
	}

	fmt.Printf("%s 发现 %d 个异常账号绑着代理，开始测试连通性...\n\n", warnIcon, len(suspects))

	// 逐个测试
	type checkResult struct {
		account api.Account
		ok      bool
		detail  string
	}
	var results []checkResult

	for i, acc := range suspects {
		tag := acc.Status
		if !acc.Schedulable {
			tag += "/调度关闭"
		}
		fmt.Printf("[%d/%d] [ID:%d] %-35s [%s] ...", i+1, len(suspects), acc.ID, acc.Name, tag)

		testResult := client.TestAccountDetail(acc.ID, model)
		if testResult.Success {
			fmt.Printf(" %s 正常\n", successIcon)
			results = append(results, checkResult{acc, true, "连接正常"})
		} else {
			fmt.Printf(" %s %s\n", failIcon, testResult.Error)
			results = append(results, checkResult{acc, false, testResult.Error})
		}

		if i < len(suspects)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 分类
	var okList, failList []checkResult
	for _, r := range results {
		if r.ok {
			okList = append(okList, r)
		} else {
			failList = append(failList, r)
		}
	}

	fmt.Printf("\n%s 测试完成: 正常 %d，异常 %d\n", infoIcon, len(okList), len(failList))

	if len(okList) > 0 {
		fmt.Printf("\n可恢复的账号（测试通过，建议重新开启调度）:\n")
		for _, r := range okList {
			fmt.Printf("  %s [ID:%d] %s\n", successIcon, r.account.ID, r.account.Name)
		}
	}

	if len(failList) == 0 {
		fmt.Printf("\n%s 没有需要解绑代理的账号\n", successIcon)
		return
	}

	// 让用户选择要解绑的
	fmt.Printf("\n确认不可用的账号（%d 个）:\n", len(failList))
	printSeparator(80)

	options := make([]huh.Option[int], len(failList))
	var preselected []int
	for i, r := range failList {
		detail := r.detail
		if len(detail) > 50 {
			detail = detail[:50] + "..."
		}
		label := fmt.Sprintf("[ID:%d] %s - %s", r.account.ID, r.account.Name, detail)
		options[i] = huh.NewOption(label, i)
		preselected = append(preselected, i)
	}

	selected := preselected
	err = huh.NewMultiSelect[int]().
		Title("选择要解绑代理的账号（空格切换选中，回车确认）").
		Options(options...).
		Value(&selected).
		Run()
	if err != nil || len(selected) == 0 {
		fmt.Println("已跳过")
		return
	}

	// 确认
	fmt.Printf("\n将解绑 %d 个账号的代理\n", len(selected))
	if !confirm("确认执行？") {
		fmt.Println("已取消")
		return
	}

	// 执行解绑
	unbindOK, unbindFail := 0, 0
	for _, idx := range selected {
		acc := failList[idx].account
		fmt.Printf("  [ID:%d] %s 解绑代理...", acc.ID, acc.Name)
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

	fmt.Printf("\n%s 完成: 成功解绑 %d，失败 %d\n", successIcon, unbindOK, unbindFail)
}
