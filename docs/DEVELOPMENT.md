# Development

VinoLlama development follows Loop Engineering:

```text
Observe -> Plan -> Implement -> Verify -> Repair -> Record
```

Backend and CLI verification:

```bash
go test ./...
go run ./cmd/vinollama --help
go run ./cmd/vinollama doctor
go run ./cmd/vinollama ps
go run ./cmd/vinollama import test-model ./testdata/model.gguf --reference
go run ./cmd/vinollama list
go run ./cmd/vinollama run test-model --backend cpu --stream
go run ./cmd/vinollama stop test-model
```

`go run ./cmd/vinollama doctor` is expected to return non-zero until at least one llama.cpp backend binary is configured. Use the output as the diagnostic evidence, then configure `VINOLLAMA_LLAMA_CPU_BIN` or `VINOLLAMA_LLAMA_OPENVINO_BIN` for a zero-exit runtime check.

Runtime/backend changes must also be covered by fake llama.cpp process or server tests when no real llama.cpp binary is available.

`vinollama run` supports multi-turn interactive chat, `/exit` and `/quit`, direct `.gguf` paths imported by reference, `--backend`, `--ctx-size`, `--threads`, and `--stream`. At the prompt, Ctrl+C exits. During generation, the first Ctrl+C cancels the current turn and the second Ctrl+C exits.

Do not commit real model files, API keys, generated secrets, or large binaries.

After backend changes, run:

```bash
go test ./...
```

After future frontend changes, run the available equivalent of:

```bash
cd desktop/frontend
npm install
npm run typecheck
npm run build
```

Wails desktop shell checks:

```bash
cd desktop
wails dev
```

`wails dev` requires the Wails CLI. Until it is installed, the Wails Go entry files remain behind the `wails` build tag and are not part of ordinary `go test ./...` runs.
