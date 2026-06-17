# API



The VinoLlama HTTP API is local-only by default and is served by:

```bash
vinollama serve
```

Default base URL:

```text
http://127.0.0.1:11435
```

Implemented endpoints:

```text
GET    /api/version
GET    /api/tags
POST   /api/show
DELETE /api/delete
POST   /api/generate
POST   /api/chat
GET    /api/runtime
POST   /api/runtime/stop
POST   /api/runtime/restart
GET    /api/doctor
GET    /api/settings
POST   /api/settings
GET    /api/deployment
POST   /api/deployment/select
GET    /api/logs
POST   /api/models/import
GET    /api/conversations
POST   /api/conversations
GET    /api/conversations/{id}
PUT    /api/conversations/{id}
DELETE /api/conversations/{id}
POST   /api/conversations/{id}/export
```

Planned endpoints:

```text
POST   /api/show extensions for richer GGUF metadata
POST   /api/runtime/restart options for context/thread overrides
```

Streaming APIs use newline-delimited JSON (NDJSON).

All examples assume the default base URL `http://127.0.0.1:11435`.

The API returns CORS headers only for the Wails desktop origin and loopback development origins such as `localhost` or `127.0.0.1`. External website origins are not granted browser access.

## Generate

`POST /api/generate` starts or reuses a managed llama.cpp server process for the requested model, then proxies to llama.cpp `/completion`.

Request:

```json
{
  "model": "test-model",
  "prompt": "Hello",
  "stream": true,
  "options": {
    "temperature": 0.7,
    "top_p": 0.9
  }
}
```

`stream=true` returns newline-delimited JSON chunks.

## Chat

`POST /api/chat` starts or reuses a managed llama.cpp server process for the requested model, then proxies to llama.cpp `/v1/chat/completions`.

Request:

```json
{
  "model": "test-model",
  "messages": [
    {"role": "system", "content": "Answer briefly."},
    {"role": "user", "content": "Hello"}
  ],
  "stream": false
}
```

The desktop UI stores per-conversation system prompts as a `system` message when saving a conversation and sends that message to `/api/chat`. UI welcome text is not sent to the model.

## Runtime

`GET /api/runtime` returns managed process snapshots.

`POST /api/runtime/stop` accepts:

```json
{
  "model": "test-model"
}
```

`POST /api/runtime/restart` accepts:

```json
{
  "model": "test-model",
  "backend": "cpu"
}
```

Errors are structured JSON with `what`, `reason`, `fix`, and `details` fields:

```json
{
  "error": {
    "what": "Runtime could not start.",
    "reason": "llama.cpp server binary was not found.",
    "fix": "Set VINOLLAMA_LLAMA_CPU_BIN or VINOLLAMA_LLAMA_OPENVINO_BIN.",
    "details": ""
  }
}
```

## Models

`POST /api/show` accepts:

```json
{
  "name": "test-model"
}
```

`DELETE /api/delete` accepts:

```json
{
  "name": "test-model",
  "delete_file": false
}
```

The default behavior deletes only the manifest. `delete_file=true` is required before VinoLlama removes the referenced GGUF file.

`POST /api/models/import` accepts:

```json
{
  "name": "test-model",
  "path": "C:\\models\\model.gguf",
  "mode": "reference"
}
```

## Settings

`GET /api/settings` returns the active in-process settings.

`POST /api/settings` accepts a partial settings patch. In the current implementation settings are not persisted to `config.yaml`; the response includes:

```json
{
  "persisted": false,
  "restart_required": true
}
```

Unsafe public binds such as `0.0.0.0` and `::` are rejected. `privacy.telemetry=true` is rejected because telemetry is not implemented and remains disabled.

Runtime settings include `runtime.backend`, `runtime.idle_timeout`, `runtime.ready_timeout`, `runtime.llama_openvino_bin`, `runtime.llama_cpu_bin`, `runtime.openvino_device`, `runtime.health_path`, `runtime.internal_port_start`, `runtime.extra_openvino_args`, `runtime.extra_cpu_args`, and `runtime.allow_unverified_flags`. `runtime.openvino_device` is passed to managed OpenVINO llama.cpp processes as `GGML_OPENVINO_DEVICE`.

The desktop settings UI uses this endpoint for backend management, llama.cpp binary paths, OpenVINO device selection, advanced runtime arguments, context size, temperature, Top P, and threads. These settings currently apply to the running in-process configuration, not to a persistent config file.

## Deployment

`GET /api/deployment` inspects local OpenVINO and llama.cpp deployment readiness. It reports OpenVINO Runtime or `setupvars` discovery, local build tools, discovered llama.cpp `llama-server` candidates, build plans derived from the llama.cpp OpenVINO backend guide, and recommendations for missing prerequisites.

The deployment API does not silently download, build, or execute remote code. It returns local status and commands for the user to run deliberately.

`POST /api/deployment/select` validates and adopts a discovered or user-provided llama.cpp server binary:

```json
{
  "kind": "openvino",
  "path": "C:\\tools\\llama-server.exe"
}
```

For `kind=openvino`, the binary must pass `--help` and VinoLlama must be able to confirm OpenVINO capability from the help output, binary name, or OpenVINO build directory. The selected path updates the active in-memory runtime settings returned by `GET /api/settings`.

## Logs

`GET /api/logs?limit=200` returns recent runtime log tail lines from the managed llama.cpp runtime log directory. It reads local logs only and does not delete log files.

## Conversations

Conversations are saved as local JSON files under the VinoLlama conversations directory. They are not uploaded or synced.

Create:

```json
POST /api/conversations
{
  "model": "test-model",
  "messages": [
    {"role": "system", "content": "Answer briefly."},
    {"role": "user", "content": "Hello"}
  ]
}
```

List:

```text
GET /api/conversations
```

Read, update, and delete:

```text
GET    /api/conversations/{id}
PUT    /api/conversations/{id}
DELETE /api/conversations/{id}
```

Export Markdown:

```text
POST /api/conversations/{id}/export
```

The desktop UI copies the Markdown export to the clipboard. The API does not upload or sync conversation data.
