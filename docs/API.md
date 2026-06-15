# VinoLlama API

The stage 3.5 HTTP API is implemented for the local runtime path. It is served by:

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

Streaming APIs should use NDJSON unless a stronger reason is introduced and documented.

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
    {"role": "user", "content": "Hello"}
  ],
  "stream": false
}
```

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

Errors are structured JSON with `what`, `reason`, `fix`, and `details` fields.

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
