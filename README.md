<div align="center">

# ClawDesk

**All-in-one AI Desktop Application**

[English](README.md) | [中文](README.cn.md)

</div>

---

## Overview

ClawDesk is an all-in-one AI desktop application built with Go + Vue 3. It integrates multi-model chat, multi-agent collaboration, agent skills (SKILL.md + MCP), session memory (vector retrieval + context compression), scheduled tasks, and operation auditing — all in a single, ready-to-use desktop app. The embedding model and ONNX Runtime are automatically downloaded on first launch — no manual setup required.

## Features

### Chat & Assistants

- **Bot System** — Each session is a configurable assistant with emoji avatar, name, description, system prompt, and bound model (provider + model). Auto-generates English names when empty. LLM can create new bots via `create_bot` tool.
- **Multi-Model Chat** — Support for OpenAI, Anthropic, DeepSeek, Alibaba Qwen, and any OpenAI-compatible API provider. Stream responses with real-time tool call status.
- **File Upload/Download** — Attach text files (content injected into prompt) and images (base64 Vision API). Code blocks with copy/download buttons.
- **Input History** — Shell-like up/down arrow key navigation, up to 50 entries.

### Skills & Tools

- **Agent Skill System** — Install skills from ClawHub or SkillHub (Tencent). Skills are SKILL.md documents that teach the AI how to use shell commands for specific tasks.
- **MCP Protocol** — Connect to MCP (Model Context Protocol) servers via stdio or SSE transport. Auto-discover and register tools via `tools/list`.
- **Function Calling Loop** — SkillManager provides all enabled tool definitions to LLM. Tool calls are dispatched, executed, results fed back. Max 10 iterations per request.
- **Built-in System Tools** — `execute_command`, `read_file`, `write_file`, `list_directory`, `fetch_url`, `create_bot`, `plan_and_execute`, etc.

### Multi-Agent Collaboration

- **Auto Routing** — LLM decides simple (single agent) vs complex (multi-agent orchestration).
- **Parallel Execution** — Independent steps run in goroutines. Dependent steps wait and inject prior results.
- **Execution Trace** — View plan summary, step details (status/role/tool calls/duration), and auto-generated Mermaid flow diagram.

### Memory

- **Vector Retrieval** — Every message is embedded locally (multilingual-e5-small ONNX, 100+ languages, 384 dimensions). Similar history retrieved before each request.
- **Context Compression** — Incremental LLM summarization when conversation exceeds threshold. Summary injected into system prompt.
- **Auto-Download** — Embedding model (~113MB) and ONNX Runtime library are automatically downloaded on first launch. App works normally during download (memory features activate once ready). Status bar shows download progress.

### Scheduled Tasks

- Create recurring tasks per assistant (fixed interval or daily at specific time).
- Results delivered via WeCom or Feishu webhooks, or saved to session history.

### Audit & Debug

- **Skill Audit** — All tool calls recorded: timestamp, bot, tool, arguments, result, success/failure, duration.
- **Storage Audit** — All persistence operations recorded.
- **Request Log** — View complete LLM request details per session: full assembled system prompt and function calling tool definitions (with parameters). Last 20 requests saved per session.

### UI & UX

- **10 Languages** — Chinese, English, Japanese, Korean, French, German, Spanish, Russian, Portuguese, Arabic.
- **Dark/Light Theme** — TDesign native theme switching.
- **Token Usage** — Per-provider/model usage tracking dashboard.
- **Status Bar** — Real-time CPU, memory, data directory size, embedding model status.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Desktop Framework | [Wails 2.11](https://wails.io/) (Go + Web) |
| Frontend | Vue 3 + TypeScript + TDesign Vue Next |
| Chat UI | @tdesign-vue-next/chat |
| Markdown | marked + mermaid |
| Local Inference | [ONNX Runtime](https://onnxruntime.ai/) (auto-downloaded, 6 platforms) |
| Embedding Model | multilingual-e5-small INT8 ONNX (113MB, 384d, auto-downloaded) |
| Tokenizer | Pure Go SentencePiece Unigram (zero native dependencies) |
| Vector DB | SQLite (modernc.org/sqlite, pure Go, WAL mode) |
| Browser Automation | playwright-go |
| i18n | vue-i18n |

## Prerequisites

- [Go 1.25+](https://go.dev/)
- [Node.js 18+](https://nodejs.org/)
- [Wails CLI](https://wails.io/docs/gettingstarted/installation)

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

## Supported Platforms

| Platform | Architecture |
|----------|-------------|
| macOS | arm64 (Apple Silicon), amd64 (Intel) |
| Windows | amd64 (x64), arm64 |
| Linux | amd64 (x64), arm64 |

## Quick Start

```bash
# Clone
git clone https://github.com/user/clawdesk.git
cd clawdesk

# Development
make dev

# Build
make build

# Run tests
make test

# Pre-download embedding assets to local cache (optional, app also auto-downloads on first launch)
make setup-cache
```

> On first launch, the app downloads the embedding model and ONNX Runtime (~163MB total). The app is fully usable during the download — vector memory features activate automatically once complete.

## Build & Package

```bash
# Build all platforms
./build.sh 1.0.0

# Build specific target
./build.sh 1.0.0 macos-arm64      # macOS Apple Silicon → DMG
./build.sh 1.0.0 macos-amd64      # macOS Intel → DMG
./build.sh 1.0.0 windows-arm64    # Windows ARM64 → Setup.exe
./build.sh 1.0.0 windows-amd64    # Windows x64 → Setup.exe
./build.sh 1.0.0 linux-arm64      # Linux ARM64 → .deb
./build.sh 1.0.0 linux-amd64      # Linux x64 → .deb
```

> Linux builds require Docker when cross-compiling from macOS. Windows setup.exe requires NSIS (`brew install nsis`) or Docker.

## Data Directory

```
~/.clawdesk/
  config.yaml              # Model provider configuration
  vectors.db               # SQLite vector database
  usage.json               # Token usage records
  audit/audit.db           # Audit database
  sessions/{id}/
    meta.json              # Bot metadata
    YYYYMMDD.jsonl          # Daily messages
    summary.json            # Compressed context summary
    request_logs.json       # LLM request logs (last 20)
    workspace/              # Session working directory
  skills/{name}/
    SKILL.md               # Skill definition
    _meta.json             # Skill metadata
  cache/
    ort/                   # ONNX Runtime + embedding model (auto-downloaded on first launch)
      multilingual-e5-small-quantized.onnx
      tokenizer.json
      libonnxruntime.dylib (or .so / .dll)
      .ready
```

## Project Structure

```
main.go                              # Entry point (Wails OnStartup/OnShutdown lifecycle)
wails.json                           # Wails project configuration
Makefile                             # Dev helpers: make dev, make test, make setup-cache
build.sh                             # Cross-platform build script

src/
  agent/
    app.go                           # Main app: frontend API, SendMessage, orchestrator integration
    llm.go                           # LLM requests: streaming, function calling loop, tool dispatch
    session.go                       # Chat session CRUD, default system prompt
    orchestrator.go                  # Multi-agent orchestration: plan, parallel exec, summarize, Mermaid
    scheduler.go                     # Scheduled tasks: interval/daily, execution, notify
    notify.go                        # Notification delivery (WeCom/Feishu webhooks)
    usage.go                         # Token usage tracking per provider/model
    sysinfo.go                       # System resource monitoring (CPU, memory, goroutines)
    tools.go                         # FileInfo type, attachment handling
    const.go                         # Event name constants (llm:start, llm:done, etc.)
  config/
    config.go                        # Model provider config (~/.clawdesk/config.yaml)
  skill/
    skill.go                         # SkillManager: CRUD, loading, SKILL.md parsing, tool definitions
    builtin.go                       # Built-in system tools (execute_command, read_file, etc.)
    executor.go                      # Command template executor (ToolResult, exit code)
    mcp.go                           # MCP client (stdio/SSE, JSON-RPC 2.0)
    browser.go                       # Browser automation (playwright-go wrapper)
    clawhub.go                       # ClawHub integration (search + download + install)
    skillhub.go                      # SkillHub integration (Tencent, API + CLI)
  memory/
    memory.go                        # MemoryManager: BuildContext, StoreMessage, compression trigger
    store.go                         # DailyStore: JSONL message storage, session metadata, request logs
    vectordb.go                      # Vector DB: per-session SQLite tables, cosine similarity search
    embedding.go                     # Embedder: text → vector via ONNX Runtime (token limit protection)
    tokenizer.go                     # Pure Go SentencePiece Unigram tokenizer (reads tokenizer.json)
    assets_download.go               # Runtime asset download (model + tokenizer + ONNX Runtime)
    ort_darwin_arm64.go              # Platform-specific ONNX Runtime download config (6 platforms)
    compress.go                      # Compressor: incremental LLM summarization
  audit/
    audit.go                         # Audit DB: skill calls + storage operations
  channels/
    channels.go                      # Channel manager: bind chat to messaging platforms
    feishu.go                        # Feishu bot adapter
    wecom.go                         # WeCom bot adapter
    dingtalk.go                      # DingTalk bot adapter

frontend/
  src/
    main.ts                          # Frontend entry
    App.vue                          # Root component
    router/index.ts                  # Vue Router configuration
    i18n/                            # 10 languages (zh/en/ja/ko/fr/de/es/ru/pt/ar)
    views/
      layout/index.vue               # Main layout: sidebar menu, titlebar, status bar
      chat/index.vue                 # Chat page: session list, message area, input, streaming
      model/index.vue                # Model provider configuration
      skill/index.vue                # Skill management (search, install, MCP config)
      channel/index.vue              # Channel configuration (Feishu/WeCom/DingTalk)
      usage/index.vue                # Token usage dashboard
      audit/index.vue                # Skill audit (tool call records)
      audit/storage.vue              # Storage audit (persistence operations)
      audit/prompt.vue               # Request log (system prompt + function calling viewer)
  wailsjs/                           # Auto-generated Wails frontend bindings
```

## License

MIT
