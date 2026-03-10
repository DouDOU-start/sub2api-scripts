// Claude OAuth 账号协议状态批量扫描脚本
// 通过 sub2api 管理员 API 扫描所有 Claude OAuth 账号，识别需要接受协议的账号
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"sub2api-scripts/internal/api"
	"sub2api-scripts/internal/config"
)

// 扫描状态
type ScanStatus string

const (
	StatusOK          ScanStatus = "正常"
	StatusNeedTerms   ScanStatus = "需要接受协议"
	StatusAuthFailed  ScanStatus = "认证失败"
	StatusError       ScanStatus = "其他错误"
	StatusRateLimited ScanStatus = "速率限制"
	StatusOverloaded  ScanStatus = "服务过载"
)

type ScanResult struct {
	Account api.Account
	Status  ScanStatus
	Detail  string
}

func main() {
	config.LoadEnvFile()

	apiURL := flag.String("api-url", "", "sub2api 服务地址")
	apiKey := flag.String("api-key", "", "管理员 API Key")
	model := flag.String("model", "", "测试用模型（留空使用默认）")
	flag.Parse()

	finalURL := config.Get(*apiURL, "SUB2API_URL", "http://localhost:8080")
	finalKey := config.Get(*apiKey, "SUB2API_KEY", "")
	finalModel := config.Get(*model, "SUB2API_MODEL", "")

	if finalKey == "" {
		fmt.Fprintln(os.Stderr, "错误: 未找到 API Key，请通过 --api-key、环境变量 SUB2API_KEY 或 .env 文件提供")
		os.Exit(1)
	}

	client := api.NewClient(finalURL, finalKey)

	// 1. 获取所有 Claude OAuth 活跃账号
	fmt.Printf("服务地址: %s\n", finalURL)
	fmt.Println("正在获取 Claude OAuth 账号列表...")
	accounts, err := client.FetchAccounts(api.AccountListOptions{
		Platform: "anthropic",
		Type:     "oauth",
		Status:   "active",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取账号列表失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("共找到 %d 个 Claude OAuth 账号\n\n", len(accounts))

	if len(accounts) == 0 {
		fmt.Println("没有找到需要扫描的账号")
		return
	}

	// 2. 逐个测试连接
	var results []ScanResult
	for i, acc := range accounts {
		fmt.Printf("[%d/%d] 测试 %s (ID: %d)...", i+1, len(accounts), acc.Name, acc.ID)
		result := scanAccount(client, acc, finalModel)
		results = append(results, result)
		fmt.Printf(" %s\n", result.Status)

		if i < len(accounts)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 3. 输出汇总
	printResults(results)
}

func scanAccount(client *api.Client, acc api.Account, model string) ScanResult {
	testResult := client.TestAccountDetail(acc.ID, model)
	if testResult.Success {
		return ScanResult{Account: acc, Status: StatusOK, Detail: "-"}
	}

	errMsg := testResult.Error
	switch api.ClassifyError(errMsg) {
	case "need_terms":
		return ScanResult{Account: acc, Status: StatusNeedTerms, Detail: "需要在 claude.ai 接受 Consumer Terms and Privacy Policy"}
	case "auth_failed":
		return ScanResult{Account: acc, Status: StatusAuthFailed, Detail: errMsg}
	case "rate_limited":
		return ScanResult{Account: acc, Status: StatusRateLimited, Detail: errMsg}
	case "overloaded":
		return ScanResult{Account: acc, Status: StatusOverloaded, Detail: errMsg}
	default:
		return ScanResult{Account: acc, Status: StatusError, Detail: errMsg}
	}
}

func printResults(results []ScanResult) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("扫描结果")
	fmt.Println(strings.Repeat("=", 80))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\t名称\t状态\t详情")
	fmt.Fprintln(w, "--\t----\t----\t----")

	countOK, countTerms, countAuth, countRate, countOverload, countErr := 0, 0, 0, 0, 0, 0
	for _, r := range results {
		detail := r.Detail
		if len(detail) > 60 {
			detail = detail[:60] + "..."
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", r.Account.ID, r.Account.Name, r.Status, detail)

		switch r.Status {
		case StatusOK:
			countOK++
		case StatusNeedTerms:
			countTerms++
		case StatusAuthFailed:
			countAuth++
		case StatusRateLimited:
			countRate++
		case StatusOverloaded:
			countOverload++
		case StatusError:
			countErr++
		}
	}
	w.Flush()

	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("总计: %d 个账号\n", len(results))
	fmt.Printf("  正常: %d | 需要接受协议: %d | 认证失败: %d | 速率限制: %d | 过载: %d | 其他错误: %d\n",
		countOK, countTerms, countAuth, countRate, countOverload, countErr)
}
