package main

import (
	"fmt"
	"time"

	"sub2api-scripts/internal/api"
)

func runProxyRename(client *api.Client, _ string) {
	printHeader("代理批量重命名")

	// 获取所有代理
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

	// 选择命名规则
	ruleIdx, err := selectOne("命名规则", []string{
		"使用 host 地址命名（如 192.168.1.1）",
		"使用 host:port 命名（如 192.168.1.1:1080）",
		"使用 protocol://host:port 命名（完整地址）",
		"使用前缀+序号命名（如 proxy-01）",
	})
	if err != nil {
		fmt.Println("已取消")
		return
	}

	var prefix string
	if ruleIdx == 3 {
		prefix, err = inputText("名称前缀", "proxy", "")
		if err != nil || prefix == "" {
			fmt.Println("已取消")
			return
		}
	}

	// 预览
	type renameItem struct {
		proxy   api.Proxy
		newName string
	}
	var items []renameItem
	for i, p := range proxies {
		var newName string
		switch ruleIdx {
		case 0:
			newName = p.Host
		case 1:
			newName = fmt.Sprintf("%s:%d", p.Host, p.Port)
		case 2:
			newName = fmt.Sprintf("%s://%s:%d", p.Protocol, p.Host, p.Port)
		case 3:
			newName = fmt.Sprintf("%s-%02d", prefix, i+1)
		}
		if newName == p.Name {
			continue
		}
		items = append(items, renameItem{proxy: p, newName: newName})
	}

	if len(items) == 0 {
		fmt.Printf("%s 所有代理名称已是最新，无需更改\n", successIcon)
		return
	}

	fmt.Printf("\n将重命名 %d 条代理:\n", len(items))
	printSeparator(80)
	for _, item := range items {
		fmt.Printf("  [ID:%d] %s -> %s\n", item.proxy.ID, item.proxy.Name, item.newName)
	}
	fmt.Println()

	if !confirm("确认执行？") {
		fmt.Println("已取消")
		return
	}

	// 执行
	ok, fail := 0, 0
	for i, item := range items {
		fmt.Printf("[%d/%d] [ID:%d] %s -> %s ...", i+1, len(items), item.proxy.ID, item.proxy.Name, item.newName)
		err := client.UpdateProxy(item.proxy.ID, api.UpdateProxyRequest{Name: &item.newName})
		if err != nil {
			fmt.Printf(" %s %v\n", failIcon, err)
			fail++
		} else {
			fmt.Printf(" %s\n", successIcon)
			ok++
		}
		if i < len(items)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	fmt.Printf("\n%s 完成: 成功 %d，失败 %d\n", successIcon, ok, fail)
}
