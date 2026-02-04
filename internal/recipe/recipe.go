// Package recipe provides YAML-based workflow definitions.
package recipe

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Recipe defines a reusable workflow
type Recipe struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Version     string            `yaml:"version"`
	Variables   map[string]string `yaml:"variables"`
	Steps       []Step            `yaml:"steps"`
}

// Step defines a single step in a recipe
type Step struct {
	Name      string            `yaml:"name"`
	Tool      string            `yaml:"tool"`
	Args      map[string]string `yaml:"args"`
	Condition string            `yaml:"condition,omitempty"`
	OnError   string            `yaml:"on_error,omitempty"` // "continue", "fail", "retry"
	Retries   int               `yaml:"retries,omitempty"`
}

// StepResult contains the result of executing a step
type StepResult struct {
	Name    string
	Success bool
	Output  string
	Error   error
}

// ExecutionResult contains the overall recipe execution result
type ExecutionResult struct {
	Recipe  string
	Success bool
	Steps   []StepResult
}

// Load loads a recipe from a YAML file
func Load(path string) (*Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipe: %w", err)
	}

	var recipe Recipe
	if err := yaml.Unmarshal(data, &recipe); err != nil {
		return nil, fmt.Errorf("failed to parse recipe: %w", err)
	}

	return &recipe, nil
}

// LoadFromString loads a recipe from a YAML string
func LoadFromString(content string) (*Recipe, error) {
	var recipe Recipe
	if err := yaml.Unmarshal([]byte(content), &recipe); err != nil {
		return nil, fmt.Errorf("failed to parse recipe: %w", err)
	}
	return &recipe, nil
}

// Executor executes recipes
type Executor struct {
	toolExecutor ToolExecutor
	variables    map[string]string
}

// ToolExecutor is the interface for executing tools
type ToolExecutor interface {
	Execute(ctx context.Context, tool string, args map[string]interface{}) (string, error)
}

// NewExecutor creates a new recipe executor
func NewExecutor(toolExec ToolExecutor) *Executor {
	return &Executor{
		toolExecutor: toolExec,
		variables:    make(map[string]string),
	}
}

// SetVariable sets a variable for interpolation
func (e *Executor) SetVariable(name, value string) {
	e.variables[name] = value
}

// Execute runs a recipe
func (e *Executor) Execute(ctx context.Context, recipe *Recipe) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Recipe: recipe.Name,
		Steps:  make([]StepResult, 0, len(recipe.Steps)),
	}

	// Merge recipe variables with executor variables
	vars := make(map[string]string)
	for k, v := range recipe.Variables {
		vars[k] = v
	}
	for k, v := range e.variables {
		vars[k] = v
	}

	// Execute each step
	for _, step := range recipe.Steps {
		// Check condition
		if step.Condition != "" {
			if !e.evaluateCondition(step.Condition, vars) {
				continue
			}
		}

		// Interpolate args
		args := e.interpolateArgs(step.Args, vars)

		// Execute with retries
		var stepResult StepResult
		stepResult.Name = step.Name

		retries := step.Retries
		if retries == 0 {
			retries = 1
		}

		for attempt := 0; attempt < retries; attempt++ {
			output, err := e.toolExecutor.Execute(ctx, step.Tool, args)
			stepResult.Output = output

			if err == nil {
				stepResult.Success = true
				break
			}

			stepResult.Error = err

			if attempt < retries-1 {
				continue // Retry
			}
		}

		result.Steps = append(result.Steps, stepResult)

		// Handle errors
		if !stepResult.Success {
			switch step.OnError {
			case "continue":
				continue
			case "fail", "":
				result.Success = false
				return result, stepResult.Error
			}
		}

		// Store output as variable for next steps
		vars["last_output"] = stepResult.Output
	}

	result.Success = true
	return result, nil
}

// interpolateArgs replaces ${var} patterns with variable values
func (e *Executor) interpolateArgs(args map[string]string, vars map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	re := regexp.MustCompile(`\$\{(\w+)\}`)

	for k, v := range args {
		interpolated := re.ReplaceAllStringFunc(v, func(match string) string {
			varName := match[2 : len(match)-1]
			if val, ok := vars[varName]; ok {
				return val
			}
			return match
		})
		result[k] = interpolated
	}

	return result
}

// evaluateCondition evaluates a simple condition
func (e *Executor) evaluateCondition(condition string, vars map[string]string) bool {
	// Simple conditions: "var == value" or "var != value"
	condition = strings.TrimSpace(condition)

	if strings.Contains(condition, "==") {
		parts := strings.SplitN(condition, "==", 2)
		if len(parts) == 2 {
			varName := strings.TrimSpace(parts[0])
			expected := strings.TrimSpace(strings.Trim(parts[1], "\"'"))
			actual, ok := vars[varName]
			return ok && actual == expected
		}
	}

	if strings.Contains(condition, "!=") {
		parts := strings.SplitN(condition, "!=", 2)
		if len(parts) == 2 {
			varName := strings.TrimSpace(parts[0])
			expected := strings.TrimSpace(strings.Trim(parts[1], "\"'"))
			actual, ok := vars[varName]
			return !ok || actual != expected
		}
	}

	// Check if variable exists
	_, exists := vars[condition]
	return exists
}

// BuiltinRecipes contains built-in recipe definitions
var BuiltinRecipes = map[string]string{
	"analyze": `
name: analyze
description: Analyze a repository and report detected stack
version: "1.0"
steps:
  - name: Scan Repository
    tool: dockerizer_analyze
    args:
      path: "${path}"
`,

	"generate": `
name: generate
description: Generate Docker configuration for a repository
version: "1.0"
steps:
  - name: Analyze
    tool: dockerizer_analyze
    args:
      path: "${path}"
  - name: Generate
    tool: dockerizer_generate
    args:
      path: "${path}"
      overwrite: "${overwrite}"
`,

	"build-and-test": `
name: build-and-test
description: Generate, build, and test Docker configuration
version: "1.0"
steps:
  - name: Generate Configuration
    tool: dockerizer_generate
    args:
      path: "${path}"
  - name: Build Image
    tool: docker_build
    args:
      path: "${path}"
      tag: "${image_tag}"
    on_error: fail
  - name: Run Container
    tool: docker_run
    args:
      image: "${image_tag}"
      detach: "true"
  - name: Check Logs
    tool: docker_logs
    args:
      container: "${container_name}"
      tail: "50"
`,

	"full-deploy": `
name: full-deploy
description: Complete deployment workflow with validation
version: "1.0"
variables:
  image_tag: "app:latest"
steps:
  - name: Analyze Repository
    tool: dockerizer_analyze
    args:
      path: "${path}"
  - name: Generate Docker Files
    tool: dockerizer_generate
    args:
      path: "${path}"
      overwrite: "true"
  - name: Build Docker Image
    tool: docker_build
    args:
      path: "${path}"
      tag: "${image_tag}"
    retries: 2
  - name: Test Container
    tool: docker_run
    args:
      image: "${image_tag}"
    on_error: fail
`,
}

// GetBuiltinRecipe returns a built-in recipe by name
func GetBuiltinRecipe(name string) (*Recipe, error) {
	content, ok := BuiltinRecipes[name]
	if !ok {
		return nil, fmt.Errorf("unknown builtin recipe: %s", name)
	}
	return LoadFromString(content)
}

// ListBuiltinRecipes returns the names of all built-in recipes
func ListBuiltinRecipes() []string {
	names := make([]string, 0, len(BuiltinRecipes))
	for name := range BuiltinRecipes {
		names = append(names, name)
	}
	return names
}
