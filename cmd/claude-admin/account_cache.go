package main

import (
	"fmt"
	"time"

	"sub2api-scripts/internal/api"
)

func runAccountCache(client *api.Client, _ string) {
	printHeader("批量更新缓存配置")

	// 输入缓存 TTL
	cacheTTL, err := inputText("缓存 TTL", "如 5m, 1h", "5m")
	if err != nil || cacheTTL == "" {
		fmt.Println("已取消")
		return
	}

	// 获取所有账号
	fmt.Printf("\n%s 正在获取所有 Claude OAuth 账号...\n", infoIcon)
	accounts, err := client.FetchAccounts(api.AccountListOptions{
		Platform: "anthropic",
		Type:     "oauth",
	})
	if err != nil {
		fmt.Printf("%s 获取账号列表失败: %v\n", failIcon, err)
		return
	}
	fmt.Printf("%s 共 %d 个账号，开始更新缓存 TTL 为 %s\n\n", infoIcon, len(accounts), cacheTTL)

	if !confirm(fmt.Sprintf("确认更新 %d 个账号的缓存配置？", len(accounts))) {
		fmt.Println("已取消")
		return
	}

	fmt.Println()
	success, fail := 0, 0
	for i, acc := range accounts {
		fmt.Printf("[%d/%d] %s (ID: %d) ...", i+1, len(accounts), acc.Name, acc.ID)

		err := client.UpdateAccount(acc.ID, api.UpdateAccountRequest{
			Extra: map[string]any{
				"enable_tls_fingerprint":     true,
				"session_id_masking_enabled": true,
				"cache_ttl_override_enabled": true,
				"cache_ttl_override_target":  cacheTTL,
			},
			ConfirmMixedChannelRisk: true,
		})
		if err != nil {
			fmt.Printf(" %s %v\n", failIcon, err)
			fail++
		} else {
			fmt.Printf(" %s\n", successIcon)
			success++
		}

		if i < len(accounts)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	fmt.Printf("\n%s 完成！成功: %d，失败: %d，共: %d\n", successIcon, success, fail, len(accounts))
}
