# VinoLlama Agent Instructions


This file defines repository-level instructions for AI coding agents working on VinoLlama.

VinoLlama is a local LLM chat tool for Intel laptops and Intel processors. It provides a CLI, local HTTP API, Desktop GUI, model management, runtime management, diagnostics, and integration with llama.cpp. The preferred inference backend is llama.cpp OpenVINO backend, with CPU backend as fallback.

VinoLlama is an independent project. It is not affiliated with Intel, OpenVINO, Ollama, or llama.cpp maintainers.

---

## 1. Non-negotiable project principles

* Local-first.
* Privacy-first.
* No account system.
* No cloud sync.
* No telemetry unless explicitly added later behind an opt-in setting.
* No remote model marketplace in the initial implementation.
* No default network binding to `0.0.0.0`.
* Default HTTP bind must be `127.0.0.1`.
* Default HTTP port must be `11435`.
* Do not use Ollama default port `11434`.
* Do not upload models.
* Do not upload prompts.
* Do not upload conversations.
* Do not include API keys.
* Do not imply official endorsement by Intel, OpenVINO, Ollama, or llama.cpp.

---

## 2. Required engineering loop

Every non-trivial task must follow this loop:

1. Inspect
2. Plan
3. Implement
4. Verify
5. Repair
6. Record

Do not skip verification unless technically impossible. If verification is impossible, explain why and provide manual verification steps.

At the end of each work session, report:

* Files changed
* Behavior changed
* Tests/checks run
* Results
* Known risks
* Next step

---

## 3. Repository structure

Preferred structure:

```text
cmd/vinollama/
internal/api/
internal/backend/
internal/backend/cpu/
internal/backend/openvino/
internal/cli/
internal/config/
internal/diagnostic/
internal/llamacpp/
internal/logging/
internal/models/
internal/runtime/
internal/server/
internal/util/
desktop/
docs/
scripts/
tests/
```

Do not force this structure if the repository already has a coherent structure. Prefer minimal, compatible changes.

---

## 4. Backend rules

Backend language: Go.

Use Go for:

* CLI
* Config loading
* Model management
* HTTP API
* Runtime process management
* Diagnostics
* Logging
* Desktop service bridge where applicable

Do not reimplement LLM inference. VinoLlama must manage llama.cpp binaries/processes rather than replacing llama.cpp.

All process execution must use `context.Context`.

All long-running processes must be stoppable.

All process errors must include actionable diagnostics.

---

## 5. Desktop rules

Desktop stack preference:

* Wails v2
* React
* TypeScript
* Vite

Avoid Electron unless the repository already uses it.

The Desktop GUI is a frontend for the local VinoLlama API. It should not directly manage llama.cpp processes except for starting/stopping the VinoLlama local service if necessary.

GUI must support:

* Chat page
* Models page
* Runtime page
* Settings page
* Doctor page
* Logs page
* First-run wizard

GUI must clearly show runtime backend state:

* OpenVINO
* CPU
* Auto
* CPU fallback

---

## 6. CLI commands

The CLI binary is:

```bash
vinollama
```

Required commands:

```bash
vinollama serve
vinollama run <model>
vinollama list
vinollama import <name> <path-to-gguf>
vinollama rm <model>
vinollama ps
vinollama stop <model>
vinollama doctor
```

CLI user-facing errors must be readable and actionable.

---

## 7. HTTP API

Default base URL:

```text
http://127.0.0.1:11435
```

Required endpoints:

```text
GET    /api/version
GET    /api/tags
POST   /api/generate
POST   /api/chat
POST   /api/show
DELETE /api/delete
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

Streaming APIs should use NDJSON unless a stronger reason exists.

---

## 8. Configuration rules

Default config location:

Windows:

```text
%USERPROFILE%\.vinollama\config.yaml
```

Linux:

```text
~/.vinollama/config.yaml
```

Precedence:

1. CLI flags
2. Environment variables
3. Config file
4. Defaults

Default config must never expose the service on public interfaces.

---

## 9. Model management rules

Model format:

* GGUF

Do not commit real model files.

Model import modes:

* copy
* link
* reference

Default import mode:

```text
reference
```

Deleting a model must not delete an external GGUF file unless the user explicitly passes `--delete-file`.

If metadata inference fails, show `unknown`; do not fail import.

---

## 10. Runtime rules

Backends:

```text
auto
openvino
cpu
```

Auto behavior:

1. Try OpenVINO.
2. If unavailable, warn and fallback to CPU.
3. If CPU is unavailable, fail with clear diagnostic output.

The runtime manager should reuse running model processes and release them after idle timeout.

Default idle timeout:

```text
10m
```

## llama.cpp backend management rules

VinoLlama must manage llama.cpp as an external runtime.

The project must not pretend backend support exists unless it can:

* discover or configure the llama.cpp binary;
* build the command line safely;
* start a llama.cpp server process with a specific model;
* wait until the server is ready;
* perform health checks;
* proxy generate/chat requests;
* stream responses;
* stop the process;
* reclaim idle processes;
* expose diagnostics through `vinollama doctor`.

Do not hardcode llama.cpp flags without checking the local llama.cpp binary help output or repository documentation.

OpenVINO and CPU backends may use different binaries or the same binary with different flags. The code must model both possibilities.

All process execution must use context and must be cancellable.

All external process errors must include:

* what command was attempted, excluding sensitive paths if needed;
* exit code if available;
* stderr tail;
* probable reason;
* suggested fix.

---

## 11. Testing rules

After Go changes, run:

```bash
go test ./...
```

After frontend changes, run the available equivalent of:

```bash
npm run typecheck
npm run build
```

If scripts do not exist, add them or document why they are unavailable.

For API changes, add or update tests.

For bug fixes, add regression tests when feasible.

Do not remove tests to make CI pass.

Do not weaken assertions without explaining why.

---

## 12. Documentation rules

Update documentation when behavior changes.

Required docs:

```text
README.md
docs/API.md
docs/BACKENDS.md
docs/DEVELOPMENT.md
docs/LOOP_ENGINEERING.md
docs/MULTI_AGENT_CONSTRAINTS.md
docs/PRIVACY.md
```

Documentation must match implemented commands and API behavior.

Do not document unimplemented features as complete. Mark them as planned or TODO.

---

## 13. Security and privacy rules

Block changes that introduce:

* Default public binding
* Silent telemetry
* Cloud upload
* Hidden remote model download
* API keys in source
* Deleting user model files without explicit consent
* Logging full prompts by default
* Logging full conversations by default
* Unclear binary download or execution behavior

If logs need to include prompts for debugging, this must be behind an explicit user setting and disabled by default.

---

## 14. Multi-agent rules

Use subagents only when explicitly requested by the user or by a task prompt.

Recommended subagents:

* Architect Agent
* Backend Agent
* Desktop Agent
* QA Agent
* Docs Agent
* Security/Privacy Agent

Parallelize read-heavy work:

* Architecture review
* Test gap analysis
* Documentation review
* Security review
* UI review

Avoid parallel write-heavy work on the same files.

Before using subagents, define:

* Agent role
* Scope
* Files/directories to inspect
* Files/directories not to edit
* Expected output format
* Whether the main agent must wait for completion

Main agent owns final merge and must resolve conflicts.

---

## 15. Commit and change discipline

Prefer small, coherent changes.

Do not mix unrelated work.

Do not reformat unrelated files.

Do not rename public APIs without migration notes.

Do not add heavy dependencies without justification.

Ask before adding:

* New production dependencies
* Remote services
* Telemetry
* Auto-update systems
* Installer frameworks
* Model download providers

---

## 16. User-facing error format

Errors should include:

```text
What happened:
Reason:
Fix:
```

Example:

```text
What happened: OpenVINO backend was requested but is unavailable.
Reason: llama.cpp OpenVINO binary was not found.
Fix: Set VINOLLAMA_LLAMA_OPENVINO_BIN or run `vinollama doctor`.
```

---

## 17. Definition of done

A task is done only when:

* Code compiles.
* Relevant tests/checks pass or limitations are documented.
* User-facing behavior is documented.
* Errors are actionable.
* Privacy defaults remain safe.
* No large model files or secrets were added.
* The final response includes changed files, commands run, results, and risks.
