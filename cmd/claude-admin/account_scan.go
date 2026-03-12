package main

import (
	"fmt"
	"text/tabwriter"
	"os"
	"time"

	"sub2api-scripts/internal/api"
)

func runAccountScan(client *api.Client, model string) {
	printHeader("协议状态扫描")

	// 获取所有活跃账号
	fmt.Printf("%s 正在获取 Claude OAuth 账号列表...\n", infoIcon)
	accounts, err := client.FetchAccounts(api.AccountListOptions{
		Platform: "anthropic",
		Type:     "oauth",
		Status:   "active",
	})
	if err != nil {
		fmt.Printf("%s 获取账号列表失败: %v\n", failIcon, err)
		return
	}
	fmt.Printf("%s 共找到 %d 个活跃账号\n\n", successIcon, len(accounts))

	if len(accounts) == 0 {
		return
	}

	// 逐个测试
	type scanResult struct {
		account api.Account
		status  string
		detail  string
	}
	var results []scanResult

	for i, acc := range accounts {
		fmt.Printf("[%d/%d] 测试 %s (ID: %d)...", i+1, len(accounts), acc.Name, acc.ID)

		testResult := client.TestAccountDetail(acc.ID, model)
		if testResult.Success {
			fmt.Printf(" %s 正常\n", successIcon)
			results = append(results, scanResult{acc, "正常", "-"})
		} else {
			errMsg := testResult.Error
			status := "其他错误"
			switch api.ClassifyError(errMsg) {
			case "need_terms":
				status = "需要接受协议"
			case "auth_failed":
				status = "认证失败"
			case "rate_limited":
				status = "速率限制"
			case "overloaded":
				status = "服务过载"
			}
			fmt.Printf(" %s %s\n", failIcon, status)
			results = append(results, scanResult{acc, status, errMsg})
		}

		if i < len(accounts)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 汇总
	printSeparator(80)
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\t名称\t状态\t详情")
	fmt.Fprintln(w, "--\t----\t----\t----")

	counts := map[string]int{}
	for _, r := range results {
		detail := r.detail
		if len(detail) > 60 {
			detail = detail[:60] + "..."
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", r.account.ID, r.account.Name, r.status, detail)
		counts[r.status]++
	}
	w.Flush()

	printSeparator(80)
	fmt.Printf("总计: %d | 正常: %d | 需要接受协议: %d | 认证失败: %d | 速率限制: %d | 过载: %d | 其他: %d\n",
		len(results), counts["正常"], counts["需要接受协议"], counts["认证失败"],
		counts["速率限制"], counts["服务过载"], counts["其他错误"])
}
