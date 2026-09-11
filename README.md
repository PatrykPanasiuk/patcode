# PatCode

PatCode is an early-stage terminal coding agent for inspecting, planning and modifying codebases from the command line.

It runs locally in your terminal, connects to LLM providers (OpenAI-compatible, Anthropic, Ollama, local), and exposes file, shell, and web tools to the model through configurable modes.

## Status

**Early-stage MVP** — functional and usable for experimentation, but not production-hardened.

| Area | Status |
|---|---|
| TUI (Bubble Tea) | implemented |
| Headless `run` command | implemented |
| 13 agent modes with policy engine | implemented |
| Web tools (`webfetch`, `websearch`) | implemented |
| Parallel tool-call execution | implemented |
| Configurable max turns | implemented (`max_turns`, default 8) |
| Streaming tool-call aggregation (OpenAI + Anthropic) | implemented |
| OpenAI-compatible provider | implemented |
| Anthropic provider | implemented |
| Ollama provider | implemented |
| OpenRouter provider | implemented |
| Builtin provider | implemented |
| Local GGUF provider (llama-server) | implemented — requires a llama-server binary or URL |
| File and shell tools | implemented |
| Path confinement + symlink protection | implemented for file/search tools |
| Safe argv-based bash (no `bash -c`) | implemented |
| Session persistence | implemented |
| Session list / show / restore / delete | implemented |
| Custom commands from config | implemented |
| RAG memory bridge | experimental, requires external setup |
| Approval gates | partial — return error in headless mode |
| OS-level sandboxing | not yet — policy engine + path confinement are advisory boundaries |

## Features

- Terminal TUI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- Headless CLI via `patcode run`
- 13 agent modes with a policy engine controlling tool access per mode
- Web research: `webfetch` (docs, API references, any URL) and `websearch` (DuckDuckGo, or Brave via `SEARCH_API_KEY`)
- Parallel tool-call execution inside a turn, ordered results
- Configurable `max_turns` per message
- Provider abstraction with OpenAI-compatible, Anthropic, Ollama, OpenRouter, local (llama.cpp), and builtin backends
- Tool set: read, write, edit, grep, glob, bash, webfetch, websearch
- Bash command allowlist/denylist, test-only policy, and argv-based execution (no shell injection surface)
- Path confinement: file and search tools cannot escape the project root, even through symlinks
- Session persistence (JSON), including `session list|show|restore|delete`
- Custom command templates from `patcode.yaml`
- Optional RAG context bridge via Python/Postgres (experimental)
- Automatic update detection on startup (cached daily check) + `patcode update`

## Installation

Requires Go 1.23+.

```bash
git clone https://github.com/PatrykPanasiuk/patcode.git
cd patcode
go build -o patcode .
```

## Quick start

```bash
# Interactive TUI
./patcode .

# Headless: ask about a project (web research available in ask mode)
./patcode run -p . --mode ask "What is the current LTS version of Next.js?"

# Headless: inspect the codebase (read-only, search + web tools)
./patcode run -p . --mode inspect "Show me the directory structure"

# Headless: plan a refactor (read + write investigations, no edits)
./patcode run -p . --mode plan "Plan a refactor of config loading"

# Headless: build changes (read, write, edit, bash — all with approval gates)
./patcode run -p . --mode build "Add tests for config loading"

# Headless: run tests (test-only bash commands)
./patcode run -p . --mode test "go test ./..."

# Headless: CI (test-only bash, all tools available)
./patcode run -p . --mode ci "Run the full CI pipeline"

# Headless: run a shell command (direct passthrough, no LLM)
./patcode run -p . --mode shell "go test ./..."
```

Web tools are available in every mode except `shell`, including `ask`, so headless research works out of the box.

## Modes

| Mode | Tool Access | Use Case |
|---|---|---|
| ask | webfetch, websearch (LLM-only otherwise) | Knowledge questions, web research |
| inspect | read, grep, glob + web | Codebase exploration, understanding structure |
| plan | read, grep, glob + web | Research and planning, producing design docs |
| review | read, grep, glob, write, edit + web | Code review with suggested edits |
| audit | read, grep, glob + web | Security/quality audit — no modifications |
| patch | read, grep, glob + web | Generating patch files or diffs (no write/edit) |
| build | read, grep, glob, write, edit, bash + web | Implementing features and fixes (approval required for writes/edits/shell) |
| fix | read, grep, glob, write, edit + web | Bug fixing (approval required for writes/edits) |
| refactor | read, grep, glob, write, edit + web | Code restructuring with write access (approval required) |
| scaffold | read, grep, glob, write, edit + web | Project scaffolding — create new files |
| test | read, grep, glob, bash (test-only) + web | Writing and running tests |
| ci | read, grep, glob, bash (test-only) + web | CI pipeline execution |
| shell | direct passthrough | Arbitrary shell commands, no LLM invocation |

**Policy levels:**
- `auto` — tool call executed automatically
- `ask` — returns an error in headless mode (approval gate not wired yet)
- `deny` — tool is blocked for this mode
- `limited` — bash uses mode-specific allowlist (e.g. test-only commands)
- `direct` — shell mode passthrough, no LLM involved

**Warning:** `build`, `fix`, `refactor`, `scaffold`, and `shell` modes can read, write, and execute commands. Only use them in directories you trust.

## Configuration

Place `patcode.yaml` in your project root:

```yaml
provider: openrouter
model: openai/gpt-4o-mini
base_url: https://openrouter.ai/api/v1
temperature: 0.7
max_tokens: 4096
max_turns: 8

permissions:
  auto_approve:
    - read
    - glob
    - grep
```

`max_turns` controls how many tool-call rounds the agent can run per message (default 8).

The permissions block describes intended auto-approval policy. Tool enforcement is wired through the permissions package, which checks each tool call against the current mode's policy (`auto`, `ask`, `deny`, `limited`). Bash commands are additionally validated against a global denylist and mode-specific allowlist.

File and search tools (`read`, `write`, `edit`, `grep`, `glob`) are hard-confined to the project root: any path that resolves outside the root — including through symlinks — is rejected. The policy engine itself remains advisory, not an OS sandbox.

### Custom commands

```yaml
commands:
  - name: review
    prompt: "Review this code for bugs and improvements: {0}"
    description: "Code review helper"
```

Usage: `/review main.go` expands `{0}` to `main.go`.

## Providers

### OpenRouter (recommended for remote models)

```yaml
provider: openrouter
api_key: sk-or-v1-...
model: openai/gpt-4o-mini
base_url: https://openrouter.ai/api/v1
```

Prefer supplying the key via the environment (`OPENROUTER_API_KEY`) — see [Security](#security).

### Ollama

```yaml
provider: ollama
model: llama3
```

Connects to `http://localhost:11434/v1` (OpenAI-compatible endpoint served by Ollama).

### OpenAI-compatible

```yaml
provider: openai
api_key: sk-...
model: gpt-4o
```

Uses the OpenAI Chat Completions API. Supports any OpenAI-compatible endpoint via the optional `base_url` field.

### Anthropic

```yaml
provider: anthropic
api_key: sk-ant-...
model: claude-sonnet-4-20250514
```

Uses the Anthropic Messages API with tool-use blocks, including streaming `input_json_delta` aggregation.

### Local (llama.cpp GGUF)

The `local` provider runs real GGUF inference by delegating to a llama.cpp `llama-server` HTTP endpoint. Two ways to use it:

1. **Attach to a running server** (no auto-start):

```yaml
provider: local
model_path: /path/to/model.gguf
base_url: http://127.0.0.1:8080/v1
```

You can also set `PATCODE_LLAMA_SERVER_URL` instead of `base_url`.

2. **Auto-start** from a `llama-server` binary on your PATH:

```yaml
provider: local
model_path: /path/to/model.gguf
```

Environment knobs: `PATCODE_LLAMA_BIN` (binary path, default `llama-server`), `PATCODE_LLAMA_PORT` (default 8080), `PATCODE_LLAMA_CTX`, `PATCODE_LLAMA_THREADS`, `PATCODE_LLAMA_GPU_LAYERS`. The provider waits up to 120s for the server to become healthy. Install llama.cpp from https://github.com/ggml-org/llama.cpp.

### Builtin

```yaml
provider: builtin
```

A purely deterministic pattern-matching provider for testing the TUI without a real LLM.

## Tools

Available to the model depending on the active mode's policy:

| Tool | Description | Risk |
|---|---|---|
| read | Read file contents with optional offset/limit (confined) | low |
| grep | Search file contents with regex (confined) | low |
| glob | Find files matching glob patterns (confined) | low |
| webfetch | Fetch a URL and return markdown/text | low |
| websearch | Search the web and return result titles/URLs/snippets | low |
| write | Create or overwrite files (confined) | medium/high |
| edit | Perform exact string replacement in files (confined) | medium/high |
| bash | Execute commands as argv with timeout (denylisted, allowlisted per mode) | high |

`webfetch` follows redirects, caps responses at 2MB, and converts HTML to markdown (title + headings + links + code). `websearch` uses DuckDuckGo by default; set `SEARCH_API_KEY` (Brave Search) for a keyed backend.

## Security model

PatCode can read files, write files, execute shell commands, and access the web depending on the active mode and tool access.

### Command execution safety

- The model-facing `bash` tool runs commands **as argv** (quoting is supported, no shell). Shell metacharacters (`|`, `&`, `;`, `<`, `>`, `$`, `` ` ``, `(`, `)`) are rejected.
- Set `PATCODE_BASH_SHELL=1` to explicitly opt back into classic `bash -c` passthrough (re-enables the injection surface).
- Commands are validated against a global denylist (`rm`, `sudo`, `curl`, `wget`, `ssh`, `scp`, `chmod`, `chown`, `dd`, `kubectl`, cloud CLIs, etc.) and mode allowlists (e.g. test-only in `test`/`ci`).
- Path-scoped confinement: `read`, `write`, `edit`, `grep`, and `glob` cannot touch anything outside the project root, including via symlinks. This is a hard check in code, not just policy.

### Secrets handling

- **Never commit API keys or tokens.** `patcode.yaml`, `.env`, `*.pem`, and key files are gitignored.
- Prefer supplying keys via environment variables. Priority: provider-specific env var → `PATCODE_API_KEY` → value in config file. Supported vars: `OPENROUTER_API_KEY`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`.
- A `pre-commit` hook (`hooks/pre-commit`) scans staged files for secret patterns and blocks the commit. Install with `cp hooks/pre-commit .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit`. If gitleaks is installed it is used for deeper scanning.
- Use `patcode.yaml.example` as a template; never copy a real key into a tracked file.

### Runtime safety

- Do **not** run it with elevated privileges (root, sudo).
- Do **not** use it on directories containing secrets (`.env`, `id_rsa`, `~/.aws`, `~/.config/gh`) unless you fully understand the risk.
- Treat model output and tool calls as **untrusted**. The model may generate paths, commands, or content you did not intend to execute.
- The policy engine and path confinement are **not a full OS sandbox** — there is no seccomp/Landlock boundary yet.

## Architecture

```
patcode/
  agent/      agent loop and tool orchestration
  cmd/        CLI commands (cobra)
  config/     YAML configuration and project resolution
  llm/        provider abstraction (OpenAI, Anthropic, Ollama, local, builtin)
  memory/     memory export and ingestion helpers
  permissions/  tool-level policy engine and bash command validation
  ragbridge/  optional local RAG context bridge
  session/    session state, mode definitions, persistence (JSON)
  tools/      file, shell, search, and web tools with policy + confinement
  tui/        Bubble Tea terminal UI
```

## Sessions

Sessions are stored as JSON in `~/.patcode/sessions` (or `session_dir`):

```bash
patcode session list            # id, mode, message count, project
patcode session show <id>       # details
patcode session restore <id>    # resume a session in the TUI
patcode session delete <id>
```

## RAG memory bridge

The `ragbridge` package provides optional local retrieval context for the LLM prompt. When enabled, it queries a Postgres-backed vector store via a local Python script.

- Controlled by environment variables: `RAG_ENABLED`, `RAG_DSN`, `RAG_TOP_K`, etc.
- Requires a Python environment with `rag.retriever` and `psycopg2`.
- The `memory` package can export sessions from Codex and OpenCode into the RAG store.

This feature is **experimental** and depends on external infrastructure not included in this repository.

## Development

```bash
go test ./...
go vet ./...
go build ./...
```

CI runs `go vet`, `go test -race -cover`, and `govulncheck` on every push/PR (`.github/workflows/ci.yml`). Tag `v*` to build release binaries via goreleaser (`.github/workflows/release.yml`, `.goreleaser.yaml`).

## Updates

- `patcode version` prints the current version (`v0.1.0` unless overridden at build time via `-ldflags "-X patcode/version.Version=vX.Y.Z"`).
- On startup patcode checks GitHub for a newer release. The result is cached in `~/.patcode/update_check.json` (one network check per 24h), so startup stays fast and offline runs print nothing. When a newer release exists it shows a one-line notice with the `patcode update` hint.
- `patcode update` checks GitHub explicitly (add `--force` to reinstall the current version) and downloads the release binary for your OS/arch, falling back to building from source when no matching asset exists.

## Limitations

- **Early-stage MVP** — the project is functional but young.
- **No full OS sandbox** — policy engine and path confinement are enforced in Go code, not via seccomp/Landlock.
- **Approval gates are partial** — `ask`-level tools return an error in headless mode instead of prompting.
- **Streaming edge cases** — rare provider-specific stream formats may still be dropped; throttled/retried frames are not resynced.

## Roadmap

- Interactive approval UI for ask-level tools
- OS-level sandboxing (Landlock/seccomp) for bash
- MCP server / plugin / skills system
- Safer `shell` mode passthrough (argv-based)
- Session export/import and search
- More provider coverage and streaming robustness

## License

MIT