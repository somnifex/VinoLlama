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
npm test
npm run typecheck
npm run build
```

Desktop frontend tests use Vitest, jsdom, and Testing Library. Current coverage focuses on the user-facing chat shell, light/dark theme switching, the collapsible chat settings sidebar, local conversation restore, and ensuring the per-conversation system prompt is sent to the local API without including the UI welcome text.

Desktop brand assets live in:

```text
desktop/frontend/src/assets/vinollama-logo.png
desktop/frontend/public/vinollama-logo.png
desktop/build/appicon.png
```

When changing brand assets, update `docs/BRANDING.md`, run the frontend checks, and visually verify both light and dark themes.

Wails desktop shell checks (v2.12.0):

```bash
cd desktop
wails version
wails dev
wails build
```

`wails build` produces `desktop/build/bin/VinoLlama.exe`. The `//go:build wails` tag has been removed from desktop Go files; no conditional build tags are required.

Cross-platform build scripts with optional flags:

```bash
# Windows (PowerShell)
./scripts/build.ps1 --skip-tests --skip-frontend --skip-desktop --clean

# Linux/macOS (bash)
./scripts/build.sh --skip-tests --skip-frontend --skip-desktop --clean
```