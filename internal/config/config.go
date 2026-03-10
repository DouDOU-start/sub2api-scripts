// 配置加载：.env 文件 + 环境变量 + 命令行参数
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadEnvFile 从当前目录向上查找 .env 文件，加载到环境变量（不覆盖已有）
func LoadEnvFile() {
	dir, _ := os.Getwd()
	for {
		envPath := filepath.Join(dir, ".env")
		if f, err := os.Open(envPath); err == nil {
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if k, v, ok := strings.Cut(line, "="); ok {
					k = strings.TrimSpace(k)
					v = strings.TrimSpace(v)
					if os.Getenv(k) == "" {
						os.Setenv(k, v)
					}
				}
			}
			f.Close()
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

// Get 按优先级获取配置: 命令行参数 > 环境变量 > 默认值
func Get(flagVal, envKey, defaultVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return defaultVal
}
