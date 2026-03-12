APP = claude-admin
CMD = ./cmd/claude-admin

.PHONY: dev build clean

# 直接运行（不生成二进制）
dev:
	go run $(CMD)

# 编译到项目根目录
build:
	go build -o $(APP) $(CMD)

# 清理编译产物
clean:
	rm -f $(APP)
