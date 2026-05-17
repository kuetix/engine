// Command mcp-server is a Model Context Protocol (MCP) server that exposes
// the kuetix/engine WSL/SWSL parser and workflow tooling. It supports stdio
// (default), HTTP/SSE, and streamable HTTP transports. Streamable HTTP is the
// transport expected by clients like GitHub Copilot (which uses "type": "http"
// in its mcp.json config).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

type httpTransport interface {
	Start(addr string) error
	Shutdown(ctx context.Context) error
}

const (
	serverName    = "kuetix-engine"
	serverVersion = "0.1.0"
)

func main() {
	var (
		httpAddr     = flag.String("http", "", "HTTP listen address (e.g. :8080). If empty, stdio is used.")
		transport    = flag.String("transport", "sse", "HTTP transport when -http is set: 'sse' or 'http' (streamable HTTP, used by GitHub Copilot).")
		baseURL      = flag.String("base-url", "", "Public base URL advertised to SSE clients (optional, SSE only).")
		ssePath      = flag.String("sse-path", "/sse", "SSE endpoint path (SSE transport only).")
		messagePath  = flag.String("message-path", "/message", "Message endpoint path (SSE transport only).")
		endpointPath = flag.String("endpoint-path", "/mcp", "Endpoint path for streamable HTTP transport.")
		stateless    = flag.Bool("stateless", false, "Run streamable HTTP transport in stateless mode.")
		pidFile      = flag.String("pid-file", "", "Write process PID to this file and remove on exit.")
	)
	flag.Parse()

	if *pidFile != "" {
		if err := writePIDFile(*pidFile); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "mcp-server: %v\n", err)
			os.Exit(1)
		}
	}

	s := server.NewMCPServer(
		serverName,
		serverVersion,
		server.WithToolCapabilities(true),
	)

	registerTools(s)

	exit := func(code int) {
		if *pidFile != "" {
			_ = os.Remove(*pidFile)
		}
		os.Exit(code)
	}

	if *httpAddr == "" {
		if err := server.ServeStdio(s); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "mcp-server: %v\n", err)
			exit(1)
		}
		return
	}

	var (
		srv    httpTransport
		banner string
	)
	switch *transport {
	case "sse":
		opts := []server.SSEOption{
			server.WithSSEEndpoint(*ssePath),
			server.WithMessageEndpoint(*messagePath),
			server.WithKeepAlive(true),
		}
		if *baseURL != "" {
			opts = append(opts, server.WithBaseURL(*baseURL))
		}
		srv = server.NewSSEServer(s, opts...)
		banner = fmt.Sprintf("SSE %s, message %s", *ssePath, *messagePath)
	case "http", "streamable-http":
		opts := []server.StreamableHTTPOption{
			server.WithEndpointPath(*endpointPath),
			server.WithStateLess(*stateless),
		}
		srv = server.NewStreamableHTTPServer(s, opts...)
		banner = fmt.Sprintf("streamable HTTP %s", *endpointPath)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "mcp-server: unknown -transport %q (want 'sse' or 'http')\n", *transport)
		exit(2)
	}

	errCh := make(chan error, 1)
	go func() {
		_, _ = fmt.Fprintf(os.Stderr, "mcp-server: listening on %s (%s)\n", *httpAddr, banner)
		errCh <- srv.Start(*httpAddr)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "mcp-server: %v\n", err)
			exit(1)
		}
	case <-sigCh:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "mcp-server: shutdown: %v\n", err)
			exit(1)
		}
	}
	if *pidFile != "" {
		_ = os.Remove(*pidFile)
	}
}

func writePIDFile(path string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("pid-file: %w", err)
		}
	}
	data := strconv.Itoa(os.Getpid()) + "\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		return fmt.Errorf("pid-file: %w", err)
	}
	return nil
}
