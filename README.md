# feishu-bot-cli

飞书 / Lark 机器人一键创建工具，无需手动去开放平台点击创建，扫码即完成 PersonalAgent 机器人的注册，自动获取 `app_id` 和 `app_secret`。

## 原理

调用飞书隐藏端点 `POST https://accounts.feishu.cn/oauth/v1/app/registration`，通过 **init → begin → poll** 三步完成设备注册流程（类似 OAuth Device Authorization Grant）：

1. `init` — 检查环境是否支持 `client_secret` 认证
2. `begin` — 发起注册，获取扫码 URL 和 `device_code`
3. `poll` — 轮询等待用户扫码授权，成功后返回 `client_id` / `client_secret`

用户扫码后飞书会自动预配一个 PersonalAgent 类型的应用，通常包括权限和事件订阅。

## 安装

```bash
# 一键安装（推荐）
go install github.com/AngusFu/feishu-bot-cli@latest

# 或自行编译
go build -o feishu-bot-cli ./cmd/
```

## 用法

### 新建机器人

```bash
# 默认飞书（国内版），终端显示二维码 + URL 链接
feishu-bot-cli new

# Lark 国际版
feishu-bot-cli new --platform lark

# 保存二维码图片 + base64 输出
feishu-bot-cli new --output-qr-image qr.png --output-qr-base64

# 自定义超时时间（秒）
feishu-bot-cli new --timeout 300

# Debug 模式
feishu-bot-cli new --debug
```

### 验证凭证

```bash
# 自动检测 feishu / lark
feishu-bot-cli verify cli_xxxx sec_xxxx

# 指定平台
feishu-bot-cli verify --platform lark cli_xxxx sec_xxxx
```

## 选项

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `--platform` | 平台类型：`feishu` / `lark` | `feishu` |
| `--timeout` | 扫码授权超时时间（秒） | `600` |
| `--output-qr-image` | 保存二维码为 PNG 图片的路径 | - |
| `--output-qr-base64` | 同时输出 base64 data URI 格式 | `false` |
| `--debug` | 打印调试日志 | `false` |

## 输出

```
🎉 机器人创建成功！
   Platform:  feishu
   App ID:    cli_xxxxxxxxxxxxxxxx
   App Secret: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
   Owner:     ou_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

💡 下一步：
   1. 前往 https://open.feishu.cn/app 查看应用详情
   2. 在开放平台检查：权限状态、事件订阅、可用范围
   3. 发布应用版本（如有需要）
```

## 注意事项

- 需要飞书 / Lark 手机 App 扫码授权
- 创建的机器人类型为 PersonalAgent
- 扫码新建后建议在开放平台确认应用已发布、权限正常
- Lark 国际版会自动识别并切换 API 地址
