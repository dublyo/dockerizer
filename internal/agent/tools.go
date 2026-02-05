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

// securePath validates and resolves a path to ensure it stays within the base directory.
// It rejects absolute paths, path traversal attempts, and symlink escapes.
// This function resolves ALL symlinks in the path chain to prevent intermediate symlink attacks.
func securePath(baseDir, path string) (string, error) {
	// Reject absolute paths
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", path)
	}

	// Resolve the base directory (must exist and we need its real path)
	realBase, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base directory: %w", err)
	}
	realBase, err = filepath.Abs(realBase)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute base path: %w", err)
	}

	// Clean and join the path
	fullPath := filepath.Join(baseDir, filepath.Clean(path))

	// Try to resolve the full path (works if file exists)
	realPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		// File doesn't exist - resolve the parent directory instead (for writes)
		parentDir := filepath.Dir(fullPath)
		realParent, parentErr := filepath.EvalSymlinks(parentDir)
		if parentErr != nil {
			// Parent doesn't exist either - walk up until we find an existing path
			realParent, parentErr = resolveExistingParent(parentDir)
			if parentErr != nil {
				return "", fmt.Errorf("failed to resolve path: %w", parentErr)
			}
		}
		realParent, _ = filepath.Abs(realParent)

		// Verify the parent is within the base
		if !isPathWithin(realParent, realBase) {
			return "", fmt.Errorf("path escapes working directory via symlink: %s", path)
		}

		return fullPath, nil
	}

	// File exists - verify the resolved path is within the base
	realPath, _ = filepath.Abs(realPath)
	if !isPathWithin(realPath, realBase) {
		return "", fmt.Errorf("path escapes working directory via symlink: %s", path)
	}

	return fullPath, nil
}

// resolveExistingParent walks up the directory tree until it finds an existing directory
func resolveExistingParent(path string) (string, error) {
	for {
		parent := filepath.Dir(path)
		if parent == path {
			// Reached root
			return filepath.EvalSymlinks(parent)
		}
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			return resolved, nil
		}
		path = parent
	}
}

// isPathWithin checks if path is within or equal to base (after both are resolved)
func isPathWithin(path, base string) bool {
	// Add trailing separator to base to prevent prefix matching issues
	// e.g., /home/user vs /home/username
	if !strings.HasSuffix(base, string(filepath.Separator)) {
		base += string(filepath.Separator)
	}
	return path == strings.TrimSuffix(base, string(filepath.Separator)) ||
		strings.HasPrefix(path, base)
}

// ToolDispatcher manages and executes agent tools
type ToolDispatcher struct {
	workDir    string
	tools      map[string]Tool
	inspectors []Inspector
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

// SetInspectors configures the inspectors to run before tool execution
func (td *ToolDispatcher) SetInspectors(inspectors []Inspector) {
	td.inspectors = inspectors
}

// Execute runs a tool by name after validating with all inspectors
func (td *ToolDispatcher) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	tool, ok := td.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}

	// Run all inspectors before executing the tool
	for _, inspector := range td.inspectors {
		if err := inspector.Inspect(ctx, name, args); err != nil {
			return "", fmt.Errorf("inspector %s rejected tool call: %w", inspector.Name(), err)
		}
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

	// Security: validate path stays within workDir
	fullPath, err := securePath(t.workDir, path)
	if err != nil {
		return "", err
	}

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

	// Security: validate path stays within workDir
	fullPath, err := securePath(t.workDir, path)
	if err != nil {
		return "", err
	}

	// Security: check for symlinks to prevent disclosure
	info, err := os.Lstat(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("symlinks are not allowed: %s", path)
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}

// ShellTool executes shell commands with strict allowlisting and argument validation
type ShellTool struct {
	workDir string
}

func (t *ShellTool) Name() string        { return "shell" }
func (t *ShellTool) Description() string { return "Execute a shell command (docker/docker-compose only)" }

func (t *ShellTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return "", fmt.Errorf("command is required")
	}

	// Security: validate command against strict allowlist with argument checking
	if err := t.validateShellCommand(command); err != nil {
		return "", err
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

// validateShellCommand performs strict validation of shell commands.
// Only docker and docker-compose are allowed, with dangerous flags blocked.
func (t *ShellTool) validateShellCommand(command string) error {
	command = strings.TrimSpace(command)

	// Block shell metacharacters and injection vectors
	blockedChars := "\n\r><|$`;&"
	for _, c := range blockedChars {
		if strings.ContainsRune(command, c) {
			return fmt.Errorf("shell metacharacter not allowed: %q", c)
		}
	}

	// Block command chaining
	if strings.Contains(command, "&&") || strings.Contains(command, "||") {
		return fmt.Errorf("command chaining not allowed")
	}

	// Parse command into parts
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	baseCmd := filepath.Base(parts[0])

	// Only allow docker and docker-compose
	switch baseCmd {
	case "docker":
		return t.validateDockerCommand(parts[1:])
	case "docker-compose":
		return t.validateDockerComposeCommand(parts[1:])
	default:
		return fmt.Errorf("only docker and docker-compose commands are allowed, got: %s", baseCmd)
	}
}

// validateDockerCommand checks docker command arguments for dangerous operations
func (t *ShellTool) validateDockerCommand(args []string) error {
	// Dangerous docker flags that could compromise the host
	dangerousFlags := []string{
		"--privileged",
		"--pid=host",
		"--network=host",
		"--userns=host",
		"--uts=host",
		"--ipc=host",
		"--cap-add",
		"--security-opt",
		"--device",
	}

	for _, arg := range args {
		// Check for dangerous flags
		for _, dangerous := range dangerousFlags {
			if strings.HasPrefix(arg, dangerous) {
				return fmt.Errorf("dangerous docker flag not allowed: %s", dangerous)
			}
		}

		// Check for dangerous volume mounts (mounting host root or sensitive paths)
		if strings.HasPrefix(arg, "-v") || strings.HasPrefix(arg, "--volume") {
			if err := t.validateVolumeMount(arg, args); err != nil {
				return err
			}
		}

		// Block --mount with dangerous options
		if strings.HasPrefix(arg, "--mount") {
			if strings.Contains(arg, "type=bind") && strings.Contains(arg, "source=/") {
				// Check if it's mounting something outside workDir
				if !strings.Contains(arg, "source="+t.workDir) {
					return fmt.Errorf("bind mounts outside working directory not allowed")
				}
			}
		}
	}

	return nil
}

// validateVolumeMount checks if a -v/--volume mount is safe
func (t *ShellTool) validateVolumeMount(arg string, args []string) error {
	var volumeSpec string

	// Handle -v=spec or -v spec formats
	if strings.Contains(arg, "=") {
		volumeSpec = strings.SplitN(arg, "=", 2)[1]
	} else if arg == "-v" || arg == "--volume" {
		// Volume spec is in the next argument - find it
		for i, a := range args {
			if a == arg && i+1 < len(args) {
				volumeSpec = args[i+1]
				break
			}
		}
	} else if strings.HasPrefix(arg, "-v") {
		volumeSpec = strings.TrimPrefix(arg, "-v")
	}

	if volumeSpec == "" {
		return nil // No spec found, let docker handle the error
	}

	// Parse volume spec: [host-src:]container-dest[:options]
	parts := strings.Split(volumeSpec, ":")
	if len(parts) >= 1 {
		hostPath := parts[0]

		// Block absolute paths outside workDir
		if filepath.IsAbs(hostPath) {
			// Resolve the workDir to compare
			absWorkDir, _ := filepath.Abs(t.workDir)
			absHostPath, _ := filepath.Abs(hostPath)

			if !strings.HasPrefix(absHostPath, absWorkDir) {
				return fmt.Errorf("volume mount outside working directory not allowed: %s", hostPath)
			}
		}

		// Block path traversal in relative paths
		if strings.Contains(hostPath, "..") {
			return fmt.Errorf("path traversal in volume mount not allowed")
		}

		// Block mounting root or sensitive directories
		sensitiveRoots := []string{"/", "/etc", "/var", "/usr", "/root", "/home"}
		for _, sensitive := range sensitiveRoots {
			if hostPath == sensitive || strings.HasPrefix(hostPath, sensitive+"/") {
				return fmt.Errorf("mounting sensitive host path not allowed: %s", hostPath)
			}
		}
	}

	return nil
}

// validateDockerComposeCommand checks docker-compose arguments
func (t *ShellTool) validateDockerComposeCommand(args []string) error {
	// docker-compose is generally safer since it reads from docker-compose.yml
	// But block commands that could be dangerous

	for _, arg := range args {
		// Block exec with arbitrary commands (could run anything)
		if arg == "exec" {
			// Find what comes after exec to see if it's safe
			for i, a := range args {
				if a == "exec" && i+1 < len(args) {
					// Allow common safe exec commands
					nextArg := args[i+1]
					if nextArg != "-T" && nextArg != "--no-TTY" &&
						!strings.HasPrefix(nextArg, "-") {
						// This is the service name, next would be the command
						// For now, block exec entirely for safety
						return fmt.Errorf("docker-compose exec not allowed (security restriction)")
					}
				}
			}
		}

		// Block run with arbitrary commands
		if arg == "run" {
			return fmt.Errorf("docker-compose run not allowed (security restriction)")
		}
	}

	return nil
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
	ruby.RegisterAll(registry)
	php.RegisterAll(registry)
	java.RegisterAll(registry)
	dotnet.RegisterAll(registry)
	elixir.RegisterAll(registry)

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
	ruby.RegisterAll(registry)
	php.RegisterAll(registry)
	java.RegisterAll(registry)
	dotnet.RegisterAll(registry)
	elixir.RegisterAll(registry)

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
