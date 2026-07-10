# PatCode

PatCode is an early-stage terminal coding agent for inspecting, planning and modifying codebases from the command line.

It runs locally in your terminal, connects to LLM providers (OpenAI-compatible, Anthropic, Ollama, local), and exposes file and shell tools to the model through configurable modes.

## Status

**Early-stage MVP** — functional and usable for experimentation, but not production-hardened.

| Area | Status |
|---|---|
| TUI (Bubble Tea) | implemented |
| Headless `run` command | implemented |
| Modes (ask, plan, build, shell) | implemented |
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
| Hardened sandbox / permissions | not yet implemented |
| Streaming tool-call aggregation | basic — may lose partial tool-call frames |
| Release binaries | not yet available |

## Features

- Terminal TUI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- Headless CLI via `patcode run`
- Four modes controlling model behaviour: ask, plan, build, shell
- Provider abstraction with OpenAI-compatible, Anthropic, Ollama, local, and builtin backends
- Tool set: read, write, edit, grep, glob, bash
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

# Headless: plan a refactor
./patcode run -p . --mode plan "Plan a refactor of config loading"

# Headless: execute changes
./patcode run -p . --mode build "Add tests for config loading"

# Headless: run a shell command
./patcode run -p . --mode shell "go test ./..."
```

## Modes

| Mode | Behaviour |
|---|---|
| ask | Answer questions. No intentional file edits or tool execution. |
| plan | Planning only. The model is instructed to produce a written plan without making changes. |
| build | Coding tools (read, write, edit, grep, glob, bash) are exposed to the model. The agent can inspect and modify files. |
| shell | The input is forwarded directly to a shell. No LLM invocation. |

**Warning:** `build` and `shell` modes can read, write, and execute arbitrary commands. Only use them in directories you trust.

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

The permissions block describes intended auto-approval policy. It is **not** a hardened sandbox — enforcement depends on the caller and the current mode. Do not rely on it for security boundaries.

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

Uses the OpenAI Chat Completions API. Supports any OpenAI-compatible endpoint.

**Note:** There is no dedicated `base_url` config field yet. To use a non-default endpoint (e.g. Together, Groq), set `model_path` to the base URL. This is a naming wart and should be cleaned up — currently `model_path` doubles as an API base URL for the `openai` provider.

### Anthropic

```yaml
provider: anthropic
api_key: sk-ant-...
model: claude-sonnet-4-20250514
```

Uses the Anthropic Messages API with tool-use blocks.

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

Available to the model in `build` mode:

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

- Do **not** run it with elevated privileges (root, sudo).
- Do **not** use it on directories containing secrets (`.env`, `id_rsa`, `~/.aws`, `~/.config/gh`) unless you fully understand the risk.
- Treat model output and tool calls as **untrusted**. The model may generate paths, commands, or content you did not intend to execute.
- Project-scoped sandboxing, path validation, symlink handling, and confirmation gates are **not yet implemented** and should be considered required hardening work.

### Recommended hardening roadmap

- Project-root path enforcement — reject absolute paths outside the target directory
- Symlink escape protection
- Explicit approval flow for bash, write, and edit
- Command denylist / allowlist
- Network command restrictions (curl, nc, ssh)
- Audit log for every tool call with result

## Architecture

```
patcode/
  agent/      agent loop and tool orchestration
  cmd/        CLI commands (cobra)
  config/     YAML configuration and project resolution
  llm/        provider abstraction (OpenAI, Anthropic, Ollama, local, builtin)
  memory/     memory export and ingestion helpers
  permissions/  permission type definitions (wiring into tool execution not yet complete)
  ragbridge/  optional local RAG context bridge
  session/    session persistence (JSON)
  tools/      file, shell, and search tools
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
- **No hardened sandbox** — tool access controls are advisory, not enforced at the OS level.
- **Local provider is a stub** — no real GGUF inference is wired.
- **Streaming tool-call handling is basic** — partial frames from the LLM may be lost.
- **No release binaries** — you must build from source.
- **Limited test coverage** and no CI pipeline.

## Roadmap

- Hardened permissions with path enforcement and confirmation gates
- Multi-step agent loop (multiple tool-call rounds per turn)
- Robust streaming tool-call aggregation with retry
- CI with tests, vet, and govulncheck
- Pre-built release binaries (GitHub Releases)
- Improved session management (list, restore, delete)
- Safer shell execution (no `-c` passthrough, arg-based commands)
- Better documentation and examples

## License

MIT
