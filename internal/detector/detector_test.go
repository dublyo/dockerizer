package detector_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dublyo/dockerizer/internal/detector"
	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers/dotnet"
	"github.com/dublyo/dockerizer/providers/elixir"
	"github.com/dublyo/dockerizer/providers/golang"
	"github.com/dublyo/dockerizer/providers/java"
	"github.com/dublyo/dockerizer/providers/nodejs"
	"github.com/dublyo/dockerizer/providers/php"
	"github.com/dublyo/dockerizer/providers/python"
	"github.com/dublyo/dockerizer/providers/ruby"
	"github.com/dublyo/dockerizer/providers/rust"
)

type detectionCase struct {
	name         string
	path         string
	detected     bool
	language     string
	framework    string
	expectedVars map[string]string
}

func TestDetectSampleApps(t *testing.T) {
	root := findTestRoot(t)

	registry := detector.NewRegistry()
	nodejs.RegisterAll(registry)
	python.RegisterAll(registry)
	golang.RegisterAll(registry)
	rust.RegisterAll(registry)
	ruby.RegisterAll(registry)
	php.RegisterAll(registry)
	java.RegisterAll(registry)
	dotnet.RegisterAll(registry)
	elixir.RegisterAll(registry)

	det := detector.New(registry)

	cases := []detectionCase{
		// Node.js
		{name: "nextjs", path: "nextjs-app", detected: true, language: "nodejs", framework: "nextjs"},
		{name: "express", path: "express-app", detected: true, language: "nodejs", framework: "express"},
		{name: "nestjs", path: "nestjs-app", detected: true, language: "nodejs", framework: "nestjs"},
		{name: "nuxt", path: "nuxt-app", detected: true, language: "nodejs", framework: "nuxt"},
		{name: "remix", path: "remix-app", detected: true, language: "nodejs", framework: "remix"},
		{name: "astro", path: "astro-app", detected: true, language: "nodejs", framework: "astro"},
		{name: "sveltekit", path: "sveltekit-app", detected: true, language: "nodejs", framework: "sveltekit"},
		{name: "hono", path: "hono-app", detected: true, language: "nodejs", framework: "hono"},
		{name: "koa", path: "koa-app", detected: true, language: "nodejs", framework: "koa"},
		{name: "fastify", path: "fastify-app", detected: true, language: "nodejs", framework: "fastify"},
		{name: "express-bun", path: "express-bun-app", detected: true, language: "nodejs", framework: "express", expectedVars: map[string]string{"packageManager": "bun"}},
		{name: "fastify-pnpm", path: "fastify-pnpm-app", detected: true, language: "nodejs", framework: "fastify", expectedVars: map[string]string{"packageManager": "pnpm"}},
		{name: "koa-yarn", path: "koa-yarn-app", detected: true, language: "nodejs", framework: "koa", expectedVars: map[string]string{"packageManager": "yarn"}},

		// Python
		{name: "django", path: "django-app", detected: true, language: "python", framework: "django"},
		{name: "fastapi", path: "fastapi-app", detected: true, language: "python", framework: "fastapi"},
		{name: "flask", path: "flask-app", detected: true, language: "python", framework: "flask"},
		{name: "fastapi-uv", path: "fastapi-uv-app", detected: true, language: "python", framework: "fastapi", expectedVars: map[string]string{"packageManager": "uv"}},

		// Go
		{name: "gin", path: "gin-app", detected: true, language: "go", framework: "gin"},
		{name: "echo", path: "echo-app", detected: true, language: "go", framework: "echo"},
		{name: "fiber", path: "fiber-app", detected: true, language: "go", framework: "fiber"},
		{name: "go-standard", path: "go-standard-app", detected: true, language: "go", framework: "standard"},

		// Rust
		{name: "actix", path: "actix-app", detected: true, language: "rust", framework: "actix-web"},
		{name: "axum", path: "axum-app", detected: true, language: "rust", framework: "axum"},

		// Ruby
		{name: "rails", path: "rails-app", detected: true, language: "ruby", framework: "rails"},

		// PHP
		{name: "laravel", path: "laravel-app", detected: true, language: "php", framework: "laravel"},
		{name: "symfony", path: "symfony-app", detected: true, language: "php", framework: "symfony"},

		// Java
		{name: "springboot", path: "springboot-app", detected: true, language: "java", framework: "springboot"},
		{name: "quarkus", path: "quarkus-app", detected: true, language: "java", framework: "quarkus"},

		// .NET
		{name: "aspnet", path: "aspnet-app", detected: true, language: "dotnet", framework: "aspnet"},
		{name: "dotnet-sln", path: "dotnet-sln-app", detected: true, language: "dotnet", framework: "aspnet", expectedVars: map[string]string{"solutionFile": "MySolution.sln", "hasDirectoryBuildProps": "true", "hasDirectoryPackagesProps": "true"}},

		// Elixir
		{name: "phoenix", path: "phoenix-app", detected: true, language: "elixir", framework: "phoenix"},

		// Unsupported / AI-only
		{name: "codeigniter", path: "codeigniter-app", detected: false},
		{name: "pyramid", path: "pyramid-app", detected: false},
		{name: "tornado", path: "tornado-app", detected: false},
		{name: "ktor", path: "ktor-app", detected: false},
		{name: "play", path: "play-app", detected: false},
		{name: "unknown", path: "unknown-app", detected: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			appPath := filepath.Join(root, tc.path)
			scan, err := scanner.New().Scan(ctx, appPath)
			if err != nil {
				t.Fatalf("scan failed for %s: %v", tc.path, err)
			}

			result, err := det.Detect(ctx, scan)
			if err != nil {
				t.Fatalf("detect failed for %s: %v", tc.path, err)
			}

			if tc.detected {
				if !result.Detected {
					t.Fatalf("expected detection for %s, got none", tc.path)
				}
				if result.Language != tc.language || result.Framework != tc.framework {
					t.Fatalf("unexpected detection for %s: got %s/%s", tc.path, result.Language, result.Framework)
				}
				for key, expected := range tc.expectedVars {
					got, ok := result.Variables[key]
					if !ok {
						t.Fatalf("missing variable %s for %s", key, tc.path)
					}
					if fmt.Sprint(got) != expected {
						t.Fatalf("unexpected %s for %s: got %v, want %s", key, tc.path, got, expected)
					}
				}
			} else {
				if result.Detected {
					t.Fatalf("expected no detection for %s, got %s/%s", tc.path, result.Language, result.Framework)
				}
			}
		})
	}
}

func findTestRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	dir := wd
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "dockerize-test")
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("could not find dockerize-test directory from %s", wd)
	return ""
}
