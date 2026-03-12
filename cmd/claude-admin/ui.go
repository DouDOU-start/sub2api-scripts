package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// 通用样式
var (
	successIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓")
	failIcon    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("✗")
	warnIcon    = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("!")
	infoIcon    = lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Render("ℹ")

	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("170"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// printHeader 打印带样式的标题
func printHeader(title string) {
	fmt.Println()
	fmt.Println(headerStyle.Render("═══ " + title + " ═══"))
	fmt.Println()
}

// printSeparator 打印分隔线
func printSeparator(width int) {
	fmt.Println(dimStyle.Render(strings.Repeat("─", width)))
}

// confirm 确认对话框
func confirm(msg string) bool {
	var ok bool
	err := huh.NewConfirm().
		Title(msg).
		Affirmative("确认").
		Negative("取消").
		Value(&ok).
		Run()
	if err != nil {
		return false
	}
	return ok
}

// inputText 文本输入
func inputText(title, placeholder, defaultVal string) (string, error) {
	var val string
	if defaultVal != "" {
		val = defaultVal
	}
	err := huh.NewInput().
		Title(title).
		Placeholder(placeholder).
		Value(&val).
		Run()
	return val, err
}

// selectOne 单选列表
func selectOne(title string, options []string) (int, error) {
	opts := make([]huh.Option[int], len(options))
	for i, o := range options {
		opts[i] = huh.NewOption(o, i)
	}
	var selected int
	err := huh.NewSelect[int]().
		Title(title).
		Options(opts...).
		Value(&selected).
		Run()
	return selected, err
}

// selectMulti 多选列表
func selectMulti(title string, options []string) ([]int, error) {
	opts := make([]huh.Option[int], len(options))
	for i, o := range options {
		opts[i] = huh.NewOption(o, i)
	}
	var selected []int
	err := huh.NewMultiSelect[int]().
		Title(title).
		Options(opts...).
		Value(&selected).
		Run()
	return selected, err
}

// progress 打印进度
func progress(current, total int, msg string) {
	fmt.Printf("[%d/%d] %s", current, total, msg)
}
