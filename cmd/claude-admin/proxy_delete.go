package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/huh"

	"sub2api-scripts/internal/api"
)

type deleteItem struct {
	proxy    api.Proxy
	accCount int
}

func runProxyDelete(client *api.Client, _ string) {
	printHeader("删除代理")

	// 选择删除方式
	modeIdx, err := selectOne("删除方式", []string{
		"手动选择代理",
		"按地址列表批量删除（从文件或粘贴输入）",
		"删除所有代理",
	})
	if err != nil {
		fmt.Println("已取消")
		return
	}

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

	// 获取账号列表统计绑定数
	fmt.Printf("%s 正在获取账号列表...\n", infoIcon)
	accounts, err := client.FetchAccounts(api.AccountListOptions{
		Platform: "anthropic",
		Type:     "oauth",
	})
	if err != nil {
		fmt.Printf("%s 获取账号列表失败: %v\n", failIcon, err)
		return
	}

	accCountMap := make(map[int64]int)
	for _, acc := range accounts {
		if acc.ProxyID != nil {
			accCountMap[*acc.ProxyID]++
		}
	}

	var toDelete []deleteItem
	totalAccounts := 0

	switch modeIdx {
	case 0:
		toDelete, totalAccounts = selectProxiesToDelete(proxies, accCountMap)
	case 1:
		toDelete, totalAccounts = matchProxiesToDelete(proxies, accCountMap)
	case 2:
		toDelete, totalAccounts = allProxiesToDelete(proxies, accCountMap)
	}

	if len(toDelete) == 0 {
		return
	}

	fmt.Printf("\n将删除以下 %d 条代理", len(toDelete))
	if totalAccounts > 0 {
		fmt.Printf("（需先解绑 %d 个账号）", totalAccounts)
	}
	fmt.Println(":")
	for _, d := range toDelete {
		fmt.Printf("  [ID:%d] %s (%d 个账号)\n", d.proxy.ID, d.proxy.Name, d.accCount)
	}

	if !confirm("确认删除？") {
		fmt.Println("已取消")
		return
	}

	deleteSet := make(map[int64]bool)
	for _, d := range toDelete {
		deleteSet[d.proxy.ID] = true
	}

	// 步骤1: 解绑账号
	if totalAccounts > 0 {
		fmt.Println("\n步骤 1/2: 解绑账号...")
		unbindOK, unbindFail := 0, 0
		for _, acc := range accounts {
			if acc.ProxyID == nil || !deleteSet[*acc.ProxyID] {
				continue
			}
			fmt.Printf("  %s 解绑代理...", acc.Name)
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
		fmt.Printf("解绑完成: 成功 %d，失败 %d\n", unbindOK, unbindFail)

		if unbindFail > 0 {
			fmt.Printf("%s 部分账号解绑失败，无法安全删除代理\n", failIcon)
			return
		}
	} else {
		fmt.Println("\n步骤 1/2: 无需解绑账号")
	}

	// 步骤2: 删除代理
	fmt.Println("步骤 2/2: 删除代理...")
	deleteOK, deleteFail := 0, 0
	for _, d := range toDelete {
		fmt.Printf("  删除 [ID:%d] %s ...", d.proxy.ID, d.proxy.Name)
		err := client.DeleteProxy(d.proxy.ID)
		if err != nil {
			fmt.Printf(" %s %v\n", failIcon, err)
			deleteFail++
		} else {
			fmt.Printf(" %s\n", successIcon)
			deleteOK++
		}
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Printf("\n%s 全部完成: 解绑 %d 个账号，删除 %d/%d 个代理\n",
		successIcon, totalAccounts, deleteOK, len(toDelete))
}

// selectProxiesToDelete 手动多选代理
func selectProxiesToDelete(proxies []api.Proxy, accCountMap map[int64]int) ([]deleteItem, int) {
	options := make([]huh.Option[int], len(proxies))
	for i, p := range proxies {
		label := fmt.Sprintf("[ID:%d] %s (%s:%d) - %d 个账号", p.ID, p.Name, p.Host, p.Port, accCountMap[p.ID])
		options[i] = huh.NewOption(label, i)
	}

	var selected []int
	err := huh.NewMultiSelect[int]().
		Title("选择要删除的代理（空格切换选中，回车确认）").
		Options(options...).
		Value(&selected).
		Run()
	if err != nil || len(selected) == 0 {
		fmt.Println("已取消")
		return nil, 0
	}

	var result []deleteItem
	total := 0
	for _, idx := range selected {
		p := proxies[idx]
		cnt := accCountMap[p.ID]
		result = append(result, deleteItem{proxy: p, accCount: cnt})
		total += cnt
	}
	return result, total
}

// matchProxiesToDelete 按地址列表匹配代理
func matchProxiesToDelete(proxies []api.Proxy, accCountMap map[int64]int) ([]deleteItem, int) {
	// 选择输入方式
	inputMode, err := selectOne("数据来源", []string{
		"从文件读取",
		"手动输入（每行一个，空行结束）",
	})
	if err != nil {
		fmt.Println("已取消")
		return nil, 0
	}

	var lines []string
	if inputMode == 0 {
		entries, dirErr := os.ReadDir("data")
		if dirErr != nil {
			fmt.Printf("%s 读取 data 目录失败: %v\n", failIcon, dirErr)
			return nil, 0
		}
		var files []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".txt") {
				files = append(files, e.Name())
			}
		}
		if len(files) == 0 {
			fmt.Printf("%s data 目录下没有 .txt 文件\n", warnIcon)
			return nil, 0
		}
		fileIdx, err := selectOne("选择代理文件", files)
		if err != nil {
			fmt.Println("已取消")
			return nil, 0
		}
		lines = readLinesFromFile("data/" + files[fileIdx])
	} else {
		fmt.Println("请输入代理地址（格式: host:port，每行一个，空行结束）:")
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
		fmt.Printf("%s 未读取到任何地址\n", warnIcon)
		return nil, 0
	}

	// 解析输入地址为 host:port 集合
	targetSet := make(map[string]bool)
	for _, line := range lines {
		entry, parseErr := parseProxyLine(line)
		if parseErr != nil {
			continue
		}
		targetSet[fmt.Sprintf("%s:%d", entry.host, entry.port)] = true
	}

	// 匹配现有代理
	var result []deleteItem
	total := 0
	matched := 0
	for _, p := range proxies {
		key := fmt.Sprintf("%s:%d", p.Host, p.Port)
		if targetSet[key] {
			cnt := accCountMap[p.ID]
			result = append(result, deleteItem{proxy: p, accCount: cnt})
			total += cnt
			matched++
		}
	}

	notFound := len(targetSet) - matched
	if notFound > 0 {
		fmt.Printf("%s %d 条地址未匹配到现有代理（可能已删除）\n", warnIcon, notFound)
	}
	if len(result) == 0 {
		fmt.Printf("%s 没有匹配到任何代理\n", warnIcon)
		return nil, 0
	}

	fmt.Printf("%s 匹配到 %d 条代理\n", infoIcon, len(result))
	return result, total
}

// allProxiesToDelete 选择所有代理
func allProxiesToDelete(proxies []api.Proxy, accCountMap map[int64]int) ([]deleteItem, int) {
	var result []deleteItem
	total := 0
	for _, p := range proxies {
		cnt := accCountMap[p.ID]
		result = append(result, deleteItem{proxy: p, accCount: cnt})
		total += cnt
	}
	return result, total
}
