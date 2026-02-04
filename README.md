# Dockerizer

AI-powered Docker configuration generator that creates production-ready Dockerfiles, docker-compose.yml, and .dockerignore files for any repository.

**https://dockerizer.dev** by [Dublyo](https://dublyo.com)

[![Go Version](https://img.shields.io/badge/Go-1.23+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## Features

- **Automatic Stack Detection** - Detects language, framework, and version with 90%+ confidence
- **Production-Ready Output** - Multi-stage builds, non-root users, health checks, optimized layers
- **AI Fallback** - Uses OpenAI, Anthropic, or Ollama when detection confidence is low
- **Interactive Setup** - Guided CLI wizard for AI configuration and customization
- **Build Plan** - Nixpacks-inspired plan command for debugging and transparency
- **Procfile Support** - Respects Heroku-style Procfiles for start commands
- **8+ Providers** - Node.js, Python, Go, Rust frameworks supported
- **Agent Mode** - Iterative analyze → generate → build → test → fix workflow
- **MCP Server** - Integration with Claude Code and Goose AI assistants
- **Recipe System** - YAML-based automation workflows

## Installation

### Quick Install (Recommended)

```bash
curl -fsSL https://dockerizer.dev/install.sh | sh
```

### Homebrew (macOS/Linux)

```bash
brew install dublyo/tap/dockerizer
```

### Using Go

```bash
go install github.com/dublyo/dockerizer/cmd/dockerizer@latest
```

### From Source

```bash
git clone https://github.com/dublyo/dockerizer
cd dockerizer/src
make build
sudo make install
```

### Docker

```bash
docker run --rm -v $(pwd):/app ghcr.io/dublyo/dockerizer /app
```

## Quick Start

```bash
# Interactive setup (recommended for first-time users)
dockerizer init

# Auto-detect and generate Docker configs
dockerizer .

# Preview build plan without generating files
dockerizer plan ./my-project

# Force AI generation for better results
ANTHROPIC_API_KEY=sk-ant-xxx dockerizer --ai ./my-project
```

## Supported Frameworks

| Language | Frameworks | Confidence |
|----------|------------|------------|
| **Node.js** | Next.js, Express | 95-100% |
| **Python** | Django, FastAPI, Flask | 90-100% |
| **Go** | Gin, Fiber, Echo | 90% |
| **Rust** | Actix Web, Axum | 90% |

*More frameworks coming soon: Nuxt, NestJS, Rails, Laravel, Spring Boot, and 15+ others*

## Commands

### `dockerizer init` (Interactive Setup)

Guided wizard that walks you through AI configuration and file generation.

```bash
dockerizer init ./my-project
```

Features:
- Auto-detects your stack and confirms
- Prompts for AI provider (Anthropic, OpenAI, Ollama)
- Securely accepts API keys
- Previews and generates files
- Saves configuration for future use

### `dockerizer plan` (Build Plan)

Output the resolved build plan as JSON or YAML without generating files. Inspired by Nixpacks.

```bash
dockerizer plan ./my-project
dockerizer plan --format yaml ./my-project
dockerizer plan --output plan.json ./my-project
```

The plan includes:
- Detection results (language, framework, version, confidence)
- Build phases with commands
- Cache directories for faster builds
- Start command resolution

### `dockerizer [path]`

Generate Docker configuration files.

```bash
dockerizer ./my-project
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--ai` | Force AI generation even for high-confidence detections |
| `-f, --force` | Overwrite existing files |
| `-o, --output` | Output directory (default: same as input) |
| `--no-compose` | Skip docker-compose.yml generation |
| `--no-ignore` | Skip .dockerignore generation |
| `--no-env` | Skip .env.example generation |
| `--json` | Output results as JSON |
| `-v, --verbose` | Enable verbose output |
| `-q, --quiet` | Suppress non-essential output |

### `dockerizer detect [path]`

Detect stack without generating files.

```bash
dockerizer detect ./my-project
dockerizer detect --all ./my-project  # Show all candidates
```

### `dockerizer agent [path]`

Run in agent mode with iterative build/test/fix cycle.

```bash
OPENAI_API_KEY=sk-xxx dockerizer agent ./my-project
```

### `dockerizer serve`

Start MCP server for AI assistant integration (stdio mode).

```bash
dockerizer serve
```

Configure in Claude Code (`~/.claude.json`):
```json
{
  "mcpServers": {
    "dockerizer": {
      "command": "dockerizer",
      "args": ["serve"]
    }
  }
}
```

### `dockerizer recipe [file]`

Execute a YAML workflow recipe.

```bash
dockerizer recipe analyze --path ./my-project
dockerizer recipe generate --path ./my-project
dockerizer recipe build-and-test --path ./my-project
```

### `dockerizer validate [dockerfile]`

Validate Dockerfile syntax and best practices.

```bash
dockerizer validate ./Dockerfile
```

## Environment Overrides

Customize build behavior via environment variables (Nixpacks-inspired):

| Variable | Description |
|----------|-------------|
| `DOCKERIZER_BUILD_CMD` | Override build command |
| `DOCKERIZER_INSTALL_CMD` | Override install/setup command |
| `DOCKERIZER_START_CMD` | Override start command |
| `DOCKERIZER_APT_PKGS` | Additional APT packages (comma-separated) |

Example:
```bash
DOCKERIZER_START_CMD="gunicorn app:app" dockerizer ./my-project
```

## Output Files

Running `dockerizer ./my-project` generates:

| File | Description |
|------|-------------|
| `Dockerfile` | Multi-stage, optimized, production-ready |
| `docker-compose.yml` | Service definition with health checks, resource limits |
| `.dockerignore` | Language-specific exclusions |
| `.env.example` | Environment variables template |

## AI Configuration

Configure AI providers via environment variables:

### Anthropic (Recommended)

```bash
export ANTHROPIC_API_KEY=sk-ant-xxx
export ANTHROPIC_MODEL=claude-3-5-haiku-20241022  # optional
```

### OpenAI

```bash
export OPENAI_API_KEY=sk-xxx
export OPENAI_MODEL=gpt-4o-mini  # optional
```

### Ollama (Local)

```bash
export OLLAMA_BASE_URL=http://localhost:11434  # optional
export OLLAMA_MODEL=llama3  # optional
```

AI is automatically used when:
- Detection confidence is below 80%
- `--ai` flag is specified
- No matching template exists for the detected stack

## Configuration File

Create `.dockerizer.yml` in your project or `~/.dockerizer.yml` globally:

```yaml
ai:
  provider: anthropic
  model: claude-3-5-haiku-20241022

defaults:
  include_compose: true
  include_ignore: true
  include_env: true
  overwrite: false
```

## Example Output

### Build Plan (JSON)

```json
{
  "version": "1.0",
  "generator": "dockerizer v1.0.0",
  "detection": {
    "detected": true,
    "language": "nodejs",
    "framework": "nextjs",
    "version": "14.0.0",
    "confidence": 95,
    "provider": "nextjs"
  },
  "phases": [
    {
      "name": "setup",
      "commands": ["npm ci"]
    },
    {
      "name": "build",
      "depends_on": ["setup"],
      "commands": ["npm run build"]
    }
  ],
  "cache_dirs": [
    {"path": "/root/.npm", "id": "npm-cache"}
  ],
  "start": {
    "cmd": "node server.js"
  }
}
```

### Generated Dockerfile

```dockerfile
# Build stage
FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

# Production stage
FROM node:20-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nextjs
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
COPY --from=builder /app/public ./public
USER nextjs
EXPOSE 3000
CMD ["node", "server.js"]
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:3000/api/health || exit 1
```

## Comparison with Nixpacks

| Feature | Dockerizer | Nixpacks |
|---------|------------|----------|
| Output | Human-editable Dockerfile | OCI image only |
| AI Fallback | Yes (OpenAI, Anthropic, Ollama) | No |
| Interactive Setup | Yes (`dockerizer init`) | No |
| Build Plan | Yes (`dockerizer plan`) | Yes |
| Agent Mode | Yes (iterative fix) | No |
| MCP Integration | Yes | No |
| Procfile Support | Yes | Yes |
| Cache Directories | Yes | Yes |
| Env Overrides | Yes (`DOCKERIZER_*`) | Yes (`NIXPACKS_*`) |

## Development

### Building

```bash
make build          # Build binary to ./build/dockerizer
make build-all      # Build for all platforms
make install        # Install to /usr/local/bin
make clean          # Remove build artifacts
```

### Adding a New Provider

1. Create provider file: `providers/<language>/<framework>.go`
2. Implement the `providers.Provider` interface
3. Register in `providers/<language>/register.go`
4. Add template in `internal/generator/generator.go`

## Roadmap

- [ ] More Node.js frameworks (Nuxt, NestJS, Remix, Astro)
- [ ] Ruby/Rails support
- [ ] PHP/Laravel support
- [ ] Java/Spring Boot support
- [ ] .NET/ASP.NET Core support
- [ ] Kubernetes manifests generation
- [ ] GitHub Actions integration
- [ ] VS Code extension
- [ ] npx-style execution

## License

MIT License - see [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## Links

- [Dockerizer.dev](https://dockerizer.dev)
- [Dublyo PaaS](https://dublyo.com)
- [GitHub](https://github.com/dublyo/dockerizer)
- [Issue Tracker](https://github.com/dublyo/dockerizer/issues)
