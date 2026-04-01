<div align="center">

# ClawDesk

**一体化 AI 桌面应用**

[English](README.md) | [中文](README.cn.md)

</div>

---

## 概述

ClawDesk 是一个一体化 AI 桌面应用，基于 Go + Vue 3 构建。集成多模型聊天、多 Agent 协作、Agent 技能（SKILL.md + MCP）、会话记忆（向量检索 + 上下文压缩）、定时任务、操作审计等能力，开箱即用。嵌入模型和 ONNX Runtime 首次启动时自动下载，无需手动安装。

## 功能特性

### 聊天与助手

- **助手系统** — 每个会话是一个可配置的助手，支持 emoji 头像、名称、描述、系统提示词、绑定模型（厂商+模型）。名称为空时自动生成英文名。LLM 可通过 `create_bot` 工具自动创建新助手。
- **多模型聊天** — 支持 OpenAI、Anthropic、DeepSeek、阿里云百炼及任何 OpenAI 兼容接口。流式响应，实时显示工具调用状态。
- **文件上传/下载** — 支持文本文件（内容注入 prompt）和图片（base64 Vision API）附件。代码块带复制/下载按钮。
- **输入历史** — 类似 shell 的上下键导航，最多保留 50 条。

### 技能与工具

- **Agent 技能系统** — 从 ClawHub 或 SkillHub（腾讯）安装技能。技能是 SKILL.md 知识文档，教 AI 如何使用 shell 命令完成特定任务。
- **MCP 协议** — 连接 MCP（Model Context Protocol）服务器，支持 stdio/SSE 传输，通过 `tools/list` 自动发现并注册工具。
- **Function Calling 循环** — SkillManager 向 LLM 提供所有已启用的工具定义，工具调用自动分发执行、结果回传。每次请求最多循环 10 次。
- **内置系统工具** — `execute_command`、`read_file`、`write_file`、`list_directory`、`fetch_url`、`create_bot`、`plan_and_execute` 等。

### 多 Agent 协作

- **自动路由** — LLM 自主判断简单问题（单 Agent）或复杂问题（多 Agent 编排）。
- **并行执行** — 无依赖步骤通过 goroutine 并行执行，有依赖步骤等待前置结果注入。
- **执行追踪** — 查看计划摘要、步骤详情（状态/角色/工具调用/耗时），自动生成 Mermaid 流程图。

### 会话记忆

- **向量检索** — 每条消息本地嵌入（multilingual-e5-small ONNX，支持 100+ 语言，384 维）。每次请求前检索相似历史。
- **上下文压缩** — 对话超过阈值时，LLM 增量总结旧消息。摘要注入系统提示词。
- **自动下载** — 嵌入模型（~113MB）和 ONNX Runtime 库首次启动时自动下载。下载期间应用可正常使用（记忆功能就绪后自动激活）。状态栏显示下载进度。

### 定时任务

- 为每个助手创建周期性任务（固定间隔或每天定时）。
- 结果可通过企业微信或飞书 Webhook 推送，或保存到会话历史。

### 审计与调试

- **技能审计** — 所有工具调用自动记录：时间、助手、工具名、参数、结果、成功/失败、耗时。
- **存储审计** — 所有持久化操作自动记录。
- **请求日志** — 查看每个会话的完整 LLM 请求详情：完整组装的系统提示词和 Function Calling 工具定义（含参数）。每个会话保留最近 20 条。

### 界面与体验

- **10 种语言** — 中文、英文、日文、韩文、法文、德文、西班牙文、俄文、葡萄牙文、阿拉伯文。
- **深色/浅色主题** — TDesign 原生主题切换。
- **Token 用量** — 按厂商/模型分类的用量统计面板。
- **状态栏** — 实时显示 CPU 使用率、内存占用、应用数据目录大小、向量模型就绪状态。

## 技术栈

| 层 | 技术 |
|---|------|
| 桌面框架 | [Wails 2.11](https://wails.io/)（Go + Web） |
| 前端 | Vue 3 + TypeScript + TDesign Vue Next |
| 聊天组件 | @tdesign-vue-next/chat |
| Markdown | marked + mermaid |
| 本地推理 | [ONNX Runtime](https://onnxruntime.ai/)（首次启动自动下载，支持 6 个平台） |
| 嵌入模型 | multilingual-e5-small INT8 ONNX（113MB，384 维，自动下载） |
| 分词器 | 纯 Go SentencePiece Unigram（零原生依赖） |
| 向量数据库 | SQLite（modernc.org/sqlite，纯 Go，WAL 模式） |
| 浏览器自动化 | playwright-go |
| 国际化 | vue-i18n |

## 环境要求

- [Go 1.25+](https://go.dev/)
- [Node.js 18+](https://nodejs.org/)
- [Wails CLI](https://wails.io/docs/gettingstarted/installation)

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

## 支持平台

| 平台 | 架构 |
|------|------|
| macOS | arm64（Apple Silicon）、amd64（Intel） |
| Windows | amd64（x64）、arm64 |
| Linux | amd64（x64）、arm64 |

## 快速开始

```bash
# 克隆项目
git clone https://github.com/user/clawdesk.git
cd clawdesk

# 开发模式
make dev

# 构建
make build

# 运行测试
make test

# 预下载嵌入资源到本地缓存（可选，应用首次启动也会自动下载）
make setup-cache
```

> 首次启动时，应用会自动下载嵌入模型和 ONNX Runtime（共约 163MB）。下载期间应用可正常使用，向量记忆功能在下载完成后自动激活。

## 构建打包

```bash
# 全平台构建
./build.sh 1.0.0

# 指定目标
./build.sh 1.0.0 macos-arm64      # macOS Apple Silicon → DMG
./build.sh 1.0.0 macos-amd64      # macOS Intel → DMG
./build.sh 1.0.0 windows-arm64    # Windows ARM64 → Setup.exe
./build.sh 1.0.0 windows-amd64    # Windows x64 → Setup.exe
./build.sh 1.0.0 linux-arm64      # Linux ARM64 → .deb
./build.sh 1.0.0 linux-amd64      # Linux x64 → .deb
```

> 从 macOS 交叉编译 Linux 需要 Docker。Windows 安装包需要 NSIS（`brew install nsis`）或 Docker。

## 数据目录

```
~/.clawdesk/
  config.yaml              # 模型厂商配置
  vectors.db               # SQLite 向量数据库
  usage.json               # Token 用量记录
  audit/audit.db           # 审计数据库
  sessions/{id}/
    meta.json              # 助手元数据
    YYYYMMDD.jsonl          # 每日消息
    summary.json            # 压缩上下文摘要
    request_logs.json       # LLM 请求日志（最近 20 条）
    workspace/              # 会话工作目录
  skills/{name}/
    SKILL.md               # 技能定义
    _meta.json             # 技能元数据
  cache/
    ort/                   # ONNX Runtime + 嵌入模型（首次启动自动下载）
      multilingual-e5-small-quantized.onnx
      tokenizer.json
      libonnxruntime.dylib (或 .so / .dll)
      .ready
```

## 项目结构

```
main.go                              # 入口文件（Wails OnStartup/OnShutdown 生命周期）
wails.json                           # Wails 项目配置
Makefile                             # 开发辅助：make dev、make test、make setup-cache
build.sh                             # 跨平台构建脚本

src/
  agent/
    app.go                           # 主应用：前端 API、SendMessage、编排器集成
    llm.go                           # LLM 请求：流式、Function Calling 循环、工具分发
    session.go                       # 聊天会话 CRUD、默认系统提示词
    orchestrator.go                  # 多 Agent 编排：规划、并行执行、汇总、Mermaid 流程图
    scheduler.go                     # 定时任务：固定间隔/每日定时、执行、通知
    notify.go                        # 通知推送（企业微信/飞书 Webhook）
    usage.go                         # Token 用量按厂商/模型统计
    sysinfo.go                       # 系统资源监控（CPU、内存、协程数）
    tools.go                         # FileInfo 类型、附件处理
    const.go                         # 事件名常量（llm:start、llm:done 等）
  config/
    config.go                        # 模型厂商配置（~/.clawdesk/config.yaml）
  skill/
    skill.go                         # SkillManager：CRUD、加载、SKILL.md 解析、工具定义
    builtin.go                       # 内置系统工具（execute_command、read_file 等）
    executor.go                      # 命令模板执行器（ToolResult、exit code 判断）
    mcp.go                           # MCP 客户端（stdio/SSE、JSON-RPC 2.0）
    browser.go                       # 浏览器自动化（playwright-go 封装）
    clawhub.go                       # ClawHub 集成（搜索 + 下载 + 安装）
    skillhub.go                      # SkillHub 集成（腾讯，API + CLI）
  memory/
    memory.go                        # MemoryManager：BuildContext、StoreMessage、压缩触发
    store.go                         # DailyStore：JSONL 消息存储、会话元数据、请求日志
    vectordb.go                      # 向量数据库：按会话分表 SQLite、余弦相似度搜索
    embedding.go                     # Embedder：文本 → 向量（ONNX Runtime，token 上限保护）
    tokenizer.go                     # 纯 Go SentencePiece Unigram 分词器（解析 tokenizer.json）
    assets_download.go               # 运行时资源下载（模型 + 分词器 + ONNX Runtime）
    ort_darwin_arm64.go              # 平台特定 ONNX Runtime 下载配置（共 6 个平台文件）
    compress.go                      # Compressor：增量 LLM 摘要压缩
  audit/
    audit.go                         # 审计数据库：技能调用 + 存储操作
  channels/
    channels.go                      # 渠道管理：聊天绑定到消息平台
    feishu.go                        # 飞书机器人适配器
    wecom.go                         # 企业微信机器人适配器
    dingtalk.go                      # 钉钉机器人适配器

frontend/
  src/
    main.ts                          # 前端入口
    App.vue                          # 根组件
    router/index.ts                  # Vue Router 路由配置
    i18n/                            # 10 种语言（中/英/日/韩/法/德/西/俄/葡/阿）
    views/
      layout/index.vue               # 主布局：侧边菜单、标题栏、状态栏
      chat/index.vue                 # 聊天页面：会话列表、消息区、输入、流式生成
      model/index.vue                # 模型厂商配置
      skill/index.vue                # 技能管理（搜索安装、MCP 配置）
      channel/index.vue              # 渠道配置（飞书/企微/钉钉）
      usage/index.vue                # Token 用量统计面板
      audit/index.vue                # 技能审计（工具调用记录）
      audit/storage.vue              # 存储审计（持久化操作记录）
      audit/prompt.vue               # 请求日志（系统提示词 + Function Calling 查看器）
  wailsjs/                           # Wails 自动生成的前端绑定
```

## 许可证

MIT
