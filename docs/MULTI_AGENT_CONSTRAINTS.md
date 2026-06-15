# Multi-Agent Constraints for VinoLlama

This document defines how multiple coding agents collaborate on VinoLlama.

## Roles

- Architect Agent: architecture, module boundaries, API contracts, ADRs.
- Backend Agent: Go CLI, API, config, model manifest, runtime, diagnostics, logging.
- Desktop Agent: Wails, React, TypeScript, GUI layout, GUI API integration.
- QA Agent: test gaps, regression cases, edge cases, cross-platform checks.
- Docs Agent: README and docs accuracy.
- Security/Privacy Agent: network binding, telemetry, logs, file deletion, downloads, secrets.

## Parallelization

Parallelize read-heavy work such as architecture review, test review, documentation review, security review, UI review, and API contract review.

Do not parallelize write-heavy work on the same files, config schema migrations, runtime lifecycle changes, dependency upgrades, or public API changes.

## Main Agent Responsibilities

The main agent must define scope, avoid file conflicts, wait for subagent outputs, merge findings, run verification, and produce the final record.

Security/privacy blockers must be fixed before work is considered complete.

## Completion Checklist

- Relevant findings are merged.
- No file conflicts remain.
- Security/privacy blockers are resolved.
- Relevant tests/checks ran.
- Documentation matches implemented behavior.
- Final output includes evidence.

## llama.cpp runtime ownership

Backend Agent owns:

- `internal/llamacpp/`
- `internal/runtime/`
- `internal/backend/`
- `internal/server` proxy logic

QA Agent owns:

- fake llama.cpp server tests
- process lifecycle tests
- timeout tests
- port allocation tests
- failure-mode tests

Security/Privacy Agent must review:

- process execution
- binary path handling
- logging
- command construction
- public network binding
- accidental prompt logging
