package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"sub2api-scripts/internal/api"
)

func runProxyExportIP(client *api.Client, _ string) {
	printHeader("导出代理 IP")

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

	// 拼接完整代理地址
	var lines string
	for _, p := range proxies {
		var addr string
		if p.Username != "" {
			addr = fmt.Sprintf("%s://%s:%s@%s:%d", p.Protocol, p.Username, p.Password, p.Host, p.Port)
		} else {
			addr = fmt.Sprintf("%s://%s:%d", p.Protocol, p.Host, p.Port)
		}
		lines += addr + "\n"
	}

	// 写入文件
	filename := fmt.Sprintf("proxy-ip-%s.txt", time.Now().Format("200601021504"))
	outPath := filepath.Join("data", filename)
	_ = os.MkdirAll("data", 0o755)

	if err := os.WriteFile(outPath, []byte(lines), 0o644); err != nil {
		fmt.Printf("%s 写入文件失败: %v\n", failIcon, err)
		return
	}

	fmt.Printf("%s 导出完成: %d 条代理已写入 %s\n", successIcon, len(proxies), outPath)
}
