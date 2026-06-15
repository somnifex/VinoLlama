# VinoLlama

VinoLlama is a local-first LLM chat tool for Intel laptops and Intel processors. It is designed to manage local GGUF models and llama.cpp processes, preferring the llama.cpp OpenVINO backend with CPU fallback.

VinoLlama is an independent project. It is not affiliated with Intel, OpenVINO, Ollama, or llama.cpp maintainers.

## Current Status

Stage 3.5 baseline is implemented. Runtime status:

- CLI: partial (`serve`, `ps`, `stop`, `doctor`, model import/list/rm implemented; interactive `run` planned)
- Model import: implemented
- llama.cpp CPU backend management: implemented for configured llama.cpp-compatible server binaries; fake process integration tests cover startup, readiness, proxying, streaming, stop, idle cleanup, and failure state
- llama.cpp OpenVINO backend management: partial until verified with a real OpenVINO-enabled llama.cpp server binary; command construction is capability-driven and does not hardcode unverified OpenVINO flags
- Desktop GUI: planned

Implemented:

- `vinollama --help`
- `vinollama doctor`
- `vinollama import <name> <path-to-gguf>`
- `vinollama list`
- `vinollama rm <model>`
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

Planned later:

- CLI chat
- Wails desktop GUI

## Safe Defaults

- HTTP host: `127.0.0.1`
- HTTP port: `11435`
- Runtime backend: `auto`
- Telemetry: `false`
- Default model import mode: `reference`

VinoLlama does not upload models, prompts, conversations, or diagnostics by default.

## Development

Run the stage-3.5 checks:

```bash
go test ./...
go run ./cmd/vinollama --help
go run ./cmd/vinollama doctor
go run ./cmd/vinollama ps
go run ./cmd/vinollama import test-model ./testdata/model.gguf --reference
go run ./cmd/vinollama list
go run ./cmd/vinollama rm test-model --yes
```

`vinollama doctor` returns non-zero when no CPU or OpenVINO llama.cpp binary is configured. Configure a real llama.cpp server binary with `VINOLLAMA_LLAMA_CPU_BIN` or `VINOLLAMA_LLAMA_OPENVINO_BIN` to validate the zero-exit runtime path.

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
