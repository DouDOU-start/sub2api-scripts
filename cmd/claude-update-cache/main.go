// 批量更新所有 Claude OAuth 账号的缓存 TTL 配置
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"sub2api-scripts/internal/api"
	"sub2api-scripts/internal/config"
)

func main() {
	config.LoadEnvFile()

	apiURL := flag.String("api-url", "", "sub2api 服务地址")
	apiKey := flag.String("api-key", "", "管理员 API Key")
	cacheTTL := flag.String("cache-ttl", "5m", "缓存 TTL（如 5m, 1h）")
	flag.Parse()

	finalURL := config.Get(*apiURL, "SUB2API_URL", "http://localhost:8080")
	finalKey := config.Get(*apiKey, "SUB2API_KEY", "")

	if finalKey == "" {
		fmt.Fprintln(os.Stderr, "错误: 未找到 API Key，请通过 --api-key、环境变量 SUB2API_KEY 或 .env 文件提供")
		os.Exit(1)
	}

	client := api.NewClient(finalURL, finalKey)

	// 获取所有 Claude OAuth 账号
	fmt.Println("正在获取所有 Claude OAuth 账号...")
	accounts, err := client.FetchAccounts(api.AccountListOptions{
		Platform: "anthropic",
		Type:     "oauth",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取账号列表失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("共 %d 个账号，开始更新缓存 TTL 为 %s\n\n", len(accounts), *cacheTTL)

	success, fail := 0, 0
	for i, acc := range accounts {
		fmt.Printf("[%d/%d] %s (ID: %d) ...", i+1, len(accounts), acc.Name, acc.ID)

		err := client.UpdateAccount(acc.ID, api.UpdateAccountRequest{
			Extra: map[string]any{
				"enable_tls_fingerprint":     true,
				"session_id_masking_enabled": true,
				"cache_ttl_override_enabled": true,
				"cache_ttl_override_target":  *cacheTTL,
			},
			ConfirmMixedChannelRisk: true,
		})
		if err != nil {
			fmt.Printf(" 失败: %v\n", err)
			fail++
		} else {
			fmt.Printf(" OK\n")
			success++
		}

		if i < len(accounts)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	fmt.Printf("\n完成！成功: %d，失败: %d，共: %d\n", success, fail, len(accounts))
}
