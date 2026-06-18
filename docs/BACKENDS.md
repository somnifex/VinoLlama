# Backends



VinoLlama manages llama.cpp processes instead of reimplementing inference.

Implemented backend modes:

- `auto`: try OpenVINO first, then fall back to CPU with a clear warning.
- `openvino`: require a llama.cpp OpenVINO-enabled server binary.
- `cpu`: require a llama.cpp CPU server binary.

The backend layer resolves llama.cpp server binaries, detects supported flags from local `--help` output, starts cancellable localhost llama.cpp processes, waits for readiness, proxies generate/chat requests, supports streaming, and stops processes explicitly or after idle timeout.

## llama.cpp OpenVINO Notes

The OpenVINO backend behavior follows the llama.cpp OpenVINO backend documentation: [OPENVINO.md](https://github.com/ggml-org/llama.cpp/blob/master/docs/backend/OPENVINO.md).

Useful environment variables:

- `GGML_OPENVINO_DEVICE`: selects the OpenVINO target device, such as `CPU`, `GPU`, `NPU`, or a specific GPU target. VinoLlama can also set this per process from `runtime.openvino_device`.
- `GGML_OPENVINO_STATEFUL_EXECUTION=1`: recommended when using GPU targets if stateless execution causes runtime issues.

VinoLlama provides a local deployment manager through `GET /api/deployment`, `POST /api/deployment/select`, and `POST /api/deployment/deploy`. It detects OpenVINO Runtime setup, required build tools, VinoLlama-managed runtime directories, and local llama.cpp server candidates. It also returns end-user recommendations and can copy a validated local llama-server into `~/.vinollama/bin` or `%USERPROFILE%\\.vinollama\\bin` for managed use. It does not silently download, build, or execute remote code.

You can still configure binaries directly with:

- `VINOLLAMA_LLAMA_OPENVINO_BIN` or `runtime.llama_openvino_bin`
- `VINOLLAMA_LLAMA_CPU_BIN` or `runtime.llama_cpu_bin`
- `VINOLLAMA_OPENVINO_DEVICE`, `GGML_OPENVINO_DEVICE`, or `runtime.openvino_device`

The desktop Settings page surfaces the deployment report and can adopt a discovered binary after the backend validates that it is executable and compatible with the requested backend.

VinoLlama starts llama.cpp with localhost binding by default:

```text
-m <model.gguf> --host 127.0.0.1 --port <internal-port> -c <ctx-size> -t <threads>
```

Keep context size conservative on laptop and edge devices, then increase it after `vinollama doctor` and runtime testing look healthy.

## llama.cpp Backend Management

VinoLlama can discover, start, health-check, reuse, proxy, stream, and stop external llama.cpp server processes. VinoLlama still does not reimplement inference.

Current implementation status:

- CPU backend management is implemented for configured llama.cpp-compatible server binaries and covered by fake process integration tests.
- OpenVINO backend management uses the same resolver, capability detector, command builder, process manager, and proxy path, but remains hardware/binary dependent until validated with a real OpenVINO-enabled llama.cpp server binary.
- Deployment management is implemented for local prerequisite inspection, end-user recommendation generation, candidate discovery, validated binary selection, and managed local deployment of user-selected llama.cpp server binaries.
- OpenVINO-specific flags are not hardcoded; they must be detected from `--help` or provided explicitly through extra args.
- The desktop Runtime and Chat views display the active backend state from the local API.

VinoLlama manages llama.cpp through a runtime manager.

Backend modes:

- `auto`
- `openvino`
- `cpu`

Binary resolution order:

1. CLI flag
2. environment variable
3. config file
4. bundled binary path under `.vinollama/bin`
5. PATH lookup

Required configuration:

- `runtime.llama_openvino_bin`
- `runtime.llama_cpu_bin`
- `runtime.openvino_device`
- `runtime.backend`
- `runtime.internal_port_start`
- `runtime.idle_timeout`

OpenVINO behavior:

- prefer OpenVINO backend when `backend=auto`;
- if unavailable, fallback to CPU;
- fallback must be visible in CLI, API and GUI;
- doctor must explain why OpenVINO is unavailable.
- real OpenVINO readiness remains dependent on a locally configured OpenVINO-enabled llama.cpp server binary.

CPU behavior:

- CPU backend must work as the reliable fallback;
- threads, ctx size and batch size must be configurable.

Process lifecycle:

- resolve binary;
- allocate port;
- build command;
- start process;
- capture stdout/stderr;
- wait for ready;
- proxy request;
- update last used time;
- stop on idle timeout;
- stop on explicit user command.

Configuration schema:

```yaml
runtime:
  backend: auto
  idle_timeout: 10m
  ready_timeout: 30s
  llama_openvino_bin: ""
  llama_cpu_bin: ""
  openvino_device: ""
  internal_port_start: 21435
  health_path: ""
  extra_openvino_args: []
  extra_cpu_args: []
  allow_unverified_flags: false
```

`extra_openvino_args` and `extra_cpu_args` are for explicit user-provided advanced flags. By default, VinoLlama only passes flags detected from the local llama.cpp `--help` output. If `allow_unverified_flags=true`, VinoLlama may pass unknown flags, but `vinollama doctor` must warn about the risk.

## Verification

Run the general backend checks after backend changes:

```bash
go test ./...
go run ./cmd/vinollama doctor
```

`vinollama doctor` may return non-zero until `VINOLLAMA_LLAMA_CPU_BIN` or `VINOLLAMA_LLAMA_OPENVINO_BIN` points to a real llama.cpp server binary. That is an environment limitation, not a code failure, when tests still pass.
