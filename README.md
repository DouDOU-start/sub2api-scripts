# sub2api-scripts

sub2api 账号代理批量管理工具，基于 TUI 交互界面。

## 配置

复制 `.env.example` 为 `.env`，填入你的 sub2api 服务信息：

```bash
cp .env.example .env
```

```env
SUB2API_URL=http://localhost:8080
SUB2API_KEY=你的管理员API Key
SUB2API_MODEL=claude-sonnet-4-6
```

配置优先级：命令行参数 > 环境变量 > `.env` 文件 > 默认值。

## 使用

```bash
# 编译
go build -o claude-admin ./cmd/claude-admin/

# 运行
./claude-admin
```

启动后进入 TUI 菜单，用方向键选择功能：

### 账号管理

| 功能 | 说明 |
|------|------|
| 查看所有账号 | 查看所有账号的状态、调度、代理信息 |
| 批量添加账号 | 从文件读取账号信息，批量认证、创建、测试，支持并发 |
| 协议状态扫描 | 扫描活跃账号，识别需要接受协议/认证失败等异常 |
| 批量恢复错误账号 | 测试 error/调度关闭的账号，成功则自动恢复调度 |
| 批量更新缓存 | 批量更新所有账号的缓存 TTL 配置 |
| 清理异常账号代理 | 检测异常账号连通性，解绑确认不可用的代理 |

### 代理管理

| 功能 | 说明 |
|------|------|
| 导入代理 | 从文件或手动输入批量导入代理（支持 `host:port` 和 `user:pass@host:port`） |
| 代理连通性检测 | 检测所有代理的连通性和延迟 |
| 删除代理 | 手动选择或按地址列表批量匹配删除，自动解绑关联账号 |
| 代理批量重命名 | 按 host/地址/前缀规则批量重命名代理 |
| 代理均衡分配 | 将超出上限的账号迁移到有空余的代理 |

## 账号文件格式

批量添加账号时使用，每行一个，`#` 开头为注释：

```
email----password----session_key
```

## 项目结构

```
├── cmd/claude-admin/       # TUI 主程序
├── internal/
│   ├── api/                # sub2api API 客户端和类型
│   └── config/             # .env 加载和配置管理
├── data/                   # 数据文件（账号、代理列表等）
├── .env.example            # 配置模板
└── go.mod
```
