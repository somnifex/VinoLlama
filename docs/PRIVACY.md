# Privacy

VinoLlama is local-first and privacy-first.

Current defaults:

- No account system.
- No cloud sync.
- No telemetry.
- No model upload.
- No prompt upload.
- No conversation upload.
- No bundled API keys.
- Default HTTP bind is `127.0.0.1`.
- Default HTTP port is `11435`, not Ollama's `11434`.
- Removing a model deletes only the manifest by default.
- Conversations are stored as local JSON files and are not uploaded or synced.
- Runtime logs are local files only. When `models.directory` is configured, runtime logs are stored beside that model root under `logs/runtime`.
- The desktop scaffold checks only the local VinoLlama API at `127.0.0.1:11435`.

Only `vinollama rm <model> --delete-file` may delete a referenced GGUF file, and the CLI still requires confirmation unless `--yes` is passed.

Future features must keep telemetry disabled by default and must not log full prompts or full conversations by default.
