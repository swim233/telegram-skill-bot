# Tasker - Telegram 群聊 AI 助手

又一个好玩的 telegram bot ！

拥有 总结群聊 问答 和聚焦搜索的功能 
快速销毁 token 额度（bushi）

一些示例命令:

```
/skill 帮我用 go 写一个hello world
```
<img width="1912" height="441" alt="image" src="https://github.com/user-attachments/assets/0386f5bb-1f7b-42e9-923c-a6dadfac6bf5" />

</br>

```
/summary 1h # 总结一个小时的聊天记录
```
<img width="1913" height="831" alt="image" src="https://github.com/user-attachments/assets/cd74e0f1-b8f0-4f2a-85e1-71dfa6d4c647" />

</br>

```
/focus 1000 查找旅行有关的内容
```
<img width="1831" height="383" alt="image" src="https://github.com/user-attachments/assets/f5d1b179-53f8-4c17-a0b6-f1cb220db545" />


## 功能

| 命令 | 说明 |
|------|------|
| `/skill <prompt>` | AI 问答（支持图片、文件附件） |
| `/summary [duration]` | AI 群聊总结（默认 24h） |
| `/focus <duration\|date\|条数> <content>` | 聚焦分析指定范围的聊天记录 |
| `/del <duration>` | 定时删除消息 |
| `/reply <duration> <content>` | 定时回复消息 |
| `/cancel [ai\|task]` | 取消进行中的任务（回复目标消息） |
| `/switch models <skill\|summary> <model_id>` | 切换 AI 模型 |
| `/switch api <skill\|summary> <api_name> [token_index]` | 切换 API |
| `/approve <command\|all> [user_id]` | 授权用户使用命令 |
| `/revoke [command] [user_id]` | 撤销授权 |
| `/list skill\|summary\|api\|perm` | 查看配置 |
| `/help` | 显示帮助 |

## 快速开始

### 1. 配置

复制配置文件并填写：

```bash
cp config/config.yaml.example config/config.yaml
```

```yaml
BOT:
  bot_token: "<BOT_TOKEN>"
  owner_id: <OWNER_ID> # bot 所有者 telegram id
  allow_list:
    - <OWNER_ID>
    - <GROUP_ID> # 此处需填写启用 bot 的群 id 否则无法使用命令

  # 注释掉的内容为默认不允许使用的指令 需要 /approve 授权
  allow_commands:
    - help
    - del
    - reply
    - cancel
    # - skill
    # - summary
    # - focus
    # - switch
    # - list_api
API:
  skill_module: "claude-opus-4-6"
  summary_module: "deepseek-chat"
  summary_api: "your_api"
  summary_token: 0
  skill_api: "your_api"
  skill_token: 0
  apis:
    - name: your_api
      endpoint: https://api.example.com/v1
      request_type: responses  # responses 或 completions
      tokens:
        - tag: main
          token: "sk-xxx"
DATA:
  sqlite_path: "./data/chat_messages.db"
```

### 2. 构建

```bash
make build    # 构建并压缩
make run      # 开发模式运行
make test     # 运行测试
```

### 3. 部署

```bash
make push     # 构建并上传到服务器
```

## 权限系统

- **owner**：`config.yaml` 中的 `owner_id`，拥有所有权限
- **allow_list**：允许使用 bot 的群组/私聊 ID
- **approve**：owner 可通过 `/approve` 授权其他用户使用特定命令
  - 可授权命令：`summary`, `skill`, `switch`, `list_api`, `all`
  - 权限数据存储在 `config/permissions.json`

## 架构

```
src/
├── main.go                      # 入口、命令注册
├── api/
│   ├── api_handler.go           # AI 请求（OpenAI SDK）
│   └── api_constance.go         # 初始化、模型列表
├── bot/
│   └── bot.go                   # Telegram Bot 初始化
├── config/
│   ├── config.go                # Viper 配置加载
│   └── permissions.go           # 权限管理
├── data/
│   └── database.go              # SQLite 消息存储
├── internal/bot/
│   ├── api/api_manager.go       # /switch、/list api
│   └── task/task_manager.go     # 异步任务、取消
├── lib/
│   └── types.go                 # 配置结构体
└── utils/
    └── html.go                  # Markdown → Telegram HTML
```

## API 配置说明

每个场景（skill/summary）独立配置 API 和模型：

- `skill_api` / `skill_token` / `skill_module`：问答场景
- `summary_api` / `summary_token` / `summary_module`：总结场景

支持的 `request_type`：
- `responses`：OpenAI Responses API
- `completions`：Chat Completions API

## License

MIT
