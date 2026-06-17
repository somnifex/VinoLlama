# Loop Engineering for VinoLlama




VinoLlama development uses Loop Engineering: a controlled engineering loop that turns each coding task into a measurable cycle.

The goal is not to make the agent produce more code. The goal is to make the agent produce code that can be inspected, verified, repaired, and trusted.

---

## 1. Core loop

Every task must follow this loop:

```text
Observe -> Plan -> Implement -> Verify -> Repair -> Record
```

### 1.1 Observe

The agent must inspect the repository before modifying it.

Required checks:

* Current directory
* Git status
* Existing project structure
* Existing README
* Existing AGENTS.md
* Existing build/test scripts
* Existing Go module
* Existing desktop/frontend project
* Existing CI configuration

Do not assume an empty repository.

### 1.2 Plan

The plan must include:

* Goal of this loop
* Files likely to change
* Files that must not change
* Test/check commands
* Risk areas
* Stop condition

Bad plan:

```text
Implement the app.
```

Good plan:

```text
Loop goal: implement config loading.
Files: internal/config/config.go, internal/config/config_test.go.
Checks: go test ./internal/config ./...
Stop condition: config defaults load and env override test passes.
```

### 1.3 Implement

Implementation rules:

* Keep changes small.
* Preserve existing public APIs unless explicitly changing them.
* Do not implement GUI and backend in the same loop unless the change is very small.
* Do not add placeholder code that pretends to work.
* Do not swallow errors.
* Do not use panic for normal user-facing failures.
* Prefer clear interfaces and testable functions.

### 1.4 Verify

Verification must be relevant and executable.

Examples:

Backend:

```bash
go test ./...
go run ./cmd/vinollama doctor
```

API:

```bash
go test ./internal/server ./...
curl http://127.0.0.1:11435/api/version
```

Frontend:

```bash
npm test
npm run typecheck
npm run build
```

Desktop:

```bash
wails build
```

If a command cannot be run, the agent must record:

* Command attempted
* Why it failed
* Whether failure is environmental or code-related
* Manual verification fallback

### 1.5 Repair

If verification fails:

1. Read the actual error.
2. Identify root cause.
3. Make the smallest fix.
4. Re-run the failing check.
5. Stop if repeated fixes would become speculative.

Do not:

* Delete tests to pass.
* Skip failing tests without justification.
* Hide errors.
* Replace strict checks with weak checks unless required and explained.

### 1.6 Record

At the end of each loop, record:

```text
Loop summary:
- Goal:
- Files changed:
- Behavior changed:
- Commands run:
- Results:
- Failures:
- Repairs:
- Remaining risks:
- Next recommended loop:
```

## Runtime loop validation

Any change to runtime/backend/llamacpp process management must run:

```bash
go test ./...
go run ./cmd/vinollama doctor
```

If a real llama.cpp binary is unavailable, tests must use a fake llama.cpp server fixture or mock process adapter.

Do not mark runtime support complete unless one of these is true:

* a real llama.cpp server was started and health-checked; or
* a fake process integration test proves command construction, ready wait, proxying and shutdown logic.

---

## 2. Stop conditions

A loop must stop when one of these is true:

1. Verification passes.
2. Maximum loop count is reached.
3. The same failure repeats without progress.
4. The next decision requires human input.
5. The agent detects a security/privacy risk.
6. The requested change conflicts with project constraints.

Default maximum loop count:

```text
3
```

After 3 failed loops, produce a handoff report instead of continuing speculative edits.

---

## 3. Evidence requirements

The agent must prefer evidence over claims.

Valid evidence:

* Test output
* Build output
* Typecheck output
* Runtime command output
* API response
* File diff summary
* Manual reproduction steps

Invalid evidence:

* “It should work”
* “Likely fixed”
* “I believe”
* Unrun tests presented as passing

---

## 4. Repair loop record

For every repair attempt, preserve a small audit record in the final response.

Format:

```text
Attempt 1:
- Finding:
- Change:
- Validation:
- Remaining delta:

Attempt 2:
- Finding:
- Change:
- Validation:
- Remaining delta:
```

This does not need to be committed as a file unless the task explicitly asks for a persistent audit trail.

---

## 5. Multi-agent use inside the loop

Subagents are allowed for read-heavy work.

Good use:

* One agent reviews backend architecture.
* One agent reviews GUI design.
* One agent reviews tests.
* One agent reviews privacy risks.

Bad use:

* Three agents editing the same runtime manager.
* Two agents changing the same config schema.
* One agent refactors while another writes tests against old APIs.

The main agent must merge subagent findings and decide the final patch.

---

## 6. Handoff format

When a loop stops without completing the task, produce:

```text
Handoff:
- Current state:
- Completed:
- Not completed:
- Blocking issue:
- Evidence:
- Recommended next action:
- Files to inspect next:
```

---

# docs/MULTI_AGENT_CONSTRAINTS.md

# Multi-Agent Constraints for VinoLlama

This document defines how multiple coding agents should collaborate on VinoLlama.

---

## 1. Agent roles

### 1.1 Architect Agent

Scope:

* System architecture
* Module boundaries
* API contracts
* Runtime lifecycle
* Desktop/backend integration
* ADRs

May edit:

```text
docs/
internal/*/interfaces.go
internal/*/types.go
```

Should avoid editing:

```text
desktop/frontend/
```

Output:

```text
Architecture findings:
- Decision:
- Alternatives:
- Risks:
- Recommended implementation:
```

---

### 1.2 Backend Agent

Scope:

* Go CLI
* HTTP API
* Config
* Model manifest
* Runtime manager
* llama.cpp process management
* Diagnostics
* Logging

May edit:

```text
cmd/
internal/
tests/
docs/API.md
docs/BACKENDS.md
```

Must run:

```bash
go test ./...
```

Output:

```text
Backend changes:
- Files changed:
- API/CLI behavior:
- Tests:
- Risks:
```

---

### 1.3 Desktop Agent

Scope:

* Wails
* React
* TypeScript
* GUI layout
* GUI state management
* API integration

May edit:

```text
desktop/
docs/
```

Should avoid editing:

```text
internal/runtime/
internal/backend/
```

Must run available frontend checks:

```bash
npm test
npm run typecheck
npm run build
```

Output:

```text
Desktop changes:
- Screens/pages changed:
- API assumptions:
- Build/typecheck:
- UX risks:
```

---

### 1.4 QA Agent

Scope:

* Test gaps
* Regression cases
* Reproduction steps
* Edge cases
* Cross-platform behavior

May edit:

```text
tests/
internal/**/*_test.go
desktop/**/*.test.*
docs/DEVELOPMENT.md
```

Should avoid:

* Large architecture changes
* New production dependencies

Output:

```text
QA findings:
- Missing tests:
- Failing cases:
- Suggested regression tests:
- Commands to run:
```

---

### 1.5 Docs Agent

Scope:

* README
* API documentation
* Backend documentation
* Development documentation
* Privacy documentation
* Branding documentation
* Troubleshooting

May edit:

```text
README.md
docs/
```

Must verify:

* Commands in docs match actual CLI/API
* Unimplemented features are marked planned/TODO
* Privacy claims match implementation
* Brand/logo claims do not imply third-party endorsement

Output:

```text
Docs changes:
- Files changed:
- Behavior documented:
- Claims requiring implementation check:
```

---

### 1.6 Security/Privacy Agent

Scope:

* Network binding
* Telemetry
* Logs
* File deletion
* Model paths
* External downloads
* Binary execution
* Secret handling
* Brand assets

May edit:

```text
internal/config/
internal/server/
internal/logging/
internal/models/
docs/PRIVACY.md
docs/BRANDING.md
```

May block merge if it finds:

* Default `0.0.0.0`
* Silent telemetry
* Model upload
* Prompt upload
* Conversation upload
* API key in source
* Deleting external GGUF files by default
* Logging full prompts/conversations by default
* Brand assets that imply third-party endorsement or hidden cloud features

Output:

```text
Security review:
- Pass/block:
- Findings:
- Required fixes:
- Optional hardening:
```

---

## 2. Parallelization rules

Parallelize:

* Code reading
* Test review
* Documentation review
* Security review
* UI review
* API contract review

Do not parallelize:

* Editing the same file
* Large refactors
* Config schema migrations
* Runtime lifecycle changes
* Dependency upgrades
* Public API changes

---

## 3. Main agent responsibilities

The main agent must:

1. Define subagent scopes.
2. Prevent file conflicts.
3. Wait for subagent outputs.
4. Merge findings.
5. Decide final implementation.
6. Run verification.
7. Produce final record.

The main agent owns correctness. Subagent output is advisory unless explicitly promoted into implementation.

---

## 4. Subagent prompt templates

### 4.1 Architecture review prompt

```text
Spawn an Architect Agent.

Scope:
- Inspect VinoLlama architecture.
- Focus on CLI/API/runtime/Desktop boundaries.
- Do not edit files.
- Identify interface risks and coupling problems.

Return:
- Current architecture summary
- Top 5 risks
- Recommended module boundaries
- Files that should own each responsibility
```

### 4.2 Backend implementation prompt

```text
Spawn a Backend Agent.

Scope:
- Implement the requested backend task only.
- Work in cmd/ and internal/.
- Add or update Go tests.
- Do not modify desktop/frontend.
- Do not change public API shape unless requested.

Verification:
- Run go test ./...

Return:
- Files changed
- Tests run
- Results
- Remaining risks
```

### 4.3 Desktop implementation prompt

```text
Spawn a Desktop Agent.

Scope:
- Implement the requested GUI task only.
- Work in desktop/.
- Use existing API contracts.
- Do not modify internal/runtime or internal/backend.
- Add frontend tests if the project has a test setup.

Verification:
- Run npm run typecheck if available.
- Run npm run build if available.

Return:
- Screens changed
- Components changed
- API assumptions
- Build/typecheck results
```

### 4.4 QA review prompt

```text
Spawn a QA Agent.

Scope:
- Review the current changes for missing tests and edge cases.
- Do not perform broad refactors.
- Add regression tests only if low-risk and scoped.
- Otherwise return a test plan.

Return:
- Test gaps
- Edge cases
- Suggested checks
- Any failing tests found
```

### 4.5 Security/privacy review prompt

```text
Spawn a Security/Privacy Agent.

Scope:
- Review current changes for privacy and local-first violations.
- Check network binding, telemetry, logging, file deletion, downloads, and secret handling.
- Do not modify unrelated code.

Return:
- Pass/block
- Findings
- Required fixes
- Optional hardening
```

---

## 5. Merge protocol

Before finalizing work, the main agent must produce:

```text
Subagent merge summary:
- Architect:
- Backend:
- Desktop:
- QA:
- Security/Privacy:
- Conflicts:
- Final decision:
```

If any Security/Privacy finding is marked block, fix it before completion.

---

## 6. File ownership guide

```text
cmd/vinollama/                Backend Agent
internal/backend/             Backend Agent
internal/config/              Backend Agent + Security Agent
internal/diagnostic/          Backend Agent
internal/llamacpp/            Backend Agent + Security Agent
internal/logging/             Backend Agent + Security Agent
internal/models/              Backend Agent + Security Agent
internal/runtime/             Backend Agent + Architect Agent
internal/server/              Backend Agent + Security Agent + API ownership
desktop/                      Desktop Agent
docs/                         Docs Agent + relevant role
scripts/                      Backend Agent + QA Agent
tests/                        QA Agent
```

---

## 7. Completion checklist

A multi-agent task is complete only when:

* Main agent merged all relevant findings.
* No file conflicts remain.
* Security/privacy blockers are resolved.
* Relevant tests/checks ran.
* Final output includes evidence.
* Documentation is updated if behavior changed.

```

```
