# sub2api-scripts

sub2api 账号批量管理工具集。

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

## 脚本列表

### claude-batch-add — 批量添加 Claude OAuth 账号

从文件读取账号信息，批量完成认证、创建、测试。

```bash
# 从文件添加
go run cmd/claude-batch-add/main.go --input accounts.txt

# 从 stdin 输入（交互模式）
go run cmd/claude-batch-add/main.go
```

**账号文件格式**（每行一个，`#` 开头为注释）：

```
email----password----session_key
```

**启动流程：**
1. 交互选择代理（可选绑定）
2. 交互选择分组（可选，多选用逗号分隔）
3. 逐个处理账号：认证 → 创建 → 测试

**已存在账号处理：**
- 代理/分组/容量缺失 → 自动补充
- 已完整 → 跳过
- 测试失败 → 自动禁用并标记原因

**参数：**

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--input` | 账号文件路径 | stdin 输入 |
| `--output` | 结果导出文件 | `batch-add-result.txt` |
| `--proxy` | 代理 ID（-1=交互选择，0=不绑定） | -1 |
| `--api-url` | 服务地址 | `.env` 中的 `SUB2API_URL` |
| `--api-key` | API Key | `.env` 中的 `SUB2API_KEY` |
| `--model` | 测试用模型 | `.env` 中的 `SUB2API_MODEL` |

---

### claude-terms-scan — 批量扫描账号协议状态

扫描所有活跃的 Claude OAuth 账号，识别需要接受协议、认证失败等异常状态。

```bash
go run cmd/claude-terms-scan/main.go
```

**识别状态：**
- 正常
- 需要接受协议（需在 claude.ai 手动接受）
- 认证失败
- 速率限制
- 服务过载

**参数：**

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--api-url` | 服务地址 | `.env` 中的 `SUB2API_URL` |
| `--api-key` | API Key | `.env` 中的 `SUB2API_KEY` |
| `--model` | 测试用模型 | `.env` 中的 `SUB2API_MODEL` |

## 项目结构

```
├── cmd/
│   ├── claude-batch-add/    # 批量添加脚本
│   └── claude-terms-scan/   # 协议扫描脚本
├── internal/
│   ├── api/                 # sub2api API 客户端和类型
│   ├── config/              # .env 加载和配置管理
│   └── interactive/         # 交互式选择（代理、分组）
├── .env.example             # 配置模板
└── go.mod
```
