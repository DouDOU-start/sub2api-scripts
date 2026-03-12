// Claude Admin - 账号代理管理 TUI 工具
// 聚合所有管理脚本为统一的终端交互界面
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"sub2api-scripts/internal/api"
	"sub2api-scripts/internal/config"
)

func main() {
	config.LoadEnvFile()

	apiURL := os.Getenv("SUB2API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	apiKey := os.Getenv("SUB2API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "错误: 未找到 API Key，请设置环境变量 SUB2API_KEY 或在 .env 文件中配置")
		os.Exit(1)
	}
	model := os.Getenv("SUB2API_MODEL")

	client := api.NewClient(apiURL, apiKey)

	for {
		// 每轮启动一个全新的 bubbletea 程序显示主菜单
		m := newMenuModel(client, model)
		p := tea.NewProgram(m, tea.WithAltScreen())
		result, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
			os.Exit(1)
		}

		final := result.(menuModel)
		if final.quitting {
			// 用户按 q 退出
			return
		}

		// 用户选择了某个功能，bubbletea 已完全退出，终端恢复正常
		if final.selected != nil {
			final.selected.run(client, model)
			fmt.Println("\n按回车键返回主菜单...")
			fmt.Scanln()
		}
	}
}
