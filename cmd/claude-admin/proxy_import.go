package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"sub2api-scripts/internal/api"
)

// proxyEntry 解析后的代理条目
type proxyEntry struct {
	host     string
	port     int
	username string
	password string
}

func runProxyImport(client *api.Client, _ string) {
	printHeader("导入代理")

	// 选择协议
	protocols := []string{"socks5", "http", "https"}
	protocolIdx, err := selectOne("代理协议", protocols)
	if err != nil {
		fmt.Println("已取消")
		return
	}
	protocol := protocols[protocolIdx]

	// 选择输入方式
	inputMode, err := selectOne("数据来源", []string{
		"从文件读取",
		"手动输入（每行一个，空行结束）",
	})
	if err != nil {
		fmt.Println("已取消")
		return
	}

	var lines []string
	if inputMode == 0 {
		entries, dirErr := os.ReadDir("data")
		if dirErr != nil {
			fmt.Printf("%s 读取 data 目录失败: %v\n", failIcon, dirErr)
			return
		}
		var files []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".txt") {
				files = append(files, e.Name())
			}
		}
		if len(files) == 0 {
			fmt.Printf("%s data 目录下没有 .txt 文件\n", warnIcon)
			return
		}
		fileIdx, err := selectOne("选择代理文件", files)
		if err != nil {
			fmt.Println("已取消")
			return
		}
		lines = readLinesFromFile("data/" + files[fileIdx])
	} else {
		fmt.Println("请输入代理地址（格式: [user:pass@]host:port，每行一个，空行结束）:")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				break
			}
			lines = append(lines, line)
		}
	}

	if len(lines) == 0 {
		fmt.Printf("%s 未读取到任何代理\n", warnIcon)
		return
	}

	// 解析并去重
	seen := make(map[string]bool)
	var parsed []proxyEntry
	for i, line := range lines {
		entry, parseErr := parseProxyLine(line)
		if parseErr != nil {
			fmt.Printf("%s 第 %d 行格式错误: %s (%v)\n", warnIcon, i+1, line, parseErr)
			continue
		}
		key := fmt.Sprintf("%s:%d", entry.host, entry.port)
		if seen[key] {
			continue
		}
		seen[key] = true
		parsed = append(parsed, entry)
	}

	if len(parsed) == 0 {
		fmt.Printf("%s 没有有效的代理地址\n", warnIcon)
		return
	}

	// 获取现有代理，用于跳过重复
	fmt.Printf("%s 正在获取现有代理列表...\n", infoIcon)
	existing, err := client.FetchProxiesPaginated("")
	if err != nil {
		fmt.Printf("%s 获取现有代理失败: %v\n", failIcon, err)
		return
	}
	existingSet := make(map[string]bool)
	for _, p := range existing {
		existingSet[fmt.Sprintf("%s:%d", p.Host, p.Port)] = true
	}

	var newEntries []proxyEntry
	skipped := 0
	for _, e := range parsed {
		key := fmt.Sprintf("%s:%d", e.host, e.port)
		if existingSet[key] {
			skipped++
			continue
		}
		newEntries = append(newEntries, e)
	}

	if skipped > 0 {
		fmt.Printf("%s 跳过 %d 条已存在的代理\n", warnIcon, skipped)
	}
	if len(newEntries) == 0 {
		fmt.Printf("%s 所有代理均已存在，无需导入\n", successIcon)
		return
	}

	// 预览
	fmt.Printf("\n将导入 %d 条代理（协议: %s）:\n", len(newEntries), protocol)
	printSeparator(60)
	for i, e := range newEntries {
		if e.username != "" {
			fmt.Printf("  %d. %s:%d (认证: %s)\n", i+1, e.host, e.port, e.username)
		} else {
			fmt.Printf("  %d. %s:%d\n", i+1, e.host, e.port)
		}
	}
	fmt.Println()

	if !confirm("确认导入？") {
		fmt.Println("已取消")
		return
	}

	// 执行导入
	ok, fail := 0, 0
	for i, e := range newEntries {
		name := fmt.Sprintf("%s:%d", e.host, e.port)
		fmt.Printf("[%d/%d] %s ...", i+1, len(newEntries), name)

		_, createErr := client.CreateProxy(api.CreateProxyRequest{
			Name:     name,
			Protocol: protocol,
			Host:     e.host,
			Port:     e.port,
			Username: e.username,
			Password: e.password,
		})
		if createErr != nil {
			fmt.Printf(" %s %v\n", failIcon, createErr)
			fail++
		} else {
			fmt.Printf(" %s\n", successIcon)
			ok++
		}
		if i < len(newEntries)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	fmt.Printf("\n%s 导入完成: 成功 %d，失败 %d\n", successIcon, ok, fail)
}

// parseProxyLine 解析代理行，支持以下格式:
//   - host:port
//   - user:pass@host:port
//   - protocol://[user:pass@]host:port
func parseProxyLine(s string) (proxyEntry, error) {
	// 去除协议前缀
	for _, prefix := range []string{"http://", "https://", "socks5://", "socks4://"} {
		s = strings.TrimPrefix(s, prefix)
	}

	var username, password string

	// 检查是否有 user:pass@ 认证信息
	if atIdx := strings.LastIndex(s, "@"); atIdx >= 0 {
		auth := s[:atIdx]
		s = s[atIdx+1:]
		if colonIdx := strings.Index(auth, ":"); colonIdx >= 0 {
			username = auth[:colonIdx]
			password = auth[colonIdx+1:]
		} else {
			username = auth
		}
	}

	// 解析 host:port
	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return proxyEntry{}, fmt.Errorf("缺少端口号")
	}
	host := s[:idx]
	port, err := strconv.Atoi(s[idx+1:])
	if err != nil {
		return proxyEntry{}, fmt.Errorf("端口号无效: %s", s[idx+1:])
	}
	if port < 1 || port > 65535 {
		return proxyEntry{}, fmt.Errorf("端口号超出范围: %d", port)
	}

	return proxyEntry{
		host:     host,
		port:     port,
		username: username,
		password: password,
	}, nil
}
