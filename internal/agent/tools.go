package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dublyo/dockerizer/internal/detector"
	"github.com/dublyo/dockerizer/internal/generator"
	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers/golang"
	"github.com/dublyo/dockerizer/providers/nodejs"
	"github.com/dublyo/dockerizer/providers/python"
	"github.com/dublyo/dockerizer/providers/rust"
)

// ToolDispatcher manages and executes agent tools
type ToolDispatcher struct {
	workDir string
	tools   map[string]Tool
}

// Tool represents an executable tool
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args map[string]interface{}) (string, error)
}

// NewToolDispatcher creates a new tool dispatcher
func NewToolDispatcher(workDir string) *ToolDispatcher {
	td := &ToolDispatcher{
		workDir: workDir,
		tools:   make(map[string]Tool),
	}

	// Register built-in tools
	td.Register(&DockerBuildTool{workDir: workDir})
	td.Register(&DockerRunTool{workDir: workDir})
	td.Register(&DockerLogsTool{})
	td.Register(&DockerStopTool{})
	td.Register(&FileWriteTool{workDir: workDir})
	td.Register(&FileReadTool{workDir: workDir})
	td.Register(&ShellTool{workDir: workDir})

	// Register dockerizer-specific tools
	td.Register(&DockrizerAnalyzeTool{workDir: workDir})
	td.Register(&DockrizerGenerateTool{workDir: workDir})

	return td
}

// Register adds a tool to the dispatcher
func (td *ToolDispatcher) Register(tool Tool) {
	td.tools[tool.Name()] = tool
}

// Execute runs a tool by name
func (td *ToolDispatcher) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	tool, ok := td.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return tool.Execute(ctx, args)
}

// WriteDockerFiles writes the generated Docker files
func (td *ToolDispatcher) WriteDockerFiles(ctx context.Context, output *Output) error {
	files := map[string]string{
		"Dockerfile":         output.Dockerfile,
		"docker-compose.yml": output.DockerCompose,
		".dockerignore":      output.Dockerignore,
		".env.example":       output.EnvExample,
	}

	for name, content := range files {
		if content == "" {
			continue
		}
		path := filepath.Join(td.workDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", name, err)
		}
	}

	return nil
}

// ListTools returns all registered tools
func (td *ToolDispatcher) ListTools() []Tool {
	tools := make([]Tool, 0, len(td.tools))
	for _, t := range td.tools {
		tools = append(tools, t)
	}
	return tools
}

// DockerBuildTool builds Docker images
type DockerBuildTool struct {
	workDir string
}

func (t *DockerBuildTool) Name() string        { return "docker_build" }
func (t *DockerBuildTool) Description() string { return "Build a Docker image from Dockerfile" }

func (t *DockerBuildTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	dockerfile, _ := args["dockerfile"].(string)
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	tag, _ := args["tag"].(string)
	if tag == "" {
		tag = "dockerize-build:latest"
	}

	cmd := exec.CommandContext(ctx, "docker", "build", "-f", dockerfile, "-t", tag, ".")
	cmd.Dir = t.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String() + stderr.String()

	if err != nil {
		return output, fmt.Errorf("docker build failed: %w\n%s", err, output)
	}

	return output, nil
}

// DockerRunTool runs Docker containers
type DockerRunTool struct {
	workDir string
}

func (t *DockerRunTool) Name() string        { return "docker_run" }
func (t *DockerRunTool) Description() string { return "Run a Docker container for testing" }

func (t *DockerRunTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	image, _ := args["image"].(string)
	if image == "" {
		return "", fmt.Errorf("image is required")
	}

	timeout := 30
	if t, ok := args["timeout"].(int); ok {
		timeout = t
	}

	containerName := fmt.Sprintf("dockerize-test-%d", time.Now().UnixNano())

	// Start container in detached mode
	runCmd := exec.CommandContext(ctx, "docker", "run", "-d", "--name", containerName, image)
	runCmd.Dir = t.workDir

	var stdout bytes.Buffer
	runCmd.Stdout = &stdout
	runCmd.Stderr = &stdout

	if err := runCmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("docker run failed: %w", err)
	}

	// Wait for container to be healthy or timeout
	time.Sleep(time.Duration(timeout) * time.Second)

	// Check container status
	inspectCmd := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Status}}", containerName)
	var inspectOut bytes.Buffer
	inspectCmd.Stdout = &inspectOut

	if err := inspectCmd.Run(); err == nil {
		status := strings.TrimSpace(inspectOut.String())
		if status != "running" {
			// Get logs for debugging
			logsCmd := exec.CommandContext(ctx, "docker", "logs", containerName)
			var logsOut bytes.Buffer
			logsCmd.Stdout = &logsOut
			logsCmd.Stderr = &logsOut
			_ = logsCmd.Run()

			// Cleanup
			_ = exec.CommandContext(ctx, "docker", "rm", "-f", containerName).Run()

			return logsOut.String(), fmt.Errorf("container exited with status: %s", status)
		}
	}

	// Container is running, clean up
	_ = exec.CommandContext(ctx, "docker", "stop", containerName).Run()
	_ = exec.CommandContext(ctx, "docker", "rm", containerName).Run()

	return "Container started and ran successfully", nil
}

// DockerLogsTool gets container logs
type DockerLogsTool struct{}

func (t *DockerLogsTool) Name() string        { return "docker_logs" }
func (t *DockerLogsTool) Description() string { return "Get logs from a Docker container" }

func (t *DockerLogsTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	container, _ := args["container"].(string)
	if container == "" {
		return "", fmt.Errorf("container name is required")
	}

	tail := "100"
	if t, ok := args["tail"].(string); ok {
		tail = t
	}

	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", tail, container)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String() + stderr.String(), err
}

// DockerStopTool stops containers
type DockerStopTool struct{}

func (t *DockerStopTool) Name() string        { return "docker_stop" }
func (t *DockerStopTool) Description() string { return "Stop a Docker container" }

func (t *DockerStopTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	container, _ := args["container"].(string)
	if container == "" {
		return "", fmt.Errorf("container name is required")
	}

	cmd := exec.CommandContext(ctx, "docker", "stop", container)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout

	err := cmd.Run()
	return stdout.String(), err
}

// FileWriteTool writes files
type FileWriteTool struct {
	workDir string
}

func (t *FileWriteTool) Name() string        { return "file_write" }
func (t *FileWriteTool) Description() string { return "Write content to a file" }

func (t *FileWriteTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)

	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	fullPath := filepath.Join(t.workDir, path)

	// Create directories if needed
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Written %d bytes to %s", len(content), path), nil
}

// FileReadTool reads files
type FileReadTool struct {
	workDir string
}

func (t *FileReadTool) Name() string        { return "file_read" }
func (t *FileReadTool) Description() string { return "Read content from a file" }

func (t *FileReadTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	fullPath := filepath.Join(t.workDir, path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}

// ShellTool executes shell commands
type ShellTool struct {
	workDir string
}

func (t *ShellTool) Name() string        { return "shell" }
func (t *ShellTool) Description() string { return "Execute a shell command" }

func (t *ShellTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return "", fmt.Errorf("command is required")
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = t.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String() + stderr.String()

	if err != nil {
		return output, fmt.Errorf("command failed: %w", err)
	}

	return output, nil
}

// DockrizerAnalyzeTool analyzes a repository to detect its stack
type DockrizerAnalyzeTool struct {
	workDir string
}

func (t *DockrizerAnalyzeTool) Name() string        { return "dockerizer_analyze" }
func (t *DockrizerAnalyzeTool) Description() string { return "Analyze a repository to detect its technology stack" }

func (t *DockrizerAnalyzeTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		path = t.workDir
	}

	// Make path absolute if relative
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.workDir, path)
	}

	// Scan repository
	scan, err := scanner.New(scanner.WithIgnoreHidden(false)).Scan(ctx, path)
	if err != nil {
		return "", fmt.Errorf("scan failed: %w", err)
	}

	// Create registry and detect
	registry := detector.NewRegistry()
	nodejs.RegisterAll(registry)
	python.RegisterAll(registry)
	golang.RegisterAll(registry)
	rust.RegisterAll(registry)

	det := detector.New(registry)
	result, err := det.Detect(ctx, scan)
	if err != nil {
		return "", fmt.Errorf("detection failed: %w", err)
	}

	// Build output
	output := map[string]interface{}{
		"detected":   result.Detected,
		"language":   result.Language,
		"framework":  result.Framework,
		"version":    result.Version,
		"confidence": result.Confidence,
		"provider":   result.Provider,
	}

	jsonOutput, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonOutput), nil
}

// DockrizerGenerateTool generates Docker configuration files
type DockrizerGenerateTool struct {
	workDir string
}

func (t *DockrizerGenerateTool) Name() string        { return "dockerizer_generate" }
func (t *DockrizerGenerateTool) Description() string { return "Generate Docker configuration files for a repository" }

func (t *DockrizerGenerateTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		path = t.workDir
	}

	// Make path absolute if relative
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.workDir, path)
	}

	overwrite := false
	if ow, ok := args["overwrite"].(string); ok && ow == "true" {
		overwrite = true
	}

	// Scan repository
	scan, err := scanner.New(scanner.WithIgnoreHidden(false)).Scan(ctx, path)
	if err != nil {
		return "", fmt.Errorf("scan failed: %w", err)
	}

	// Create registry and detect
	registry := detector.NewRegistry()
	nodejs.RegisterAll(registry)
	python.RegisterAll(registry)
	golang.RegisterAll(registry)
	rust.RegisterAll(registry)

	det := detector.New(registry)
	result, err := det.Detect(ctx, scan)
	if err != nil {
		return "", fmt.Errorf("detection failed: %w", err)
	}

	if !result.Detected {
		return "", fmt.Errorf("could not detect project stack")
	}

	// Generate files
	gen := generator.New(
		generator.WithOverwrite(overwrite),
		generator.WithCompose(true),
		generator.WithIgnore(true),
		generator.WithEnv(true),
	)

	output, err := gen.Generate(result, path)
	if err != nil {
		return "", fmt.Errorf("generation failed: %w", err)
	}

	// Build result
	var files []string
	for name := range output.Files {
		files = append(files, name)
	}

	resultOutput := map[string]interface{}{
		"success":  true,
		"language": result.Language,
		"framework": result.Framework,
		"files":    files,
	}

	jsonOutput, err := json.MarshalIndent(resultOutput, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonOutput), nil
}
