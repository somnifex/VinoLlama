# VinoLlama

![VinoLlama logo](desktop/frontend/public/vinollama-logo.png)

VinoLlama is a local-first LLM chat tool for Intel laptops and Intel processors. It is designed to manage local GGUF models and llama.cpp processes, preferring the llama.cpp OpenVINO backend with CPU fallback.

VinoLlama is an independent project. It is not affiliated with Intel, OpenVINO, Ollama, or llama.cpp maintainers.

## Current Status

Stage 9 desktop quality baseline is implemented. Runtime status:

- CLI: partial (`serve`, `run`, `ps`, `stop`, `doctor`, model import/list/rm implemented)
- Model import: implemented
- llama.cpp CPU backend management: implemented for configured llama.cpp-compatible server binaries; fake process integration tests cover startup, readiness, proxying, streaming, stop, idle cleanup, and failure state
- llama.cpp OpenVINO backend management: partial until verified with a real OpenVINO-enabled llama.cpp server binary; command construction is capability-driven and does not hardcode unverified OpenVINO flags
- Desktop GUI: stage-9 React/Vite quality baseline implemented; Chat, Models, Runtime, Settings, Doctor, Logs, local conversation workflows, light/dark theme, generated project logo, and initial frontend tests are connected

Implemented:

- `vinollama --help`
- `vinollama doctor`
- `vinollama import <name> <path-to-gguf>`
- `vinollama list`
- `vinollama rm <model>`
- `vinollama run <model>`
- `vinollama run <path-to-gguf>`
- Safe default configuration loading
- Basic structured logging
- Basic local environment diagnostics
- GGUF manifest read/write
- Filename-based parameter and quantization inference
- Backend interface with CPU/OpenVINO/auto checks
- Cancellable llama.cpp process start/stop primitives
- llama.cpp binary discovery and capability detection
- Capability-aware server command construction
- Runtime manager process reuse and idle cleanup
- Ready/health checks
- Generate/chat proxying instead of fake responses
- `vinollama serve`
- `vinollama ps`
- `vinollama stop <model>`
- API endpoints for show/delete/settings/logs/model import
- Runtime restart endpoint
- Local conversations API with Markdown export
- Interactive CLI chat with non-streaming and streaming output
- Wails/React/Vite desktop shell with Chat, Models, Runtime, Settings, Doctor, and Logs views
- Desktop Chat streaming through `/api/chat`
- Desktop model import through `/api/models/import`
- Desktop runtime stop/restart through `/api/runtime/stop` and `/api/runtime/restart`
- Desktop settings patch through `/api/settings`
- Desktop conversation list/read/save/update/delete through `/api/conversations`
- Desktop conversation Markdown export through `/api/conversations/{id}/export`
- Desktop doctor/log copy and log filtering
- Desktop light/dark theme switching
- Desktop collapsible chat settings sidebar with backend, context, temperature, Top P, threads, and system prompt controls
- Project logo assets embedded in the desktop frontend and Wails app icon path
- Vitest/Testing Library desktop frontend tests

Planned later:

- Wails packaging once the Wails CLI and desktop runtime dependencies are installed locally

## Safe Defaults

- HTTP host: `127.0.0.1`
- HTTP port: `11435`
- Runtime backend: `auto`
- Telemetry: `false`
- Default model import mode: `reference`

VinoLlama does not upload models, prompts, conversations, or diagnostics by default.

## Documentation

- `docs/API.md`: local HTTP API contract.
- `docs/BACKENDS.md`: llama.cpp CPU/OpenVINO backend behavior.
- `docs/BRANDING.md`: logo assets and brand safety rules.
- `docs/DEVELOPMENT.md`: development and verification commands.
- `docs/LOOP_ENGINEERING.md`: required engineering loop.
- `docs/MULTI_AGENT_CONSTRAINTS.md`: collaboration boundaries for agents.
- `docs/PRIVACY.md`: privacy defaults and local-first guarantees.

## Development

Run the backend and CLI checks:

```bash
go test ./...
go run ./cmd/vinollama --help
go run ./cmd/vinollama doctor
go run ./cmd/vinollama ps
go run ./cmd/vinollama import test-model ./testdata/model.gguf --reference
go run ./cmd/vinollama list
go run ./cmd/vinollama run test-model --backend cpu --stream
go run ./cmd/vinollama rm test-model --yes
```

Run the desktop frontend checks:

```bash
cd desktop/frontend
npm install
npm test
npm run typecheck
npm run build
```

Wails packaging checks require the Wails CLI:

```bash
cd desktop
wails version
wails dev
wails build
```

If `wails` is unavailable, use the frontend and Go checks above as the current verification baseline and record the missing CLI as an environment limitation.

`vinollama doctor` returns non-zero when no CPU or OpenVINO llama.cpp binary is configured. Configure a real llama.cpp server binary with `VINOLLAMA_LLAMA_CPU_BIN` or `VINOLLAMA_LLAMA_OPENVINO_BIN` to validate the zero-exit runtime path.

`vinollama run <model>` starts a local interactive chat using the runtime manager and the configured llama.cpp server binary. Type `/exit` or `/quit` to stop. Passing a local `.gguf` path imports it by reference before starting chat. During generation, the first Ctrl+C cancels the current turn and returns to the prompt; pressing Ctrl+C again exits.

Remove only the manifest by default:

Only pass `--delete-file` when you explicitly want VinoLlama to delete the GGUF file referenced by the manifest.

The default config path is:

- Windows: `%USERPROFILE%\.vinollama\config.yaml`
- Linux: `~/.vinollama/config.yaml`

Environment overrides supported in stage 2:

- `VINOLLAMA_BACKEND`
- `VINOLLAMA_HOST`
- `VINOLLAMA_PORT`
- `VINOLLAMA_MODELS`
- `VINOLLAMA_LLAMA_OPENVINO_BIN`
- `VINOLLAMA_LLAMA_CPU_BIN`
- `VINOLLAMA_LOG_LEVEL`

OpenVINO-specific llama.cpp environment variables are passed through to child processes, including `GGML_OPENVINO_DEVICE` and `GGML_OPENVINO_STATEFUL_EXECUTION`.

Branding and logo usage are documented in `docs/BRANDING.md`.

Runtime configuration keys added in stage 3.5:

```yaml
runtime:
  backend: auto
  idle_timeout: 10m
  ready_timeout: 30s
  llama_openvino_bin: ""
  llama_cpu_bin: ""
  internal_port_start: 21435
  health_path: ""
  extra_openvino_args: []
  extra_cpu_args: []
  allow_unverified_flags: false
```