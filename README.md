# PatCode

PatCode is an early-stage terminal coding agent for inspecting, planning and modifying codebases from the command line.

It runs locally in your terminal, connects to LLM providers (OpenAI-compatible, Anthropic, Ollama, local), and exposes file and shell tools to the model through configurable modes.

## Status

**Early-stage MVP** — functional and usable for experimentation, but not production-hardened.

| Area | Status |
|---|---|
| TUI (Bubble Tea) | implemented |
| Headless `run` command | implemented |
| 13 agent modes with policy engine | implemented |
| OpenAI-compatible provider | implemented |
| Anthropic provider | implemented |
| Ollama provider | implemented |
| Builtin provider | implemented |
| File and shell tools | implemented |
| Session persistence | implemented |
| Custom commands from config | implemented |
| RAG memory bridge | experimental, requires external setup |
| Local GGUF provider | stub — no inference backend wired |
| Multi-step agent loop | partial — single tool-call round |
| Bash allow/deny-list and test-only policy | implemented |
| Approval gates | partial — return error in headless mode |
| Streaming tool-call aggregation | basic — may lose partial tool-call frames |
| Release binaries | not yet available |

## Features

- Terminal TUI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- Headless CLI via `patcode run`
- 13 agent modes with a policy engine controlling tool access per mode
- Provider abstraction with OpenAI-compatible, Anthropic, Ollama, local, and builtin backends
- Tool set: read, write, edit, grep, glob, bash
- Bash command allowlist/denylist and test-only policy in `test` and `ci` modes
- Session persistence (JSON) for conversation history
- Custom command templates from `patcode.yaml`
- Optional RAG context bridge via Python/Postgres (experimental)

## Installation

Requires Go 1.21+.

```bash
git clone https://github.com/PatrykPanasiuk/patcode.git
cd patcode
go build -o patcode .
```

## Quick start

```bash
# Interactive TUI
./patcode .

# Headless: ask about a project
./patcode run -p . --mode ask "Explain this project"

# Headless: inspect the codebase (read-only, search tools only)
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

## Modes

| Mode | Tool Access | Use Case |
|---|---|---|
| ask | none (LLM-only) | Knowledge questions, architectural discussions |
| inspect | read, grep, glob | Codebase exploration, understanding structure |
| plan | read, grep, glob | Research and planning, producing design docs |
| review | read, grep, glob, write, edit | Code review with suggested edits |
| audit | read, grep, glob | Security/quality audit — no modifications |
| patch | read, grep, glob | Generating patch files or diffs (no write/edit) |
| build | read, grep, glob, write, edit, bash | Implementing features and fixes (approval required for writes/edits/shell) |
| fix | read, grep, glob, write, edit | Bug fixing (approval required for writes/edits) |
| refactor | read, grep, glob, write, edit | Code restructuring with write access (approval required) |
| scaffold | read, grep, glob, write, edit | Project scaffolding — create new files |
| test | read, grep, glob, bash (test-only) | Writing and running tests |
| ci | read, grep, glob, bash (test-only) | CI pipeline execution |
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
provider: ollama
model: llama3
temperature: 0.7
max_tokens: 4096

permissions:
  auto_approve:
    - read
    - glob
    - grep
```

The permissions block describes intended auto-approval policy. Tool enforcement is now wired through the permissions package, which checks each tool call against the current mode's policy (`auto`, `ask`, `deny`, `limited`). Bash commands are additionally validated against a global denylist and mode-specific allowlist. The system is **advisory** — there is no OS-level sandboxing.

### Custom commands

```yaml
commands:
  - name: review
    prompt: "Review this code for bugs and improvements: {0}"
    description: "Code review helper"
```

Usage: `/review main.go` expands `{0}` to `main.go`.

## Providers

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

Uses the Anthropic Messages API with tool-use blocks.

### OpenRouter

```yaml
provider: openrouter
api_key: sk-or-v1-...
model: openai/gpt-4o-mini
base_url: https://openrouter.ai/api/v1
```

Uses the OpenAI-compatible endpoint. Prefer supplying the key via the
environment (`OPENROUTER_API_KEY` or `PATCODE_API_KEY`) instead of a file —
see [Security](#security).

### Local (experimental)

```yaml
provider: local
model_path: /path/to/model.gguf
```

The `local` provider is a **stub**. It echoes the prompt back word-by-word as a placeholder. No GGUF inference is wired — CGO bindings (e.g. `go-llama.cpp`) would need to be integrated.

### Builtin

```yaml
provider: builtin
```

A purely deterministic pattern-matching provider for testing the TUI without a real LLM.

## Tools

Available to the model depending on the active mode's policy:

| Tool | Description | Risk |
|---|---|---|
| read | Read file contents with optional offset/limit | low |
| grep | Search file contents with regex | low |
| glob | Find files matching glob patterns | low |
| write | Create or overwrite files | medium/high |
| edit | Perform exact string replacement in files | medium/high |
| bash | Execute shell commands with timeout | high |

## Security model

PatCode can read files, write files, and execute shell commands depending on the active mode and tool access.

### Secrets handling

- **Never commit API keys or tokens.** `patcode.yaml`, `.env`, `*.pem`, and key files are gitignored.
- Prefer supplying keys via environment variables. Priority: provider-specific env var → `PATCODE_API_KEY` → value in config file. Supported vars: `OPENROUTER_API_KEY`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`.
- A `pre-commit` hook (`hooks/pre-commit`) scans staged files for secret patterns and blocks the commit. Install with `cp hooks/pre-commit .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit`. If gitleaks is installed it is used for deeper scanning.
- Use `patcode.yaml.example` as a template; never copy a real key into a tracked file.

### Runtime safety

- Do **not** run it with elevated privileges (root, sudo).
- Do **not** use it on directories containing secrets (`.env`, `id_rsa`, `~/.aws`, `~/.config/gh`) unless you fully understand the risk.
- Treat model output and tool calls as **untrusted**. The model may generate paths, commands, or content you did not intend to execute.
- A policy engine enforces per-mode tool access (auto/ask/deny/limited) for every tool call at runtime.
- Bash commands are validated against a global denylist (rm, sudo, curl, wget, ssh, scp, chmod, chown, dd, kubectl, docker) and mode-specific allowlists (e.g. test-only in `test` and `ci` modes).
- There is **no OS-level sandboxing** — the policy engine is advisory, not a security boundary.
- Symlink protection, path-scoped confinement, and interactive approval gates are **not yet implemented**.

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
  tools/      file, shell, and search tools with runtime policy enforcement
  tui/        Bubble Tea terminal UI
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

## Limitations

- **Early-stage MVP** — the project is functional but young.
- **Policy engine is advisory** — per-mode tool access and bash allowlist/denylist are enforced in Go code, not at the OS level.
- **Local provider is a stub** — no real GGUF inference is wired.
- **Streaming tool-call handling is basic** — partial frames from the LLM may be lost.
- **No release binaries** — you must build from source.
- **Limited test coverage** and no CI pipeline.

## Roadmap

- OS-level sandboxing and path-scoped confinement
- Interactive approval UI for ask-level tools
- Multi-step agent loop (multiple tool-call rounds per turn)
- Robust streaming tool-call aggregation with retry
- CI with tests, vet, and govulncheck
- Pre-built release binaries (GitHub Releases)
- Improved session management (list, restore, delete)
- Safer shell execution (no `-c` passthrough, arg-based commands)
- Better documentation and examples

## License

MIT
