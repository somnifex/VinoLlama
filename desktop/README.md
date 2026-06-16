# VinoLlama Desktop

This directory contains the VinoLlama desktop shell.

Current scope:

- Wails v2 CLI installed. `wails build` produces `build/bin/VinoLlama.exe`.
- React + TypeScript + Vite frontend in `frontend/`.
- Local service status detection through `http://127.0.0.1:11435/api/version`.
- Runtime process table through `GET /api/runtime` when the local API is running.
- Stage-7 desktop workspace with Chat, Models, Runtime, Settings, Doctor, and Logs views.
- Simple Ollama-inspired chat layout for ordinary users, without copying Ollama branding or cloud behavior.
- Light and dark desktop themes.
- Collapsible chat settings sidebar for backend, context size, temperature, Top P, threads, and per-conversation system prompt.
- Stage-8 API integration for chat streaming, model import, model selection, runtime stop, runtime restart, settings patching, doctor report copy, log filtering/copying, and local conversation list/read/save/update/delete/export.
- Stage-9 frontend quality baseline with Vitest/Testing Library tests.
- Generated project logo embedded in the React sidebar, Vite favicon, and Wails app icon source path.

The desktop frontend is a local API client. It does not upload models, prompts, logs, or conversations.

Conversation export copies a Markdown export from the local API to the clipboard. Conversations remain stored by the backend in the local VinoLlama data directory.

Settings changes currently follow the backend behavior: they update the running in-process configuration and the response indicates whether a restart may be required. They are not persisted to `config.yaml` yet.

Development commands:

```bash
cd desktop/frontend
npm install
npm test
npm run typecheck
npm run build
```

Wails build commands require the Wails CLI and Wails Go dependencies:

```bash
cd desktop
wails dev
wails build
```

The `wails build` output is at `build/bin/VinoLlama.exe`.

Logo assets:

```text
frontend/src/assets/vinollama-logo.png
frontend/public/vinollama-logo.png
build/appicon.png
```

Logo generation and brand constraints are documented in `../docs/BRANDING.md`.
