# PatCode

AI coding agent for your terminal. Connects to OpenAI, Anthropic, Ollama, or runs locally.

## Usage

```bash
patcode [project-directory]
```

## Modes

- **ask** — answer questions, no changes
- **plan** — propose a plan only
- **build** — inspect and edit files
- **shell** — run shell commands

## Configuration

Place `patcode.yaml` in your project root:

```yaml
provider: ollama
model: llama3
temperature: 0.7
max_tokens: 4096
```

Supported providers: `openai`, `anthropic`, `ollama`, `local`, `builtin`.

## Install

```bash
go build -o patcode .
```

Requires Go 1.21+.

## License

MIT
