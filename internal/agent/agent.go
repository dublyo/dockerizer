// Package agent provides the Agent Mode for iterative Docker configuration.
package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/dublyo/dockerizer/internal/ai"
	"github.com/dublyo/dockerizer/internal/scanner"
)

// Agent orchestrates the iterative build/test/fix cycle
type Agent struct {
	provider    ai.Provider
	tools       *ToolDispatcher
	session     *Session
	inspectors  []Inspector
	maxAttempts int
	events      chan AgentEvent
}

// AgentConfig configures the agent
type AgentConfig struct {
	AIProvider  ai.Provider
	MaxAttempts int
	WorkDir     string
	Verbose     bool
}

// AgentEvent represents an event during agent execution
type AgentEvent struct {
	Type      EventType
	Timestamp time.Time
	Message   string
	Data      interface{}
}

// EventType represents the type of agent event
type EventType string

const (
	EventStart      EventType = "start"
	EventAnalyzing  EventType = "analyzing"
	EventGenerating EventType = "generating"
	EventBuilding   EventType = "building"
	EventTesting    EventType = "testing"
	EventFixing     EventType = "fixing"
	EventSuccess    EventType = "success"
	EventError      EventType = "error"
	EventComplete   EventType = "complete"
)

// Inspector validates tool calls before execution
type Inspector interface {
	Name() string
	Inspect(ctx context.Context, tool string, args map[string]interface{}) error
}

// New creates a new Agent
func New(cfg AgentConfig) *Agent {
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 5
	}

	return &Agent{
		provider:    cfg.AIProvider,
		tools:       NewToolDispatcher(cfg.WorkDir),
		session:     NewSession(),
		maxAttempts: cfg.MaxAttempts,
		events:      make(chan AgentEvent, 100),
		inspectors: []Inspector{
			&SecurityInspector{},
			&SyntaxInspector{},
		},
	}
}

// Events returns the event channel for monitoring
func (a *Agent) Events() <-chan AgentEvent {
	return a.events
}

// Run executes the agent loop
func (a *Agent) Run(ctx context.Context, scan *scanner.ScanResult, instructions string) (*Result, error) {
	a.emit(EventStart, "Starting agent", nil)

	result := &Result{
		StartTime: time.Now(),
		Attempts:  make([]Attempt, 0),
	}

	for attempt := 1; attempt <= a.maxAttempts; attempt++ {
		a.emit(EventAnalyzing, fmt.Sprintf("Attempt %d/%d: Analyzing project", attempt, a.maxAttempts), nil)

		attemptResult := a.runAttempt(ctx, scan, instructions, attempt)
		result.Attempts = append(result.Attempts, attemptResult)

		if attemptResult.Success {
			a.emit(EventSuccess, "Docker configuration generated successfully", nil)
			result.Success = true
			result.FinalOutput = attemptResult.Output
			break
		}

		if attempt < a.maxAttempts {
			a.emit(EventFixing, fmt.Sprintf("Build failed, analyzing error for fix (attempt %d)", attempt), attemptResult.Error)
			// Add the error to context for next attempt
			instructions = fmt.Sprintf("%s\n\nPrevious attempt failed with error:\n%s\n\nPlease fix this issue.", instructions, attemptResult.Error)
		}
	}

	result.EndTime = time.Now()
	a.emit(EventComplete, "Agent completed", result)

	return result, nil
}

// runAttempt executes a single attempt
func (a *Agent) runAttempt(ctx context.Context, scan *scanner.ScanResult, instructions string, attemptNum int) Attempt {
	attempt := Attempt{
		Number:    attemptNum,
		StartTime: time.Now(),
	}

	// Generate Docker configuration
	a.emit(EventGenerating, "Generating Docker configuration", nil)
	response, err := a.provider.Generate(ctx, scan, instructions)
	if err != nil {
		attempt.Error = err.Error()
		attempt.EndTime = time.Now()
		return attempt
	}

	attempt.Output = &Output{
		Dockerfile:    response.Dockerfile,
		DockerCompose: response.DockerCompose,
		Dockerignore:  response.Dockerignore,
		EnvExample:    response.EnvExample,
	}

	// Write files
	if err := a.tools.WriteDockerFiles(ctx, attempt.Output); err != nil {
		attempt.Error = fmt.Sprintf("failed to write files: %v", err)
		attempt.EndTime = time.Now()
		return attempt
	}

	// Build Docker image
	a.emit(EventBuilding, "Building Docker image", nil)
	buildResult, err := a.tools.Execute(ctx, "docker_build", map[string]interface{}{
		"dockerfile": "Dockerfile",
		"tag":        "dockerize-test:latest",
	})
	if err != nil {
		attempt.Error = fmt.Sprintf("build failed: %v", err)
		attempt.BuildLog = buildResult
		attempt.EndTime = time.Now()
		return attempt
	}
	attempt.BuildLog = buildResult

	// Test the container
	a.emit(EventTesting, "Testing container", nil)
	testResult, err := a.tools.Execute(ctx, "docker_run", map[string]interface{}{
		"image":   "dockerize-test:latest",
		"timeout": 30,
	})
	if err != nil {
		attempt.Error = fmt.Sprintf("test failed: %v", err)
		attempt.TestLog = testResult
		attempt.EndTime = time.Now()
		return attempt
	}
	attempt.TestLog = testResult

	// Success!
	attempt.Success = true
	attempt.EndTime = time.Now()
	return attempt
}

// emit sends an event to the event channel
func (a *Agent) emit(eventType EventType, message string, data interface{}) {
	select {
	case a.events <- AgentEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		Message:   message,
		Data:      data,
	}:
	default:
		// Channel full, skip event
	}
}

// Result contains the overall agent result
type Result struct {
	Success     bool
	StartTime   time.Time
	EndTime     time.Time
	Attempts    []Attempt
	FinalOutput *Output
}

// Attempt represents a single generation attempt
type Attempt struct {
	Number    int
	StartTime time.Time
	EndTime   time.Time
	Success   bool
	Error     string
	Output    *Output
	BuildLog  string
	TestLog   string
}

// Output contains the generated files
type Output struct {
	Dockerfile    string
	DockerCompose string
	Dockerignore  string
	EnvExample    string
}

// Session manages conversation history
type Session struct {
	ID       string
	Messages []Message
}

// Message represents a conversation message
type Message struct {
	Role    string
	Content string
	Time    time.Time
}

// NewSession creates a new session
func NewSession() *Session {
	return &Session{
		ID:       fmt.Sprintf("session-%d", time.Now().UnixNano()),
		Messages: make([]Message, 0),
	}
}

// AddMessage adds a message to the session
func (s *Session) AddMessage(role, content string) {
	s.Messages = append(s.Messages, Message{
		Role:    role,
		Content: content,
		Time:    time.Now(),
	})
}
