# VinoLlama Desktop

This directory contains the stage-6 desktop scaffold.

Current scope:

- Wails v2 project metadata in `wails.json`.
- React + TypeScript + Vite frontend in `frontend/`.
- Local service status detection through `http://127.0.0.1:11435/api/version`.
- Runtime process table through `GET /api/runtime` when the local API is running.

The desktop frontend is a local API client. It does not upload models, prompts, logs, or conversations.

Development commands:

```bash
cd desktop/frontend
npm install
npm run typecheck
npm run build
```

Wails build commands require the Wails CLI and Wails Go dependencies:

```bash
cd desktop
wails dev
wails build
```

The Wails Go entry files are behind the `wails` build tag so normal backend checks such as `go test ./...` continue to work before the Wails CLI is installed.
