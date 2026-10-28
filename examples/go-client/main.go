// This example reads MCP sync status only. Build/run in an authorized environment.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 4 {
		return errors.New("usage: go-client /absolute/capsule-memory /private/device-home SCOPE_ID")
	}
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "capsule-memory-example", Version: "0.1.0"}, nil)
	command := exec.CommandContext(ctx, os.Args[1], "--home", os.Args[2], "mcp", "--scope", os.Args[3])
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		return errors.New("MCP connection failed")
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "sync_status", Arguments: map[string]any{}})
	if err != nil {
		return errors.New("MCP status call failed")
	}
	if result.IsError {
		return errors.New("MCP status tool returned an error")
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
