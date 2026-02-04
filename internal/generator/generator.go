// Package generator provides output file generation functionality.
package generator

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/dublyo/dockerizer/internal/ai"
	"github.com/dublyo/dockerizer/internal/detector"
	"github.com/dublyo/dockerizer/internal/errors"
	"github.com/dublyo/dockerizer/internal/scanner"
)

// Generator generates Docker configuration files
type Generator interface {
	Generate(result *detector.DetectionResult, outputPath string) (*Output, error)
	GenerateWithAIFallback(ctx context.Context, result *detector.DetectionResult, scan *scanner.ScanResult, outputPath string) (*Output, error)
}

// Output contains the generated files
type Output struct {
	Dockerfile    string
	DockerCompose string
	Dockerignore  string
	EnvExample    string
	Files         map[string]string // path -> content
}

// Option configures the generator
type Option func(*generator)

// generator implements Generator
type generator struct {
	providerPath   string // Path to provider templates
	overwrite      bool
	includeCompose bool
	includeIgnore  bool
	includeEnv     bool
	aiProvider     ai.Provider // Optional AI provider for fallback
}

// New creates a new generator
func New(opts ...Option) Generator {
	g := &generator{
		overwrite:      false,
		includeCompose: true,
		includeIgnore:  true,
		includeEnv:     true,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// WithOverwrite allows overwriting existing files
func WithOverwrite(overwrite bool) Option {
	return func(g *generator) {
		g.overwrite = overwrite
	}
}

// WithCompose enables/disables docker-compose generation
func WithCompose(include bool) Option {
	return func(g *generator) {
		g.includeCompose = include
	}
}

// WithIgnore enables/disables .dockerignore generation
func WithIgnore(include bool) Option {
	return func(g *generator) {
		g.includeIgnore = include
	}
}

// WithEnv enables/disables .env.example generation
func WithEnv(include bool) Option {
	return func(g *generator) {
		g.includeEnv = include
	}
}

// WithProviderPath sets the path to provider templates (for external templates)
func WithProviderPath(path string) Option {
	return func(g *generator) {
		g.providerPath = path
	}
}

// WithAIProvider sets an AI provider for fallback generation
func WithAIProvider(provider ai.Provider) Option {
	return func(g *generator) {
		g.aiProvider = provider
	}
}

// Generate creates all Docker configuration files
func (g *generator) Generate(result *detector.DetectionResult, outputPath string) (*Output, error) {
	output := &Output{
		Files: make(map[string]string),
	}

	// Prepare template variables
	vars := make(map[string]interface{})
	for k, v := range result.Variables {
		vars[k] = v
	}
	vars["language"] = result.Language
	vars["framework"] = result.Framework
	vars["version"] = result.Version

	// Generate Dockerfile
	dockerfile, err := g.generateDockerfile(result.Template, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Dockerfile: %w", err)
	}
	output.Dockerfile = dockerfile
	output.Files["Dockerfile"] = dockerfile

	// Generate docker-compose.yml
	if g.includeCompose {
		compose, err := g.generateCompose(vars)
		if err != nil {
			return nil, fmt.Errorf("failed to generate docker-compose.yml: %w", err)
		}
		output.DockerCompose = compose
		output.Files["docker-compose.yml"] = compose
	}

	// Generate .dockerignore
	if g.includeIgnore {
		ignore, err := g.generateDockerignore(result.Language, vars)
		if err != nil {
			return nil, fmt.Errorf("failed to generate .dockerignore: %w", err)
		}
		output.Dockerignore = ignore
		output.Files[".dockerignore"] = ignore
	}

	// Generate .env.example
	if g.includeEnv {
		envExample, err := g.generateEnvExample(vars)
		if err != nil {
			return nil, fmt.Errorf("failed to generate .env.example: %w", err)
		}
		output.EnvExample = envExample
		output.Files[".env.example"] = envExample
	}

	// Write files if outputPath is provided
	if outputPath != "" {
		if err := g.writeFiles(output, outputPath); err != nil {
			return nil, err
		}
	}

	return output, nil
}

// GenerateWithAIFallback tries rule-based generation first, then falls back to AI if it fails
func (g *generator) GenerateWithAIFallback(ctx context.Context, result *detector.DetectionResult, scan *scanner.ScanResult, outputPath string) (*Output, error) {
	// Try rule-based generation first
	output, err := g.Generate(result, "")
	if err == nil {
		// Rule-based generation succeeded
		if outputPath != "" {
			if writeErr := g.writeFiles(output, outputPath); writeErr != nil {
				return nil, writeErr
			}
		}
		return output, nil
	}

	// Check if AI provider is configured
	if g.aiProvider == nil {
		return nil, fmt.Errorf("rule-based generation failed and no AI provider configured: %w", err)
	}

	// Check if AI provider is available
	if !g.aiProvider.IsAvailable() {
		return nil, fmt.Errorf("rule-based generation failed and AI provider is not available: %w", err)
	}

	// Fall back to AI generation
	aiResponse, aiErr := g.aiProvider.Generate(ctx, scan, "")
	if aiErr != nil {
		return nil, fmt.Errorf("both rule-based and AI generation failed: rule-based: %w, AI: %v", err, aiErr)
	}

	// Convert AI response to Output
	output = &Output{
		Dockerfile:    aiResponse.Dockerfile,
		DockerCompose: aiResponse.DockerCompose,
		Dockerignore:  aiResponse.Dockerignore,
		EnvExample:    aiResponse.EnvExample,
		Files:         make(map[string]string),
	}

	if output.Dockerfile != "" {
		output.Files["Dockerfile"] = output.Dockerfile
	}
	if g.includeCompose && output.DockerCompose != "" {
		output.Files["docker-compose.yml"] = output.DockerCompose
	}
	if g.includeIgnore && output.Dockerignore != "" {
		output.Files[".dockerignore"] = output.Dockerignore
	}
	if g.includeEnv && output.EnvExample != "" {
		output.Files[".env.example"] = output.EnvExample
	}

	// Write files if outputPath is provided
	if outputPath != "" {
		if writeErr := g.writeFiles(output, outputPath); writeErr != nil {
			return nil, writeErr
		}
	}

	return output, nil
}

// generateDockerfile generates a Dockerfile from the template
func (g *generator) generateDockerfile(templatePath string, vars map[string]interface{}) (string, error) {
	// Try to load from provider path first if set
	var tmplContent []byte
	var err error

	if g.providerPath != "" {
		fullPath := filepath.Join(g.providerPath, templatePath)
		tmplContent, err = os.ReadFile(fullPath)
	}

	if err != nil || g.providerPath == "" {
		// Fall back to embedded templates
		// The template path is like "nodejs/nextjs.tmpl", we need to look in the providers directory
		// For now, use a simple fallback template
		tmplContent, err = getProviderTemplate(templatePath)
		if err != nil {
			return "", fmt.Errorf("%w: %s", errors.ErrTemplateNotFound, templatePath)
		}
	}

	return g.executeTemplate(string(tmplContent), vars)
}

// generateCompose generates a docker-compose.yml file
func (g *generator) generateCompose(vars map[string]interface{}) (string, error) {
	tmpl := composeTemplate
	return g.executeTemplate(tmpl, vars)
}

// generateDockerignore generates a .dockerignore file
func (g *generator) generateDockerignore(language string, vars map[string]interface{}) (string, error) {
	ignoreContent := baseDockerignore

	// Add language-specific ignores
	switch language {
	case "nodejs":
		ignoreContent += nodejsDockerignore
	case "python":
		ignoreContent += pythonDockerignore
	case "go":
		ignoreContent += goDockerignore
	case "rust":
		ignoreContent += rustDockerignore
	case "ruby":
		ignoreContent += rubyDockerignore
	case "php":
		ignoreContent += phpDockerignore
	case "java":
		ignoreContent += javaDockerignore
	case "dotnet":
		ignoreContent += dotnetDockerignore
	case "elixir":
		ignoreContent += elixirDockerignore
	}

	return ignoreContent, nil
}

// generateEnvExample generates a .env.example file
func (g *generator) generateEnvExample(vars map[string]interface{}) (string, error) {
	port := "3000"
	if p, ok := vars["port"].(string); ok {
		port = p
	}

	env := fmt.Sprintf(`# Environment Configuration
# Generated by Dublyo Dockerizer

# Application
APP_NAME=myapp
NODE_ENV=production
PORT=%s

# Domain (for Traefik routing)
DOMAIN=myapp.example.com

# Resource Limits
MEMORY_LIMIT=512M
MEMORY_RESERVATION=256M

# Add your environment variables below
# DATABASE_URL=
# REDIS_URL=
# API_KEY=
`, port)

	return env, nil
}

// executeTemplate executes a template with the given variables
func (g *generator) executeTemplate(tmplContent string, vars map[string]interface{}) (string, error) {
	funcMap := template.FuncMap{
		"default": func(def, val interface{}) interface{} {
			if val == nil || val == "" {
				return def
			}
			return val
		},
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
		"title": func(s string) string {
			if len(s) == 0 {
				return s
			}
			return strings.ToUpper(s[:1]) + s[1:]
		},
		"trimSuffix": strings.TrimSuffix,
		"replace":    strings.ReplaceAll,
	}

	tmpl, err := template.New("template").Funcs(funcMap).Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("%w: %v", errors.ErrTemplateInvalid, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("template execution failed: %w", err)
	}

	return buf.String(), nil
}

// writeFiles writes output files to disk
func (g *generator) writeFiles(output *Output, outputPath string) error {
	for filename, content := range output.Files {
		fullPath := filepath.Join(outputPath, filename)

		// Check if file exists
		if !g.overwrite {
			if _, err := os.Stat(fullPath); err == nil {
				// File exists, skip
				continue
			}
		}

		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("%w: %s: %v", errors.ErrWriteFailed, filename, err)
		}
	}

	return nil
}

// getProviderTemplate returns the template content for a provider
func getProviderTemplate(templatePath string) ([]byte, error) {
	templates := map[string]string{
		// Node.js
		"nodejs/nextjs.tmpl":    nextjsTemplate,
		"nodejs/nuxt.tmpl":      nuxtTemplate,
		"nodejs/nestjs.tmpl":    nestjsTemplate,
		"nodejs/remix.tmpl":     remixTemplate,
		"nodejs/astro.tmpl":     astroTemplate,
		"nodejs/sveltekit.tmpl": sveltekitTemplate,
		"nodejs/hono.tmpl":      honoTemplate,
		"nodejs/koa.tmpl":       koaTemplate,
		"nodejs/express.tmpl":   expressTemplate,
		// Python
		"python/django.tmpl":  djangoTemplate,
		"python/fastapi.tmpl": fastapiTemplate,
		"python/flask.tmpl":   flaskTemplate,
		// Go
		"go/gin.tmpl":      ginTemplate,
		"go/fiber.tmpl":    fiberTemplate,
		"go/echo.tmpl":     echoTemplate,
		"go/standard.tmpl": goStandardTemplate,
		// Rust
		"rust/actix.tmpl": actixTemplate,
		"rust/axum.tmpl":  axumTemplate,
		// Ruby
		"ruby/rails.tmpl": railsTemplate,
		// PHP
		"php/laravel.tmpl": laravelTemplate,
		"php/symfony.tmpl": symfonyTemplate,
		// Java
		"java/springboot.tmpl": springbootTemplate,
		// .NET
		"dotnet/aspnet.tmpl": aspnetTemplate,
		// Elixir
		"elixir/phoenix.tmpl": phoenixTemplate,
	}

	if tmpl, ok := templates[templatePath]; ok {
		return []byte(tmpl), nil
	}

	return nil, errors.ErrTemplateNotFound
}

// Template constants
const composeTemplate = `# Docker Compose Configuration
# Generated by Dublyo Dockerizer
# https://github.com/dublyo/dockerizer

services:
  app:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: ${APP_NAME:-app}
    restart: unless-stopped
    init: true  # Proper signal handling and zombie process reaping
    ports:
      - "${PORT:-{{.port | default "3000"}}}:{{.port | default "3000"}}"

    # Environment
    env_file:
      - .env
    environment:
      - NODE_ENV=production

    # Health Check (uses wget which is available in Alpine)
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:{{.port | default "3000"}}/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s

    # Resource Limits
    deploy:
      resources:
        limits:
          memory: ${MEMORY_LIMIT:-512M}
        reservations:
          memory: ${MEMORY_RESERVATION:-256M}

    # Logging (prevent disk exhaustion)
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

    # Networking (uncomment for Traefik reverse proxy)
    # networks:
    #   - web
    #   - internal
    # labels:
    #   - "traefik.enable=true"
    #   - "traefik.http.routers.${APP_NAME:-app}.rule=Host(` + "`${DOMAIN}`" + `)"
    #   - "traefik.http.routers.${APP_NAME:-app}.entrypoints=websecure"
    #   - "traefik.http.routers.${APP_NAME:-app}.tls.certresolver=letsencrypt"
    #   - "traefik.http.services.${APP_NAME:-app}.loadbalancer.server.port={{.port | default "3000"}}"

# Uncomment for Traefik reverse proxy setup
# networks:
#   web:
#     external: true
#   internal:
#     driver: bridge
`

const baseDockerignore = `# Docker ignore file
# Generated by Dublyo Dockerizer

# Git
.git
.gitignore
.gitattributes

# Docker
Dockerfile*
docker-compose*
.docker

# IDE
.idea
.vscode
*.swp
*.swo
*~

# OS
.DS_Store
Thumbs.db

# Documentation
README.md
CHANGELOG.md
LICENSE
docs/

# CI/CD
.github
.gitlab-ci.yml
.travis.yml
Jenkinsfile

# Testing
coverage/
.nyc_output/
*.test.*
__tests__/
test/
tests/
`

const nodejsDockerignore = `
# Node.js specific
node_modules/
npm-debug.log*
yarn-debug.log*
yarn-error.log*
.npm
.yarn

# Build outputs
dist/
build/
.next/
.nuxt/
.output/

# Environment
.env
.env.local
.env.*.local

# TypeScript
*.tsbuildinfo
`

const pythonDockerignore = `
# Python specific
__pycache__/
*.py[cod]
*$py.class
.Python
venv/
.venv/
ENV/
env/
.eggs/
*.egg-info/
.mypy_cache/
.pytest_cache/

# Environment
.env
.env.local
`

const goDockerignore = `
# Go specific
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test
*.out
vendor/

# Environment
.env
.env.local
`

const rustDockerignore = `
# Rust specific
target/
**/*.rs.bk
Cargo.lock

# Environment
.env
.env.local
`

// Embedded templates for providers
const nextjsTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Next.js
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM node:{{.nodeVersion | default "20"}}-alpine AS builder

WORKDIR /app

{{if eq .packageManager "pnpm"}}
# Enable pnpm
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY yarn.lock ./
{{else if eq .packageManager "bun"}}
# Install bun
RUN npm install -g bun
COPY bun.lockb ./
{{else}}
COPY package-lock.json ./
{{end}}

COPY package.json ./

{{if eq .packageManager "pnpm"}}
RUN pnpm install --frozen-lockfile
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile
{{else if eq .packageManager "bun"}}
RUN bun install --frozen-lockfile
{{else}}
RUN npm ci
{{end}}

# Copy source
COPY . .

# Build
ENV NEXT_TELEMETRY_DISABLED=1
{{if eq .packageManager "pnpm"}}
RUN pnpm build
{{else if eq .packageManager "yarn"}}
RUN yarn build
{{else if eq .packageManager "bun"}}
RUN bun run build
{{else}}
RUN npm run build
{{end}}

# Production stage
FROM node:{{.nodeVersion | default "20"}}-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

# Create non-root user
RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nextjs

{{if .standalone}}
# Copy standalone build
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
COPY --from=builder /app/public ./public

USER nextjs

EXPOSE {{.port | default "3000"}}
ENV PORT={{.port | default "3000"}}
ENV HOSTNAME="0.0.0.0"

CMD ["node", "server.js"]
{{else}}
# Copy build output
COPY --from=builder --chown=nextjs:nodejs /app/.next ./.next
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/package.json ./package.json
COPY --from=builder /app/public ./public

USER nextjs

EXPOSE {{.port | default "3000"}}
ENV PORT={{.port | default "3000"}}

{{if eq .packageManager "pnpm"}}
CMD ["pnpm", "start"]
{{else if eq .packageManager "yarn"}}
CMD ["yarn", "start"]
{{else if eq .packageManager "bun"}}
CMD ["bun", "run", "start"]
{{else}}
CMD ["npm", "start"]
{{end}}
{{end}}

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "3000"}}/api/health || exit 1
`

const expressTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Express.js
# https://github.com/dublyo/dockerizer
# ============================================

{{if .typescript}}
# Build stage (TypeScript)
FROM node:{{.nodeVersion | default "20"}}-alpine AS builder

WORKDIR /app

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY yarn.lock ./
{{else}}
COPY package-lock.json ./
{{end}}

COPY package.json ./
COPY tsconfig.json ./

{{if eq .packageManager "pnpm"}}
RUN pnpm install --frozen-lockfile
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile
{{else}}
RUN npm ci
{{end}}

COPY . .

{{if eq .packageManager "pnpm"}}
RUN pnpm build
{{else if eq .packageManager "yarn"}}
RUN yarn build
{{else}}
RUN npm run build
{{end}}

# Production stage
FROM node:{{.nodeVersion | default "20"}}-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production

# Create non-root user
RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 expressjs

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY --from=builder /app/pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY --from=builder /app/yarn.lock ./
{{else}}
COPY --from=builder /app/package-lock.json ./
{{end}}

COPY --from=builder /app/package.json ./
COPY --from=builder /app/dist ./dist

{{if eq .packageManager "pnpm"}}
RUN pnpm install --frozen-lockfile --prod
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile --production
{{else}}
RUN npm ci --only=production
{{end}}

USER expressjs

EXPOSE {{.port | default "3000"}}
ENV PORT={{.port | default "3000"}}

CMD ["node", "dist/index.js"]
{{else}}
# Production stage (JavaScript)
FROM node:{{.nodeVersion | default "20"}}-alpine

WORKDIR /app

ENV NODE_ENV=production

# Create non-root user
RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 expressjs

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY yarn.lock ./
{{else}}
COPY package-lock.json ./
{{end}}

COPY package.json ./

{{if eq .packageManager "pnpm"}}
RUN pnpm install --frozen-lockfile --prod
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile --production
{{else}}
RUN npm ci --only=production
{{end}}

COPY . .

USER expressjs

EXPOSE {{.port | default "3000"}}
ENV PORT={{.port | default "3000"}}

CMD ["node", "{{.mainFile | default "index.js"}}"]
{{end}}

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "3000"}}/health || exit 1
`

// Django template
const djangoTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Django
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM python:{{.pythonVersion | default "3.12"}}-slim AS builder

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    libpq-dev \
    && rm -rf /var/lib/apt/lists/*

# Install Python dependencies
{{if eq .packageManager "poetry"}}
RUN pip install poetry
COPY pyproject.toml poetry.lock* ./
RUN poetry config virtualenvs.create false && poetry install --no-dev --no-interaction --no-ansi
{{else if eq .packageManager "pipenv"}}
RUN pip install pipenv
COPY Pipfile Pipfile.lock* ./
RUN pipenv install --system --deploy --ignore-pipfile
{{else}}
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
{{end}}

COPY . .

# Collect static files
RUN python manage.py collectstatic --noinput

# Production stage
FROM python:{{.pythonVersion | default "3.12"}}-slim AS runner

WORKDIR /app

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    libpq5 \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN useradd --create-home --shell /bin/bash django

# Copy installed packages and app
COPY --from=builder /usr/local/lib/python{{.pythonVersion | default "3.12"}}/site-packages /usr/local/lib/python{{.pythonVersion | default "3.12"}}/site-packages
COPY --from=builder /app /app

# Set ownership
RUN chown -R django:django /app

USER django

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1

EXPOSE {{.port | default "8000"}}

{{if eq .wsgiServer "gunicorn"}}
CMD ["gunicorn", "--bind", "0.0.0.0:{{.port | default "8000"}}", "--workers", "2", "--threads", "4", "{{.projectName | default "config"}}.wsgi:application"]
{{else if eq .wsgiServer "uvicorn"}}
CMD ["uvicorn", "{{.projectName | default "config"}}.asgi:application", "--host", "0.0.0.0", "--port", "{{.port | default "8000"}}"]
{{else}}
CMD ["gunicorn", "--bind", "0.0.0.0:{{.port | default "8000"}}", "--workers", "2", "{{.projectName | default "config"}}.wsgi:application"]
{{end}}

HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:{{.port | default "8000"}}/health')" || exit 1
`

// FastAPI template
const fastapiTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: FastAPI
# https://github.com/dublyo/dockerizer
# ============================================

FROM python:{{.pythonVersion | default "3.12"}}-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

{{if eq .packageManager "poetry"}}
RUN pip install poetry
COPY pyproject.toml poetry.lock* ./
RUN poetry config virtualenvs.create false && poetry install --no-dev --no-interaction --no-ansi
{{else if eq .packageManager "pipenv"}}
RUN pip install pipenv
COPY Pipfile Pipfile.lock* ./
RUN pipenv install --system --deploy --ignore-pipfile
{{else}}
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
{{end}}

COPY . .

# Create non-root user
RUN useradd --create-home --shell /bin/bash appuser
RUN chown -R appuser:appuser /app
USER appuser

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1

EXPOSE {{.port | default "8000"}}

CMD ["uvicorn", "{{.moduleName | default "main"}}:app", "--host", "0.0.0.0", "--port", "{{.port | default "8000"}}"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:{{.port | default "8000"}}/health')" || exit 1
`

// Flask template
const flaskTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Flask
# https://github.com/dublyo/dockerizer
# ============================================

FROM python:{{.pythonVersion | default "3.12"}}-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

{{if eq .packageManager "poetry"}}
RUN pip install poetry
COPY pyproject.toml poetry.lock* ./
RUN poetry config virtualenvs.create false && poetry install --no-dev --no-interaction --no-ansi
{{else if eq .packageManager "pipenv"}}
RUN pip install pipenv
COPY Pipfile Pipfile.lock* ./
RUN pipenv install --system --deploy --ignore-pipfile
{{else}}
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
{{end}}

COPY . .

# Create non-root user
RUN useradd --create-home --shell /bin/bash flask
RUN chown -R flask:flask /app
USER flask

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV FLASK_APP={{.mainFile | default "app.py"}}
ENV FLASK_ENV=production

EXPOSE {{.port | default "5000"}}

{{if eq .wsgiServer "gunicorn"}}
CMD ["gunicorn", "--bind", "0.0.0.0:{{.port | default "5000"}}", "--workers", "2", "--threads", "4", "{{.moduleName | default "app"}}:app"]
{{else}}
CMD ["gunicorn", "--bind", "0.0.0.0:{{.port | default "5000"}}", "--workers", "2", "{{.moduleName | default "app"}}:app"]
{{end}}

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:{{.port | default "5000"}}/health')" || exit 1
`

// Gin template
const ginTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Gin
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM golang:{{.goVersion | default "1.22"}}-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/server {{.mainPath | default "."}}

# Production stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Copy binary
COPY --from=builder /app/server /app/server

# Set ownership
RUN chown -R appuser:appgroup /app

USER appuser

EXPOSE {{.port | default "8080"}}

CMD ["/app/server"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "8080"}}/health || exit 1
`

// Fiber template
const fiberTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Fiber
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM golang:{{.goVersion | default "1.22"}}-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/server {{.mainPath | default "."}}

# Production stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Copy binary
COPY --from=builder /app/server /app/server

RUN chown -R appuser:appgroup /app

USER appuser

EXPOSE {{.port | default "3000"}}

CMD ["/app/server"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "3000"}}/health || exit 1
`

// Echo template
const echoTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Echo
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM golang:{{.goVersion | default "1.22"}}-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/server {{.mainPath | default "."}}

# Production stage
FROM alpine:latest

WORKDIR /app

RUN apk --no-cache add ca-certificates

RUN addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /app/server /app/server

RUN chown -R appuser:appgroup /app

USER appuser

EXPOSE {{.port | default "8080"}}

CMD ["/app/server"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "8080"}}/health || exit 1
`

// Go standard library template
const goStandardTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Runtime: Go (Standard Library)
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM golang:{{.goVersion | default "1.22"}}-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/server {{.mainPath | default "."}}

# Production stage
FROM alpine:latest

WORKDIR /app

RUN apk --no-cache add ca-certificates

RUN addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /app/server /app/server

RUN chown -R appuser:appgroup /app

USER appuser

EXPOSE {{.port | default "8080"}}

CMD ["/app/server"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "8080"}}/health || exit 1
`

// Actix template
const actixTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Actix Web
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM rust:{{.rustVersion | default "1.75"}}-slim AS builder

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config \
    libssl-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy manifest files
COPY Cargo.toml Cargo.lock* ./

# Create dummy source to cache dependencies
RUN mkdir src && echo "fn main() {}" > src/main.rs
RUN cargo build --release
RUN rm -rf src

# Copy actual source code
COPY . .

# Build the application
RUN touch src/main.rs && cargo build --release

# Production stage
FROM debian:bookworm-slim

WORKDIR /app

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libssl3 \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN useradd --create-home --shell /bin/bash appuser

# Copy binary
COPY --from=builder /app/target/release/{{.projectName | default "app"}} /app/server

RUN chown -R appuser:appuser /app

USER appuser

EXPOSE {{.port | default "8080"}}

CMD ["/app/server"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD curl -f http://localhost:{{.port | default "8080"}}/health || exit 1
`

// Axum template
const axumTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Axum
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM rust:{{.rustVersion | default "1.75"}}-slim AS builder

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config \
    libssl-dev \
    && rm -rf /var/lib/apt/lists/*

COPY Cargo.toml Cargo.lock* ./

RUN mkdir src && echo "fn main() {}" > src/main.rs
RUN cargo build --release
RUN rm -rf src

COPY . .

RUN touch src/main.rs && cargo build --release

# Production stage
FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libssl3 \
    curl \
    && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --shell /bin/bash appuser

COPY --from=builder /app/target/release/{{.projectName | default "app"}} /app/server

RUN chown -R appuser:appuser /app

USER appuser

EXPOSE {{.port | default "8080"}}

CMD ["/app/server"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD curl -f http://localhost:{{.port | default "8080"}}/health || exit 1
`

// Ruby dockerignore
const rubyDockerignore = `
# Ruby specific
*.gem
*.rbc
/.bundle
/vendor/bundle
/log/*
/tmp/*
/db/*.sqlite3
/db/*.sqlite3-*
/public/system
/coverage/
/spec/tmp
*.orig

# Environment
.env
.env.local
/.env*.local
`

// PHP dockerignore
const phpDockerignore = `
# PHP specific
/vendor/
*.log
/storage/*.key
.phpunit.result.cache
/node_modules/
/public/hot
/public/storage
/storage/*.key

# Environment
.env
.env.backup
.env.production
`

// Java dockerignore
const javaDockerignore = `
# Java specific
target/
build/
*.class
*.jar
*.war
*.ear
*.logs
*.iml
.gradle/
.idea/
*.hprof

# Environment
.env
.env.local
application-local.properties
application-local.yml
`

const dotnetDockerignore = `
# .NET specific
bin/
obj/
*.user
*.suo
*.userosscache
*.sln.docstates
.vs/
*.nupkg
*.snupkg
project.lock.json
project.fragment.lock.json

# Build results
[Dd]ebug/
[Rr]elease/
x64/
x86/
[Aa][Rr][Mm]/
[Aa][Rr][Mm]64/
bld/
[Bb]in/
[Oo]bj/

# NuGet
*.nupkg
**/[Pp]ackages/*
!**/[Pp]ackages/build/

# Environment
.env
appsettings.*.json
!appsettings.json
`

const elixirDockerignore = `
# Elixir specific
_build/
deps/
*.ez
*.beam
.fetch
erl_crash.dump

# Mix artifacts
mix.lock.local
.mix/

# Phoenix
/priv/static/assets/
/priv/static/cache_manifest.json
node_modules/

# Environment
.env
config/*.secret.exs
config/dev.secret.exs
config/prod.secret.exs
config/test.secret.exs
`

// NestJS template
const nestjsTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: NestJS
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM node:{{.nodeVersion | default "20"}}-alpine AS builder

WORKDIR /app

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY yarn.lock ./
{{else}}
COPY package-lock.json ./
{{end}}

COPY package.json ./
COPY tsconfig*.json ./

{{if eq .packageManager "pnpm"}}
RUN pnpm install --frozen-lockfile
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile
{{else}}
RUN npm ci
{{end}}

COPY . .

{{if eq .packageManager "pnpm"}}
RUN pnpm build
{{else if eq .packageManager "yarn"}}
RUN yarn build
{{else}}
RUN npm run build
{{end}}

# Production stage
FROM node:{{.nodeVersion | default "20"}}-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production

# Create non-root user
RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nestjs

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY --from=builder /app/pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY --from=builder /app/yarn.lock ./
{{else}}
COPY --from=builder /app/package-lock.json ./
{{end}}

COPY --from=builder /app/package.json ./
COPY --from=builder /app/dist ./dist

{{if eq .packageManager "pnpm"}}
RUN pnpm install --frozen-lockfile --prod
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile --production
{{else}}
RUN npm ci --only=production
{{end}}

USER nestjs

EXPOSE {{.port | default "3000"}}
ENV PORT={{.port | default "3000"}}

CMD ["node", "dist/main.js"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "3000"}}/health || exit 1
`

// Nuxt template
const nuxtTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Nuxt.js
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM node:{{.nodeVersion | default "20"}}-alpine AS builder

WORKDIR /app

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY yarn.lock ./
{{else}}
COPY package-lock.json ./
{{end}}

COPY package.json ./

{{if eq .packageManager "pnpm"}}
RUN pnpm install --frozen-lockfile
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile
{{else}}
RUN npm ci
{{end}}

COPY . .

{{if eq .packageManager "pnpm"}}
RUN pnpm build
{{else if eq .packageManager "yarn"}}
RUN yarn build
{{else}}
RUN npm run build
{{end}}

# Production stage
FROM node:{{.nodeVersion | default "20"}}-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production

# Create non-root user
RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nuxtjs

{{if eq .nuxtVersion "3"}}
# Nuxt 3 output
COPY --from=builder /app/.output ./.output

USER nuxtjs

EXPOSE {{.port | default "3000"}}
ENV PORT={{.port | default "3000"}}
ENV HOST=0.0.0.0

CMD ["node", ".output/server/index.mjs"]
{{else}}
# Nuxt 2 output
COPY --from=builder /app/.nuxt ./.nuxt
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/package.json ./

USER nuxtjs

EXPOSE {{.port | default "3000"}}
ENV PORT={{.port | default "3000"}}
ENV HOST=0.0.0.0

CMD ["npm", "start"]
{{end}}

HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "3000"}}/health || exit 1
`

// Rails template
const railsTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Ruby on Rails
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM ruby:{{.rubyVersion | default "3.3"}}-slim AS builder

WORKDIR /app

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    libpq-dev \
    nodejs \
    npm \
    git \
    && rm -rf /var/lib/apt/lists/*

# Install bundler
RUN gem install bundler

# Install gems
COPY Gemfile Gemfile.lock ./
RUN bundle config set --local deployment 'true' && \
    bundle config set --local without 'development test' && \
    bundle install --jobs 4 --retry 3

# Copy application
COPY . .

{{if .hasAssets}}
# Precompile assets
RUN SECRET_KEY_BASE=dummy bundle exec rails assets:precompile
{{end}}

# Production stage
FROM ruby:{{.rubyVersion | default "3.3"}}-slim AS runner

WORKDIR /app

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    libpq5 \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN useradd --create-home --shell /bin/bash rails

# Copy gems and app
COPY --from=builder /usr/local/bundle /usr/local/bundle
COPY --from=builder /app /app

# Set ownership
RUN chown -R rails:rails /app

USER rails

ENV RAILS_ENV=production
ENV RAILS_LOG_TO_STDOUT=true
ENV RAILS_SERVE_STATIC_FILES=true

EXPOSE {{.port | default "3000"}}

CMD ["bundle", "exec", "rails", "server", "-b", "0.0.0.0", "-p", "{{.port | default "3000"}}"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
  CMD curl -f http://localhost:{{.port | default "3000"}}/health || exit 1
`

// Laravel template
const laravelTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Laravel
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM php:{{.phpVersion | default "8.3"}}-fpm-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache \
    git \
    curl \
    libpng-dev \
    oniguruma-dev \
    libxml2-dev \
    zip \
    unzip \
    nodejs \
    npm

# Install PHP extensions
RUN docker-php-ext-install pdo_mysql mbstring exif pcntl bcmath gd

# Install Composer
COPY --from=composer:latest /usr/bin/composer /usr/bin/composer

# Copy composer files
COPY composer.json composer.lock ./

# Install dependencies
RUN composer install --no-dev --no-scripts --no-autoloader --prefer-dist

# Copy application
COPY . .

# Generate optimized autoloader
RUN composer dump-autoload --optimize

{{if .hasVite}}
# Build frontend assets
RUN npm ci && npm run build
{{else if .hasMix}}
RUN npm ci && npm run production
{{end}}

# Production stage
FROM php:{{.phpVersion | default "8.3"}}-fpm-alpine AS runner

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache \
    libpng \
    oniguruma \
    libxml2 \
    nginx \
    supervisor \
    curl

# Install PHP extensions
RUN docker-php-ext-install pdo_mysql mbstring exif pcntl bcmath gd opcache

# Create non-root user
RUN addgroup -S laravel && adduser -S laravel -G laravel

# Copy application
COPY --from=builder /app /app
COPY --from=builder /usr/bin/composer /usr/bin/composer

# Set permissions
RUN chown -R laravel:laravel /app \
    && chmod -R 775 /app/storage /app/bootstrap/cache

# Create nginx config
RUN echo 'server { \
    listen 8000; \
    server_name _; \
    root /app/public; \
    index index.php; \
    location / { \
        try_files $uri $uri/ /index.php?$query_string; \
    } \
    location ~ \.php$ { \
        fastcgi_pass 127.0.0.1:9000; \
        fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name; \
        include fastcgi_params; \
    } \
}' > /etc/nginx/http.d/default.conf

# Create supervisor config
RUN echo '[supervisord] \
nodaemon=true \
user=root \
[program:php-fpm] \
command=php-fpm -F \
autostart=true \
autorestart=true \
[program:nginx] \
command=nginx -g "daemon off;" \
autostart=true \
autorestart=true' > /etc/supervisord.conf

EXPOSE {{.port | default "8000"}}

CMD ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
  CMD curl -f http://localhost:{{.port | default "8000"}}/health || exit 1
`

// Spring Boot template
const springbootTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Spring Boot
# https://github.com/dublyo/dockerizer
# ============================================

{{if eq .buildTool "maven"}}
# Build stage (Maven)
FROM eclipse-temurin:{{.javaVersion | default "21"}}-jdk-alpine AS builder

WORKDIR /app

# Copy Maven wrapper and pom
COPY .mvn/ .mvn/
COPY mvnw pom.xml ./

# Download dependencies
RUN chmod +x ./mvnw && ./mvnw dependency:go-offline -B

# Copy source and build
COPY src ./src
RUN ./mvnw package -DskipTests -B

{{else}}
# Build stage (Gradle)
FROM eclipse-temurin:{{.javaVersion | default "21"}}-jdk-alpine AS builder

WORKDIR /app

# Copy Gradle wrapper and build files
COPY gradlew ./
COPY gradle ./gradle
COPY build.gradle* settings.gradle* ./

# Download dependencies
RUN chmod +x ./gradlew && ./gradlew dependencies --no-daemon

# Copy source and build
COPY src ./src
RUN ./gradlew bootJar --no-daemon -x test

{{end}}

# Production stage
FROM eclipse-temurin:{{.javaVersion | default "21"}}-jre-alpine AS runner

WORKDIR /app

# Create non-root user
RUN addgroup -S spring && adduser -S spring -G spring

{{if eq .buildTool "maven"}}
# Copy JAR from Maven build
COPY --from=builder /app/target/*.jar app.jar
{{else}}
# Copy JAR from Gradle build
COPY --from=builder /app/build/libs/*.jar app.jar
{{end}}

# Set ownership
RUN chown -R spring:spring /app

USER spring

# JVM options for containers
ENV JAVA_OPTS="-XX:+UseContainerSupport -XX:MaxRAMPercentage=75.0"

EXPOSE {{.port | default "8080"}}

ENTRYPOINT ["sh", "-c", "java $JAVA_OPTS -jar app.jar"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "8080"}}/actuator/health || exit 1
`

// Remix template
const remixTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Remix
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM node:{{.nodeVersion | default "20"}}-alpine AS builder

WORKDIR /app

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY yarn.lock ./
{{else}}
COPY package-lock.json ./
{{end}}

COPY package.json ./

{{if eq .packageManager "pnpm"}}
RUN pnpm install --frozen-lockfile
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile
{{else}}
RUN npm ci
{{end}}

COPY . .

RUN npm run build

# Production stage
FROM node:{{.nodeVersion | default "20"}}-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production

# Create non-root user
RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 remix

{{if eq .packageManager "pnpm"}}
COPY --from=builder /app/pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY --from=builder /app/yarn.lock ./
{{else}}
COPY --from=builder /app/package-lock.json ./
{{end}}

COPY --from=builder /app/package.json ./
COPY --from=builder /app/build ./build
COPY --from=builder /app/public ./public

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
RUN pnpm install --frozen-lockfile --prod
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile --production
{{else}}
RUN npm ci --only=production
{{end}}

USER remix

EXPOSE {{.port | default "3000"}}
ENV PORT={{.port | default "3000"}}

CMD ["npm", "start"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "3000"}}/health || exit 1
`

// Astro template
const astroTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Astro
# https://github.com/dublyo/dockerizer
# ============================================

{{if eq .outputMode "static"}}
# Build stage
FROM node:{{.nodeVersion | default "20"}}-alpine AS builder

WORKDIR /app

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY yarn.lock ./
{{else}}
COPY package-lock.json ./
{{end}}

COPY package.json ./

{{if eq .packageManager "pnpm"}}
RUN pnpm install --frozen-lockfile
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile
{{else}}
RUN npm ci
{{end}}

COPY . .

RUN npm run build

# Production stage - static file serving with nginx
FROM nginx:alpine AS runner

COPY --from=builder /app/dist /usr/share/nginx/html

# Custom nginx config for SPA routing
RUN echo 'server { \
    listen 80; \
    server_name _; \
    root /usr/share/nginx/html; \
    index index.html; \
    location / { \
        try_files $uri $uri/ /index.html; \
    } \
    gzip on; \
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml; \
}' > /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost/ || exit 1
{{else}}
# Build stage (SSR mode)
FROM node:{{.nodeVersion | default "20"}}-alpine AS builder

WORKDIR /app

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY yarn.lock ./
{{else}}
COPY package-lock.json ./
{{end}}

COPY package.json ./

{{if eq .packageManager "pnpm"}}
RUN pnpm install --frozen-lockfile
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile
{{else}}
RUN npm ci
{{end}}

COPY . .

RUN npm run build

# Production stage (SSR)
FROM node:{{.nodeVersion | default "20"}}-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production
ENV HOST=0.0.0.0

# Create non-root user
RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 astro

{{if eq .packageManager "pnpm"}}
COPY --from=builder /app/pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY --from=builder /app/yarn.lock ./
{{else}}
COPY --from=builder /app/package-lock.json ./
{{end}}

COPY --from=builder /app/package.json ./
COPY --from=builder /app/dist ./dist

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
RUN pnpm install --frozen-lockfile --prod
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile --production
{{else}}
RUN npm ci --only=production
{{end}}

USER astro

EXPOSE {{.port | default "4321"}}
ENV PORT={{.port | default "4321"}}

CMD ["node", "./dist/server/entry.mjs"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "4321"}}/ || exit 1
{{end}}
`

// SvelteKit template
const sveltekitTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: SvelteKit
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM node:{{.nodeVersion | default "20"}}-alpine AS builder

WORKDIR /app

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY yarn.lock ./
{{else}}
COPY package-lock.json ./
{{end}}

COPY package.json ./

{{if eq .packageManager "pnpm"}}
RUN pnpm install --frozen-lockfile
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile
{{else}}
RUN npm ci
{{end}}

COPY . .

RUN npm run build

# Production stage
FROM node:{{.nodeVersion | default "20"}}-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production

# Create non-root user
RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 sveltekit

{{if eq .packageManager "pnpm"}}
COPY --from=builder /app/pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY --from=builder /app/yarn.lock ./
{{else}}
COPY --from=builder /app/package-lock.json ./
{{end}}

COPY --from=builder /app/package.json ./
COPY --from=builder /app/build ./build

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
RUN pnpm install --frozen-lockfile --prod
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile --production
{{else}}
RUN npm ci --only=production
{{end}}

USER sveltekit

EXPOSE {{.port | default "3000"}}
ENV PORT={{.port | default "3000"}}

CMD ["node", "build"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "3000"}}/ || exit 1
`

// Hono template
const honoTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Hono
# https://github.com/dublyo/dockerizer
# ============================================

{{if eq .runtime "bun"}}
# Bun runtime
FROM oven/bun:1 AS builder

WORKDIR /app

COPY bun.lockb package.json ./
RUN bun install --frozen-lockfile

COPY . .
{{if .typescript}}
RUN bun build ./src/index.ts --outdir ./dist --target bun
{{end}}

FROM oven/bun:1-alpine AS runner

WORKDIR /app

RUN addgroup -S hono && adduser -S hono -G hono

COPY --from=builder /app/package.json ./
COPY --from=builder /app/node_modules ./node_modules
{{if .typescript}}
COPY --from=builder /app/dist ./dist
{{else}}
COPY --from=builder /app/src ./src
{{end}}

USER hono

EXPOSE {{.port | default "3000"}}
ENV PORT={{.port | default "3000"}}

{{if .typescript}}
CMD ["bun", "run", "dist/index.js"]
{{else}}
CMD ["bun", "run", "src/index.ts"]
{{end}}
{{else}}
# Node.js runtime
FROM node:{{.nodeVersion | default "20"}}-alpine AS builder

WORKDIR /app

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY yarn.lock ./
{{else}}
COPY package-lock.json ./
{{end}}

COPY package.json tsconfig*.json ./

{{if eq .packageManager "pnpm"}}
RUN pnpm install --frozen-lockfile
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile
{{else}}
RUN npm ci
{{end}}

COPY . .

{{if .typescript}}
RUN npm run build
{{end}}

FROM node:{{.nodeVersion | default "20"}}-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production

RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 hono

{{if eq .packageManager "pnpm"}}
COPY --from=builder /app/pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY --from=builder /app/yarn.lock ./
{{else}}
COPY --from=builder /app/package-lock.json ./
{{end}}

COPY --from=builder /app/package.json ./
{{if .typescript}}
COPY --from=builder /app/dist ./dist
{{else}}
COPY --from=builder /app/src ./src
{{end}}

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
RUN pnpm install --frozen-lockfile --prod
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile --production
{{else}}
RUN npm ci --only=production
{{end}}

USER hono

EXPOSE {{.port | default "3000"}}
ENV PORT={{.port | default "3000"}}

{{if .typescript}}
CMD ["node", "dist/index.js"]
{{else}}
CMD ["node", "{{.mainEntry | default "src/index.js"}}"]
{{end}}
{{end}}

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "3000"}}/health || exit 1
`

// Koa template
const koaTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Koa
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM node:{{.nodeVersion | default "20"}}-alpine AS builder

WORKDIR /app

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY yarn.lock ./
{{else}}
COPY package-lock.json ./
{{end}}

COPY package.json ./
{{if .typescript}}
COPY tsconfig*.json ./
{{end}}

{{if eq .packageManager "pnpm"}}
RUN pnpm install --frozen-lockfile
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile
{{else}}
RUN npm ci
{{end}}

COPY . .

{{if .typescript}}
RUN npm run build
{{end}}

# Production stage
FROM node:{{.nodeVersion | default "20"}}-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production

# Create non-root user
RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 koa

{{if eq .packageManager "pnpm"}}
COPY --from=builder /app/pnpm-lock.yaml ./
{{else if eq .packageManager "yarn"}}
COPY --from=builder /app/yarn.lock ./
{{else}}
COPY --from=builder /app/package-lock.json ./
{{end}}

COPY --from=builder /app/package.json ./
{{if .typescript}}
COPY --from=builder /app/dist ./dist
{{else}}
COPY --from=builder /app/src ./src
COPY --from=builder /app/*.js ./
{{end}}

{{if eq .packageManager "pnpm"}}
RUN corepack enable && corepack prepare pnpm@latest --activate
RUN pnpm install --frozen-lockfile --prod
{{else if eq .packageManager "yarn"}}
RUN yarn install --frozen-lockfile --production
{{else}}
RUN npm ci --only=production
{{end}}

USER koa

EXPOSE {{.port | default "3000"}}
ENV PORT={{.port | default "3000"}}

{{if .hasStartScript}}
CMD ["npm", "start"]
{{else if .typescript}}
CMD ["node", "dist/index.js"]
{{else}}
CMD ["node", "{{.mainEntry | default "app.js"}}"]
{{end}}

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "3000"}}/health || exit 1
`

// Symfony template
const symfonyTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Symfony
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM php:{{.phpVersion | default "8.3"}}-fpm-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache \
    git \
    curl \
    libpng-dev \
    libxml2-dev \
    zip \
    unzip \
    icu-dev \
    oniguruma-dev

# Install PHP extensions
RUN docker-php-ext-install pdo_mysql mbstring intl opcache

# Install Composer
COPY --from=composer:latest /usr/bin/composer /usr/bin/composer

# Copy composer files
COPY composer.json composer.lock symfony.lock ./

# Install dependencies
RUN composer install --no-dev --no-scripts --no-autoloader --prefer-dist

# Copy application
COPY . .

# Generate optimized autoloader and run scripts
RUN composer dump-autoload --optimize --classmap-authoritative
RUN php bin/console cache:clear --env=prod --no-debug
RUN php bin/console cache:warmup --env=prod --no-debug

{{if .hasEncore}}
# Build assets with Encore
RUN apk add --no-cache nodejs npm
RUN npm install && npm run build
{{end}}

# Production stage
FROM php:{{.phpVersion | default "8.3"}}-fpm-alpine AS runner

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache \
    libpng \
    libxml2 \
    icu-libs \
    nginx \
    supervisor \
    curl

# Install PHP extensions
RUN docker-php-ext-install pdo_mysql mbstring intl opcache

# Create non-root user
RUN addgroup -S symfony && adduser -S symfony -G symfony

# Copy application
COPY --from=builder /app /app

# Set permissions
RUN chown -R symfony:symfony /app \
    && chmod -R 775 /app/var

# Configure PHP-FPM
RUN echo '[www]' > /usr/local/etc/php-fpm.d/www.conf && \
    echo 'user = symfony' >> /usr/local/etc/php-fpm.d/www.conf && \
    echo 'group = symfony' >> /usr/local/etc/php-fpm.d/www.conf && \
    echo 'listen = 127.0.0.1:9000' >> /usr/local/etc/php-fpm.d/www.conf && \
    echo 'pm = dynamic' >> /usr/local/etc/php-fpm.d/www.conf && \
    echo 'pm.max_children = 5' >> /usr/local/etc/php-fpm.d/www.conf && \
    echo 'pm.start_servers = 2' >> /usr/local/etc/php-fpm.d/www.conf && \
    echo 'pm.min_spare_servers = 1' >> /usr/local/etc/php-fpm.d/www.conf && \
    echo 'pm.max_spare_servers = 3' >> /usr/local/etc/php-fpm.d/www.conf

# Create nginx config
RUN echo 'server { \
    listen 8000; \
    server_name _; \
    root /app/public; \
    index index.php; \
    location / { \
        try_files $uri /index.php$is_args$args; \
    } \
    location ~ ^/index\.php(/|$) { \
        fastcgi_pass 127.0.0.1:9000; \
        fastcgi_split_path_info ^(.+\.php)(/.*)$; \
        include fastcgi_params; \
        fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name; \
        fastcgi_param DOCUMENT_ROOT $realpath_root; \
        internal; \
    } \
    location ~ \.php$ { \
        return 404; \
    } \
}' > /etc/nginx/http.d/default.conf

# Create supervisor config
RUN echo '[supervisord]' > /etc/supervisord.conf && \
    echo 'nodaemon=true' >> /etc/supervisord.conf && \
    echo 'user=root' >> /etc/supervisord.conf && \
    echo '' >> /etc/supervisord.conf && \
    echo '[program:php-fpm]' >> /etc/supervisord.conf && \
    echo 'command=php-fpm -F' >> /etc/supervisord.conf && \
    echo 'autostart=true' >> /etc/supervisord.conf && \
    echo 'autorestart=true' >> /etc/supervisord.conf && \
    echo '' >> /etc/supervisord.conf && \
    echo '[program:nginx]' >> /etc/supervisord.conf && \
    echo 'command=nginx -g "daemon off;"' >> /etc/supervisord.conf && \
    echo 'autostart=true' >> /etc/supervisord.conf && \
    echo 'autorestart=true' >> /etc/supervisord.conf

EXPOSE {{.port | default "8000"}}

CMD ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
  CMD curl -f http://localhost:{{.port | default "8000"}}/ || exit 1
`

// ASP.NET Core template
const aspnetTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: ASP.NET Core
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM mcr.microsoft.com/dotnet/sdk:{{.dotnetVersion | default "8.0"}}-alpine AS builder

WORKDIR /src

# Copy project files
{{if .solutionFile}}
COPY {{.solutionFile}} ./
{{end}}
COPY *.csproj ./

# Restore dependencies
RUN dotnet restore

# Copy all source files
COPY . .

# Build and publish
RUN dotnet publish -c Release -o /app/publish --no-restore

# Production stage
FROM mcr.microsoft.com/dotnet/aspnet:{{.dotnetVersion | default "8.0"}}-alpine AS runner

WORKDIR /app

# Create non-root user
RUN addgroup -S dotnet && adduser -S aspnet -G dotnet

# Copy published app
COPY --from=builder /app/publish .

# Set ownership
RUN chown -R aspnet:dotnet /app

USER aspnet

# ASP.NET Core configuration
ENV ASPNETCORE_URLS=http://+:{{.port | default "8080"}}
ENV ASPNETCORE_ENVIRONMENT=Production
ENV DOTNET_RUNNING_IN_CONTAINER=true

EXPOSE {{.port | default "8080"}}

# Get the DLL name from project file (defaults to app.dll)
ENTRYPOINT ["dotnet", "{{.projectName | default "app"}}.dll"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "8080"}}/health || exit 1
`

// Phoenix template
const phoenixTemplate = `# ============================================
# Dockerfile generated by Dublyo Dockerizer
# Framework: Phoenix (Elixir)
# https://github.com/dublyo/dockerizer
# ============================================

# Build stage
FROM elixir:{{.elixirVersion | default "1.16"}}-alpine AS builder

# Install build dependencies
RUN apk add --no-cache build-base git npm

WORKDIR /app

# Install hex and rebar
RUN mix local.hex --force && mix local.rebar --force

# Set build environment
ENV MIX_ENV=prod

# Copy mix files
COPY mix.exs mix.lock ./
COPY config config

# Install dependencies
RUN mix deps.get --only prod
RUN mix deps.compile

{{if .hasAssets}}
# Build assets
COPY assets assets
COPY priv priv
RUN cd assets && npm install && npm run deploy
RUN mix phx.digest
{{else}}
COPY priv priv
{{end}}

# Copy application code
COPY lib lib

# Compile application
RUN mix compile

# Build release
RUN mix release

# Production stage
FROM alpine:3.19 AS runner

# Install runtime dependencies
RUN apk add --no-cache libstdc++ openssl ncurses-libs

WORKDIR /app

# Create non-root user
RUN addgroup -S phoenix && adduser -S phoenix -G phoenix

# Copy release from builder
COPY --from=builder /app/_build/prod/rel/{{.appName | default "app"}} ./

# Set ownership
RUN chown -R phoenix:phoenix /app

USER phoenix

# Runtime configuration
ENV HOME=/app
ENV MIX_ENV=prod
ENV PHX_SERVER=true
ENV PORT={{.port | default "4000"}}

EXPOSE {{.port | default "4000"}}

CMD ["bin/{{.appName | default "app"}}", "start"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:{{.port | default "4000"}}/ || exit 1
`
