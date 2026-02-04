// Package mcp provides Model Context Protocol server functionality.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/dublyo/dockerizer/internal/detector"
	"github.com/dublyo/dockerizer/internal/generator"
	"github.com/dublyo/dockerizer/internal/scanner"
)

// Server implements the MCP protocol for dockerizer
type Server struct {
	registry  *detector.Registry
	generator generator.Generator
	scanner   scanner.Scanner
	mu        sync.RWMutex
}

// NewServer creates a new MCP server
func NewServer(registry *detector.Registry) *Server {
	return &Server{
		registry:  registry,
		generator: generator.New(),
		scanner:   scanner.New(),
	}
}

// Message represents an MCP JSON-RPC message
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Tool represents an MCP tool definition
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// Run starts the MCP server on stdin/stdout
func (s *Server) Run(ctx context.Context) error {
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Read line (JSON-RPC message)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}

		// Parse message
		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			s.sendError(encoder, nil, -32700, "Parse error", nil)
			continue
		}

		// Handle message
		response := s.handleMessage(ctx, &msg)
		if response != nil {
			encoder.Encode(response)
		}
	}
}

// handleMessage processes an incoming MCP message
func (s *Server) handleMessage(ctx context.Context, msg *Message) *Message {
	switch msg.Method {
	case "initialize":
		return s.handleInitialize(msg)
	case "tools/list":
		return s.handleToolsList(msg)
	case "tools/call":
		return s.handleToolsCall(ctx, msg)
	case "shutdown":
		return &Message{JSONRPC: "2.0", ID: msg.ID, Result: nil}
	default:
		return s.errorResponse(msg.ID, -32601, "Method not found", nil)
	}
}

// handleInitialize handles the initialize request
func (s *Server) handleInitialize(msg *Message) *Message {
	return &Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]bool{
					"listChanged": false,
				},
			},
			"serverInfo": map[string]string{
				"name":    "dockerizer",
				"version": "1.0.0",
			},
		},
	}
}

// handleToolsList returns the list of available tools
func (s *Server) handleToolsList(msg *Message) *Message {
	tools := []Tool{
		{
			Name:        "dockerizer_analyze",
			Description: "Analyze a repository to detect its technology stack (language, framework, version)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the repository to analyze",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "dockerizer_generate",
			Description: "Generate Dockerfile, docker-compose.yml, .dockerignore, and .env.example for a repository",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the repository",
					},
					"output_path": map[string]interface{}{
						"type":        "string",
						"description": "Path to write output files (defaults to repository path)",
					},
					"overwrite": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to overwrite existing files",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "docker_build",
			Description: "Build a Docker image from a Dockerfile",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the build context",
					},
					"dockerfile": map[string]interface{}{
						"type":        "string",
						"description": "Path to Dockerfile (relative to context)",
					},
					"tag": map[string]interface{}{
						"type":        "string",
						"description": "Tag for the built image",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "docker_run",
			Description: "Run a Docker container for testing",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"image": map[string]interface{}{
						"type":        "string",
						"description": "Image name to run",
					},
					"port": map[string]interface{}{
						"type":        "string",
						"description": "Port mapping (e.g., '8080:8080')",
					},
					"detach": map[string]interface{}{
						"type":        "boolean",
						"description": "Run in detached mode",
					},
				},
				"required": []string{"image"},
			},
		},
		{
			Name:        "docker_logs",
			Description: "Get logs from a Docker container",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"container": map[string]interface{}{
						"type":        "string",
						"description": "Container name or ID",
					},
					"tail": map[string]interface{}{
						"type":        "string",
						"description": "Number of lines to show from the end",
					},
				},
				"required": []string{"container"},
			},
		},
	}

	return &Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result: map[string]interface{}{
			"tools": tools,
		},
	}
}

// handleToolsCall executes a tool
func (s *Server) handleToolsCall(ctx context.Context, msg *Message) *Message {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.errorResponse(msg.ID, -32602, "Invalid params", nil)
	}

	var result interface{}
	var err error

	switch params.Name {
	case "dockerizer_analyze":
		result, err = s.toolAnalyze(ctx, params.Arguments)
	case "dockerizer_generate":
		result, err = s.toolGenerate(ctx, params.Arguments)
	case "docker_build":
		result, err = s.toolDockerBuild(ctx, params.Arguments)
	case "docker_run":
		result, err = s.toolDockerRun(ctx, params.Arguments)
	case "docker_logs":
		result, err = s.toolDockerLogs(ctx, params.Arguments)
	default:
		return s.errorResponse(msg.ID, -32602, "Unknown tool: "+params.Name, nil)
	}

	if err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": fmt.Sprintf("Error: %v", err),
					},
				},
				"isError": true,
			},
		}
	}

	return &Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("%v", result),
				},
			},
		},
	}
}

// Tool implementations

func (s *Server) toolAnalyze(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	scan, err := s.scanner.Scan(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	det := detector.New(s.registry)
	result, err := det.Detect(ctx, scan)
	if err != nil {
		return nil, fmt.Errorf("detection failed: %w", err)
	}

	return map[string]interface{}{
		"detected":   result.Detected,
		"language":   result.Language,
		"framework":  result.Framework,
		"version":    result.Version,
		"confidence": result.Confidence,
		"provider":   result.Provider,
		"variables":  result.Variables,
	}, nil
}

func (s *Server) toolGenerate(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	outputPath, _ := args["output_path"].(string)
	if outputPath == "" {
		outputPath = path
	}

	overwrite, _ := args["overwrite"].(bool)

	// Scan and detect
	scan, err := s.scanner.Scan(ctx, path)
	if err != nil {
		return nil, err
	}

	det := detector.New(s.registry)
	result, err := det.Detect(ctx, scan)
	if err != nil {
		return nil, err
	}

	if !result.Detected {
		return nil, fmt.Errorf("could not detect project type")
	}

	// Generate
	gen := generator.New(generator.WithOverwrite(overwrite))
	output, err := gen.Generate(result, outputPath)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0)
	for f := range output.Files {
		files = append(files, f)
	}

	return map[string]interface{}{
		"success":   true,
		"files":     files,
		"language":  result.Language,
		"framework": result.Framework,
	}, nil
}

func (s *Server) toolDockerBuild(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}

	dockerfile, _ := args["dockerfile"].(string)
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}

	tag, _ := args["tag"].(string)
	if tag == "" {
		tag = "dockerize-build:latest"
	}

	// Return description of what would happen (actual implementation uses agent tools)
	return fmt.Sprintf("Would build image %s from %s in %s", tag, dockerfile, path), nil
}

func (s *Server) toolDockerRun(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	image, _ := args["image"].(string)
	if image == "" {
		return nil, fmt.Errorf("image is required")
	}

	return fmt.Sprintf("Would run container from image %s", image), nil
}

func (s *Server) toolDockerLogs(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	container, _ := args["container"].(string)
	if container == "" {
		return nil, fmt.Errorf("container is required")
	}

	return fmt.Sprintf("Would get logs from container %s", container), nil
}

// Helper functions

func (s *Server) errorResponse(id interface{}, code int, message string, data interface{}) *Message {
	return &Message{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

func (s *Server) sendError(encoder *json.Encoder, id interface{}, code int, message string, data interface{}) {
	encoder.Encode(s.errorResponse(id, code, message, data))
}
