// 交互式选择：代理、分组
package interactive

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"sub2api-scripts/internal/api"
)

// SelectProxy 交互选择代理，返回选中的代理 ID（nil 表示不绑定）
func SelectProxy(proxies []api.Proxy) *int64 {
	if len(proxies) == 0 {
		fmt.Println("没有可用的代理，将不绑定代理")
		return nil
	}

	fmt.Println("可用代理列表:")
	fmt.Println("  0. 不绑定代理")
	for i, p := range proxies {
		fmt.Printf("  %d. [ID:%d] %s (%s)\n", i+1, p.ID, p.Name, p.Address)
	}
	fmt.Print("请选择代理编号: ")

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		var choice int
		if _, err := fmt.Sscanf(scanner.Text(), "%d", &choice); err == nil && choice >= 1 && choice <= len(proxies) {
			id := proxies[choice-1].ID
			fmt.Printf("已选择代理: %s (ID: %d)\n", proxies[choice-1].Name, id)
			return &id
		}
	}
	return nil
}

// SelectGroups 交互选择分组（多选），返回选中的分组 ID 列表
func SelectGroups(groups []api.Group) []int64 {
	if len(groups) == 0 {
		return nil
	}

	fmt.Println("\n可用分组列表（多选用逗号分隔，如 1,3；直接回车跳过）:")
	for i, g := range groups {
		fmt.Printf("  %d. [ID:%d] %s (%s)\n", i+1, g.ID, g.Name, g.Platform)
	}
	fmt.Print("请选择分组编号: ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return nil
	}

	text := strings.TrimSpace(scanner.Text())
	if text == "" {
		return nil
	}

	var selected []int64
	for _, part := range strings.Split(text, ",") {
		var choice int
		if _, err := fmt.Sscanf(strings.TrimSpace(part), "%d", &choice); err == nil && choice >= 1 && choice <= len(groups) {
			selected = append(selected, groups[choice-1].ID)
		}
	}

	if len(selected) > 0 {
		names := make([]string, len(selected))
		for i, id := range selected {
			for _, g := range groups {
				if g.ID == id {
					names[i] = g.Name
					break
				}
			}
		}
		fmt.Printf("已选择分组: %s\n", strings.Join(names, ", "))
	}
	return selected
}
