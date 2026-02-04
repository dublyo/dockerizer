package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/dublyo/dockerizer/internal/mcp"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run as MCP server for Claude Code/Goose integration",
	Long: `Run dockerizer as a Model Context Protocol (MCP) server.

This allows dockerizer to be used as a tool provider for AI coding assistants
like Claude Code and Goose. The server communicates via stdin/stdout using
the MCP protocol.

Configuration in Claude Code (~/.claude.json):
{
  "mcpServers": {
    "dockerizer": {
      "command": "dockerizer",
      "args": ["serve"]
    }
  }
}

Configuration in Goose (profiles.yaml):
extensions:
  dockerizer:
    name: dockerizer
    cmd: dockerizer
    args: ["serve"]
    type: stdio`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel()
	}()

	// Create registry and server
	registry := setupRegistry()
	server := mcp.NewServer(registry)

	// Run server
	return server.Run(ctx)
}
