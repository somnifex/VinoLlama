你是一名资深系统软件工程师、产品工程负责人和本地 AI 推理工具架构师。请在当前仓库中开发一个名为 **VinoLlama** 的本地大模型聊天工具。

VinoLlama 的定位类似 Ollama，但不是 Ollama 的 fork，也不声称完全兼容 Ollama。它面向 Intel 笔记本和 Intel 处理器优化，后台推理引擎基于 llama.cpp，优先使用 llama.cpp 的 OpenVINO 后端，同时可选支持纯 CPU 后端作为 fallback。项目需要同时支持 CLI、本地 HTTP API 和 Desktop GUI。

你必须严格遵守本仓库的 `AGENTS.md`、`docs/LOOP_ENGINEERING.md`、`docs/MULTI_AGENT_CONSTRAINTS.md` 以及相关子目录下的局部 `AGENTS.md`。如果这些文件不存在，请先创建它们，再进入开发。

---

## 0. 工作方式：Loop Engineering

本项目采用 Loop Engineering，即所有开发都必须进入闭环：

1. **Observe / Inspect**
   * 先检查仓库结构。
   * 读取 README、AGENTS.md、已有源码、构建脚本、测试脚本。
   * 不要直接假设项目为空。
   * 不要凭空假设 llama.cpp、OpenVINO、Wails、React、Go 依赖已经存在。
2. **Plan**
   * 给出分阶段计划。
   * 标明本轮只修改哪些模块。
   * 标明本轮验收命令。
   * 计划必须可执行，不能只写概念。
3. **Implement**
   * 小步提交式修改。
   * 每轮只完成一个清晰目标。
   * 不要一次性重构全仓库。
   * 不要写无法编译的占位代码。
   * 对暂时无法实现的部分写 TODO，并解释依赖条件。
4. **Verify**
   * 每轮必须运行最小相关检查。
   * 后端修改至少运行 `go test ./...`。
   * 前端修改至少运行类型检查、lint 或构建命令。
   * API 修改必须补充或更新测试。
   * 无法运行测试时，必须说明原因，并提供手动验证步骤。
5. **Repair**
   * 如果测试失败，先分析失败原因。
   * 优先最小修复。
   * 修复后重新运行失败测试。
   * 不要用删除测试、放宽断言、跳过检查来掩盖失败。
6. **Record**
   * 每轮结束输出：
     * 修改了什么
     * 为什么这样改
     * 运行了哪些命令
     * 命令结果
     * 剩余风险
     * 下一步建议

最多连续循环 3 轮。如果 3 轮后仍未通过，应停止大规模修改，输出清晰的人工交接报告。

---

## 1. 产品目标

VinoLlama 是一个本地运行的 LLM 管理和聊天工具，提供：

1. 命令行工具：`vinollama`
2. 本地 HTTP API：默认 `127.0.0.1:11435`
3. Desktop GUI：参考 Ollama Desktop 的极简体验，但不得复制其品牌、图标、文案、界面素材或专有设计
4. 模型管理：导入、列出、删除、查看 GGUF 模型
5. 后端管理：OpenVINO / CPU / auto
6. llama.cpp 进程管理
7. 流式输出
8. 本地聊天会话
9. Doctor 诊断
10. 日志查看
11. Windows 和 Linux 优先支持

核心原则：

* 本地优先
* 默认不联网
* 默认不上传模型
* 默认不上传聊天内容
* 默认不监听公网
* 优先 OpenVINO，失败回退 CPU
* 功能可解释，错误可诊断
* 架构清晰，不重造推理框架

---

## 2. 技术栈

默认技术栈如下。除非当前仓库已有明显不同方案，否则按此实现：

Backend:

* Go
* Cobra 或等价 CLI 框架
* net/http、chi 或 Gin，优先简单可维护
* YAML 配置
* context 管理外部进程
* structured logging

Desktop:

* Wails v2
* React
* TypeScript
* Vite
* Tailwind CSS 或轻量 CSS module
* 不优先使用 Electron

Inference:

* llama.cpp
* OpenVINO backend
* CPU backend
* GGUF 模型

Testing:

* Go unit tests
* HTTP API tests
* Frontend typecheck
* GUI smoke test
* Windows/Linux build scripts

---

## 3. 项目结构目标

如果仓库为空，请创建：

```text
.
├── AGENTS.md
├── README.md
├── go.mod
├── cmd/
│   └── vinollama/
│       └── main.go
├── internal/
│   ├── api/
│   ├── backend/
│   │   ├── cpu/
│   │   └── openvino/
│   ├── cli/
│   ├── config/
│   ├── diagnostic/
│   ├── llamacpp/
│   ├── logging/
│   ├── models/
│   ├── runtime/
│   ├── server/
│   └── util/
├── desktop/
│   ├── app/
│   ├── frontend/
│   └── assets/
├── docs/
│   ├── API.md
│   ├── BACKENDS.md
│   ├── DEVELOPMENT.md
│   ├── LOOP_ENGINEERING.md
│   ├── MULTI_AGENT_CONSTRAINTS.md
│   └── PRIVACY.md
├── scripts/
│   ├── build.ps1
│   ├── build.sh
│   ├── build-llama-openvino.ps1
│   ├── build-llama-openvino.sh
│   ├── build-llama-cpu.ps1
│   └── build-llama-cpu.sh
└── tests/
```

如果仓库已有结构，请不要强行迁移。先识别现有结构，再以最小侵入方式加入 VinoLlama 模块。

---

## 4. CLI 设计

主命令：

```bash
vinollama
```

必须实现：

### 4.1 `vinollama serve`

启动本地服务。

默认：

```text
host: 127.0.0.1
port: 11435
```

不得默认占用 Ollama 的 11434。

参数：

```bash
vinollama serve \
  --host 127.0.0.1 \
  --port 11435 \
  --config /path/to/config.yaml \
  --backend auto \
  --verbose
```

backend 可选值：

```text
auto
openvino
cpu
```

行为：

* `openvino`：强制 OpenVINO。
* `cpu`：强制 CPU。
* `auto`：优先 OpenVINO，失败回退 CPU，并记录回退原因。

### 4.2 `vinollama run <model>`

启动交互式聊天。

示例：

```bash
vinollama run qwen2.5-7b
vinollama run ./models/model.gguf
vinollama run qwen2.5-7b --backend openvino
vinollama run qwen2.5-7b --ctx-size 4096 --threads 8
```

要求：

* 支持多轮对话。
* 支持流式输出。
* 支持 Ctrl+C 中断生成。
* 第一次 Ctrl+C 只停止生成，不直接退出程序。
* 再次 Ctrl+C 可退出。
* 服务未启动时，可以启动临时本地服务，或直接启动一次性推理进程。优先简单可靠。

### 4.3 `vinollama list`

列出本地模型。

字段：

```text
NAME
PARAMETERS
QUANTIZATION
SIZE
BACKEND_HINT
MODIFIED
```

### 4.4 `vinollama import <name> <path-to-gguf>`

导入本地 GGUF 模型。

示例：

```bash
vinollama import qwen2.5-7b D:\models\qwen2.5-7b-instruct-q4_k_m.gguf
```

支持：

```bash
--copy
--link
--reference
```

默认优先 `reference`，避免复制大文件。

要求：

* Windows 下软链接可能需要管理员权限，失败时回退 reference。
* 不要误删用户原始模型文件。
* 导入后生成 manifest。
* 尽量推断参数规模、量化类型、文件大小。

### 4.5 `vinollama rm <model>`

删除模型记录。

要求：

* 默认只删除 manifest。
* 外部引用的 GGUF 文件不得删除。
* 只有显式传入 `--delete-file` 才允许删除模型文件。
* 删除前确认，除非传入 `--yes`。

### 4.6 `vinollama ps`

显示运行中模型。

字段：

```text
MODEL
BACKEND
PID
PORT
CTX_SIZE
UPTIME
MEMORY_HINT
```

### 4.7 `vinollama stop <model>`

停止指定模型推理进程。

### 4.8 `vinollama doctor`

检查环境。

至少检查：

* 操作系统
* CPU 架构
* x86_64
* Intel CPU 信息
* OpenVINO Runtime
* llama.cpp OpenVINO 二进制
* llama.cpp CPU 二进制
* 模型目录权限
* 配置文件有效性
* 默认端口占用
* 当前服务状态

输出级别：

```text
PASS
WARN
FAIL
```

FAIL 必须给修复建议。

---

## 5. HTTP API 设计

默认地址：

```text
http://127.0.0.1:11435
```

必须实现以下接口。

### 5.1 `GET /api/version`

返回：

```json
{
  "version": "0.1.0",
  "name": "VinoLlama"
}
```

### 5.2 `GET /api/tags`

列出本地模型，格式尽量接近 Ollama 常用接口，但不要声称完全兼容。

### 5.3 `POST /api/generate`

请求：

```json
{
  "model": "qwen2.5-7b",
  "prompt": "你好",
  "stream": true,
  "options": {
    "temperature": 0.7,
    "top_p": 0.9,
    "num_ctx": 4096
  }
}
```

要求：

* `stream=true` 返回 NDJSON。
* `stream=false` 返回完整结果。
* 支持停止生成。
* 错误返回结构化 JSON。

### 5.4 `POST /api/chat`

请求：

```json
{
  "model": "qwen2.5-7b",
  "messages": [
    {"role": "system", "content": "你是一个简洁的助手"},
    {"role": "user", "content": "解释一下 OpenVINO"}
  ],
  "stream": true,
  "options": {
    "temperature": 0.7
  }
}
```

支持 role：

```text
system
user
assistant
```

初版可以使用简单 chat template，但必须预留模型 family template 选择接口。

### 5.5 `POST /api/show`

查看模型信息。

### 5.6 `DELETE /api/delete`

删除模型记录。

### 5.7 GUI 补充 API

```text
GET    /api/runtime
POST   /api/runtime/stop
POST   /api/runtime/restart
GET    /api/doctor
GET    /api/logs
GET    /api/settings
POST   /api/settings
POST   /api/models/import
GET    /api/conversations
POST   /api/conversations
GET    /api/conversations/{id}
PUT    /api/conversations/{id}
DELETE /api/conversations/{id}
POST   /api/conversations/{id}/export
```

---

## 6. 模型管理

默认目录：

Windows:

```text
%USERPROFILE%\.vinollama
```

Linux:

```text
~/.vinollama
```

结构：

```text
.vinollama/
  config.yaml
  models/
    manifests/
      qwen2.5-7b.yaml
    blobs/
  conversations/
  logs/
  runtime/
  bin/
    llama-openvino/
    llama-cpu/
```

manifest 示例：

```yaml
name: qwen2.5-7b
format: gguf
path: D:\models\qwen2.5-7b-instruct-q4_k_m.gguf
size: 4680000000
quantization: Q4_K_M
parameters: 7B
backend_hint: auto
created_at: "2026-06-15T12:00:00Z"
modified_at: "2026-06-15T12:00:00Z"
template: auto
source: reference
```

要求：

* 初版可从文件名推断 quantization 和 parameters。
* 推断失败显示 unknown。
* 不因元数据推断失败导致导入失败。
* 后续可扩展 GGUF header 读取。

---

## 7. 后端抽象

定义 Backend 接口：

```go
type Backend interface {
    Name() string
    Check(ctx context.Context) DiagnosticResult
    Start(ctx context.Context, req StartRequest) (*ProcessHandle, error)
    Stop(ctx context.Context, handle *ProcessHandle) error
    Health(ctx context.Context, handle *ProcessHandle) error
}
```

实现：

```text
OpenVINOBackend
CPUBackend
AutoBackend
```

AutoBackend：

1. 检查 OpenVINOBackend。
2. 可用则使用 OpenVINO。
3. 不可用则记录 WARN 并回退 CPU。
4. CPU 也不可用则返回明确错误。

进程策略：

* 初版每个模型启动一个 llama.cpp server 子进程。
* 为每个子进程分配内部端口。
* VinoLlama API 负责转发或转换请求。
* 默认 idle timeout 10 分钟。
* 有请求时复用进程。
* 空闲超时后释放进程。
* 所有外部进程必须使用 context 管理。
* Windows 和 Linux 路径都必须兼容。

---

## 8. 配置文件

默认 `config.yaml`：

```yaml
server:
  host: 127.0.0.1
  port: 11435

runtime:
  backend: auto
  idle_timeout: 10m
  llama_openvino_bin: ""
  llama_cpu_bin: ""
  internal_port_start: 21435

generation:
  ctx_size: 4096
  temperature: 0.7
  top_p: 0.9
  threads: 0

models:
  directory: ""
  default_import_mode: reference

desktop:
  start_service_on_launch: true
  stop_service_on_exit: false
  theme: system
  compact_mode: false

privacy:
  telemetry: false

logging:
  level: info
  file: ""
```

优先级：

1. CLI 参数
2. 环境变量
3. 配置文件
4. 默认值

环境变量：

```text
VINOLLAMA_BACKEND
VINOLLAMA_HOST
VINOLLAMA_PORT
VINOLLAMA_MODELS
VINOLLAMA_LLAMA_OPENVINO_BIN
VINOLLAMA_LLAMA_CPU_BIN
VINOLLAMA_LOG_LEVEL
```

---

## 9. Desktop GUI

GUI 是 VinoLlama 后端服务的图形前端，不应绕过后端直接管理 llama.cpp 子进程，除非用于启动 VinoLlama 服务本身。

推荐架构：

```text
Desktop GUI
  -> VinoLlama local API
    -> runtime manager
      -> llama.cpp server process
        -> OpenVINO / CPU backend
```

技术：

* Wails v2
* React
* TypeScript
* Vite
* Tailwind CSS 或轻量 CSS

### 9.1 页面

必须实现：

1. Chat
2. Models
3. Runtime
4. Settings
5. Doctor
6. Logs

### 9.2 Chat 页面

布局：

左侧：

* 新建会话
* 会话列表
* 当前模型选择
* Models
* Runtime
* Doctor
* Settings

主区域：

* 消息列表
* 输入框
* 模型选择
* 后端状态标签
* 发送按钮
* 停止生成按钮

要求：

* 支持 Markdown。
* 支持代码块。
* 支持复制消息。
* 支持重新生成。
* 支持清空会话。
* Enter 发送。
* Shift+Enter 换行。
* Escape 停止生成。
* 流式输出。

### 9.3 Models 页面

功能：

* 模型列表
* 导入 GGUF
* 删除模型记录
* 打开模型目录
* 设置默认模型
* 显示名称、路径、大小、量化、参数规模、推荐后端、最近使用时间

### 9.4 Runtime 页面

显示：

* 当前运行模型
* 当前后端
* fallback 状态
* llama.cpp PID
* 内部端口
* ctx size
* threads
* uptime
* idle timeout
* tokens/s，如果可获得
* memory hint，如果可获得

操作：

* 停止模型
* 重启模型
* 切换后端
* 打开日志
* 运行 doctor

### 9.5 Settings 页面

配置：

* Host
* Port
* 默认后端
* idle timeout
* llama.cpp OpenVINO binary
* llama.cpp CPU binary
* 模型目录
* 默认导入模式
* 默认 ctx size
* 默认 threads
* 默认 temperature
* 默认 top_p
* theme
* compact mode
* start service on launch
* stop service on exit
* telemetry，默认 false

### 9.6 Doctor 页面

展示 `vinollama doctor` 结构化结果。

要求：

* PASS / WARN / FAIL
* FAIL 必须显示修复建议
* 支持复制诊断报告
* 支持导出文本

### 9.7 Logs 页面

功能：

* 查看最近日志
* 按 info / warn / error 过滤
* 搜索
* 复制错误
* 打开日志目录
* 清空显示区域但不删除日志文件

### 9.8 首次启动向导

如果没有配置或没有模型，显示 First-run Wizard：

1. 欢迎页
2. 运行环境检查
3. 选择 llama.cpp 后端二进制
4. 导入 GGUF 模型
5. 设置默认模型
6. 进入 Chat

---

## 10. 聊天会话

保存位置：

```text
.vinollama/conversations/
```

结构：

```json
{
  "id": "conversation-id",
  "title": "short title",
  "model": "qwen2.5-7b",
  "created_at": "2026-06-15T12:00:00Z",
  "updated_at": "2026-06-15T12:05:00Z",
  "messages": [
    {
      "role": "user",
      "content": "你好",
      "created_at": "2026-06-15T12:01:00Z"
    }
  ]
}
```

要求：

* 本地保存。
* 不上传。
* 支持删除。
* 支持重命名。
* 支持切换。
* 标题初版可由用户第一句话截断生成。
* 支持导出 Markdown。

---

## 11. 隐私与安全

硬性约束：

* 不加入账号系统。
* 不加入云同步。
* 不默认下载模型。
* 不默认上传任何数据。
* 不内置商业 API Key。
* 不默认监听 `0.0.0.0`。
* 不默认开放公网访问。
* 不默认加入 telemetry。
* README 必须说明 VinoLlama 是独立项目，与 Intel、OpenVINO、Ollama 无官方从属关系。
* 不使用 Intel 或 OpenVINO 的商标暗示官方背书。

---

## 12. 开发阶段

### 阶段 1：仓库初始化

目标：

* 创建项目骨架。
* 实现 `vinollama --help`。
* 实现配置加载。
* 实现日志。
* 实现基础 doctor。

验收：

```bash
go test ./...
go run ./cmd/vinollama --help
go run ./cmd/vinollama doctor
```

### 阶段 2：模型管理

目标：

* 初始化模型目录。
* 实现 import/list/rm。
* 实现 manifest 读写。
* 实现参数和量化推断。

验收：

```bash
go test ./...
go run ./cmd/vinollama import test-model ./testdata/model.gguf --reference
go run ./cmd/vinollama list
```

如果没有真实 GGUF，创建测试 fixture 或 mock，不要提交大型模型文件。

### 阶段 3：后端抽象

目标：

* Backend 接口。
* CPUBackend。
* OpenVINOBackend。
* AutoBackend。
* 进程启动、停止、健康检查。

验收：

```bash
go test ./...
go run ./cmd/vinollama doctor
```

### 阶段 4：HTTP API

目标：

* serve。
* `/api/version`
* `/api/tags`
* `/api/generate`
* `/api/chat`
* settings/runtime/doctor/logs APIs

验收：

```bash
go test ./...
go run ./cmd/vinollama serve
curl http://127.0.0.1:11435/api/version
curl http://127.0.0.1:11435/api/tags
```

### 阶段 5：CLI 聊天

目标：

* `vinollama run`
* 多轮对话
* 流式输出
* Ctrl+C 中断

验收：

```bash
go test ./...
go run ./cmd/vinollama run test-model
```

### 阶段 6：Desktop 框架

目标：

* Wails + React + TypeScript 初始化。
* GUI 启动。
* 检查本地服务。
* 服务未运行时自动启动。

验收：

```bash
cd desktop/frontend
npm install
npm run typecheck
npm run build
```

以及 Wails 构建命令。

### 阶段 7：GUI 页面

目标：

* Chat 静态布局
* Models 静态布局
* Runtime 静态布局
* Settings 静态布局
* Doctor 静态布局
* Logs 静态布局

验收：

```bash
npm run typecheck
npm run build
```

### 阶段 8：GUI 联调

目标：

* Chat 接入 `/api/chat`
* Models 接入 `/api/tags` 和 `/api/models/import`
* Settings 接入 `/api/settings`
* Runtime 接入 `/api/runtime`
* Doctor 接入 `/api/doctor`
* Logs 接入 `/api/logs`

验收：

```bash
go test ./...
npm run typecheck
npm run build
```

### 阶段 9：文档与打包

目标：

* README
* API.md
* BACKENDS.md
* DEVELOPMENT.md
* PRIVACY.md
* Windows build
* Linux build

验收：

```bash
go test ./...
scripts/build.sh
scripts/build.ps1
```

---

## 13. 多 Agent 工作模式

当任务较大时，请显式使用 subagents，按以下角色分工：

1. Architect Agent
   * 负责架构设计、模块边界、接口稳定性。
   * 不直接大规模写代码。
   * 输出 ADR 或设计说明。
2. Backend Agent
   * 负责 Go CLI、API、配置、模型管理、runtime、后端抽象。
   * 必须补测试。
3. Desktop Agent
   * 负责 Wails、React、TypeScript、GUI 页面和 API 联调。
   * 必须运行 typecheck/build。
4. QA Agent
   * 负责测试计划、回归测试、边界条件、失败复现。
   * 不直接扩大功能范围。
5. Docs Agent
   * 负责 README、API 文档、后端说明、隐私说明、故障排查。
   * 文档必须和实际命令一致。
6. Security/Privacy Agent
   * 负责检查默认监听、文件删除、模型路径、日志泄露、telemetry、外部请求。
   * 发现风险必须阻断合并。

使用 subagents 时，主 agent 必须等待所有 subagents 完成，再合并结论。写密集任务不得让多个 agent 同时修改同一文件。

---

## 14. 代码质量

要求：

* 不要把所有逻辑写进 main.go。
* 不要硬编码用户目录。
* 不要硬编码模型名称。
* 不要提交大型模型文件。
* 不要提交密钥。
* 所有外部进程调用必须支持取消和超时。
* Windows/Linux 路径必须兼容。
* 所有用户可见错误必须包含：
  * 发生了什么
  * 可能原因
  * 修复建议
* API 错误必须结构化。
* GUI 错误必须可读。
* README 中的命令必须能运行。

---

## 15. 当前任务执行要求

现在开始开发。请先执行以下步骤：

1. 检查当前仓库结构。
2. 判断是否已有 Go module、desktop 项目、README、AGENTS.md。
3. 如果缺少 `AGENTS.md`，先创建。
4. 如果缺少 Loop Engineering 文档，先创建。
5. 输出阶段计划。
6. 从阶段 1 开始小步实现。
7. 每完成一阶段，运行验收命令。
8. 输出审计记录。

不要一次性实现所有功能。每轮必须闭环。
## Stage 3.5: llama.cpp backend management completion

Before GUI or additional API surface work, VinoLlama must complete real llama.cpp runtime management.

The stage 3.5 acceptance target is:

- discover configured or installed llama.cpp server binaries;
- distinguish CPU and OpenVINO backend binaries;
- detect supported command-line flags from local `--help` output;
- build commands without hardcoding unverified OpenVINO flags;
- start a llama.cpp server for a specific GGUF model;
- allocate an internal localhost port;
- wait for readiness and perform health checks;
- proxy generate/chat requests, including streaming;
- stop model processes explicitly;
- reclaim idle model processes;
- update process state when child processes exit;
- collect stdout/stderr logs with bounded diagnostic tails;
- expose backend readiness through `vinollama doctor`;
- keep the default HTTP bind on `127.0.0.1:11435`;
- keep telemetry, uploads, account systems, and remote model marketplaces out of scope.

Do not mark OpenVINO or CPU backend support complete unless a real llama.cpp server was health-checked or a fake process integration test proves command construction, ready waiting, proxying, and shutdown.
