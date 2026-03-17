package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sub2api-scripts/internal/api"
)

// 菜单项
type menuItem struct {
	name string
	desc string
	run  func(client *api.Client, model string)
}

// 菜单分组
type menuGroup struct {
	title string
	items []menuItem
}

type menuModel struct {
	client   *api.Client
	model    string
	groups   []menuGroup
	cursor   int
	allItems []menuItem
	selected *menuItem // 选中的菜单项，退出后由 main 执行
	quitting bool
}

func newMenuModel(client *api.Client, model string) menuModel {
	groups := []menuGroup{
		{
			title: "账号管理",
			items: []menuItem{
				{name: "查看所有账号", desc: "查看所有账号的状态、调度、代理信息", run: runAccountList},
				{name: "批量添加账号", desc: "从文件读取账号信息，批量添加到 sub2api", run: runAccountAdd},
				{name: "协议状态扫描", desc: "扫描所有账号，识别需要接受协议的账号", run: runAccountScan},
				{name: "批量更新缓存", desc: "批量更新所有账号的缓存 TTL 配置", run: runAccountCache},
				{name: "批量恢复错误账号", desc: "测试 error/调度关闭的账号，成功则自动恢复", run: runAccountRecover},
			{name: "清理异常账号代理", desc: "检测异常账号连通性，解绑确认不可用的代理", run: runAccountCleanup},
				{name: "批量重新授权", desc: "用 SK 文件重新授权已有账号，更新 Token（不新建账号）", run: runAccountReauth},
				{name: "批量删除异常账号", desc: "删除所有 error/调度关闭的异常账号", run: runAccountDeleteError},
			},
		},
		{
			title: "代理管理",
			items: []menuItem{
				{name: "导入代理", desc: "从文件或手动输入批量导入代理地址", run: runProxyImport},
				{name: "代理连通性检测", desc: "检测所有代理的连通性和延迟", run: runProxyCheck},
				{name: "删除代理", desc: "选择并删除代理，自动解绑关联账号", run: runProxyDelete},
				{name: "代理批量重命名", desc: "按 host/地址/前缀规则批量重命名代理", run: runProxyRename},
				{name: "代理均衡分配", desc: "将超出上限的账号迁移到有空余的代理", run: runProxyRebalance},
				{name: "导出代理 IP", desc: "导出所有代理的完整地址到文件", run: runProxyExportIP},
				{name: "解绑所有代理并删除", desc: "解绑所有账号的代理绑定，然后删除全部代理", run: runAccountUnbindAndDeleteProxies},
			},
		},
	}

	var all []menuItem
	for _, g := range groups {
		all = append(all, g.items...)
	}

	return menuModel{
		client:   client,
		model:    model,
		groups:   groups,
		allItems: all,
	}
}

func (m menuModel) Init() tea.Cmd {
	return nil
}

func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.allItems)-1 {
				m.cursor++
			}
		case "enter":
			// 记录选中项，退出 bubbletea，由 main 循环执行
			item := m.allItems[m.cursor]
			m.selected = &item
			return m, tea.Quit
		}
	}
	return m, nil
}

// 样式
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170")).
			MarginBottom(1)

	groupStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Bold(true).
			MarginTop(1)

	itemStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	selectedStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(lipgloss.Color("170")).
			Bold(true)

	descStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)
)

func (m menuModel) View() string {
	if m.quitting || m.selected != nil {
		return ""
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render("  Claude Admin - 账号代理管理"))
	b.WriteString("\n")

	idx := 0
	for _, g := range m.groups {
		b.WriteString(groupStyle.Render("  " + g.title))
		b.WriteString("\n")

		for _, item := range g.items {
			cursor := "  "
			style := itemStyle
			if idx == m.cursor {
				cursor = "▸ "
				style = selectedStyle
			}

			line := style.Render(cursor + item.name)
			if idx == m.cursor {
				line += " " + descStyle.Render(item.desc)
			}
			b.WriteString(line)
			b.WriteString("\n")
			idx++
		}
	}

	b.WriteString(helpStyle.Render("  ↑/↓ 移动  enter 选择  q 退出"))

	return b.String()
}
