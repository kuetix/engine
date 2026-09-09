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
	"log/slog"
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

const serverName = "kuetix-engine"

// Version and BuildTime are injected at build time via -ldflags
// (see the Makefile / .goreleaser.yaml). They fall back to a dev default so
// `go run` / `go build` without ldflags still work.
var (
	Version   = "0.1.0-dev"
	BuildTime = "unknown"

	// serverVersion is the version string advertised to MCP clients.
	serverVersion = Version
)

// runnerBin and runScratchDir configure wsl_run (see run.go): runnerBin is
// the path to the `runner` binary (github.com/kuetix/runner) used as the
// execution sandbox, and runScratchDir is where inline/one-off WSL/SWSL
// source gets written to disk before the runner executes it. Package-level
// because tools.go's handlers need them and are registered before main()
// finishes parsing flags.
var (
	runnerBin     string
	runScratchDir string
)

func main() {
	var (
		httpAddr          = flag.String("http", "", "HTTP listen address (e.g. :8080). If empty, stdio is used.")
		transport         = flag.String("transport", "sse", "HTTP transport when -http is set: 'sse' or 'http' (streamable HTTP, used by GitHub Copilot).")
		baseURL           = flag.String("base-url", "", "Public base URL advertised to SSE clients (optional, SSE only).")
		ssePath           = flag.String("sse-path", "/sse", "SSE endpoint path (SSE transport only).")
		messagePath       = flag.String("message-path", "/message", "Message endpoint path (SSE transport only).")
		endpointPath      = flag.String("endpoint-path", "/mcp", "Endpoint path for streamable HTTP transport.")
		stateless         = flag.Bool("stateless", false, "Run streamable HTTP transport in stateless mode.")
		pidFile           = flag.String("pid-file", "", "Write process PID to this file and remove on exit.")
		runnerBinFlag     = flag.String("runner-bin", os.Getenv("KUETIX_RUNNER_BIN"), "Path to the kuetix `runner` binary used by wsl_run to execute workflows. Defaults to $KUETIX_RUNNER_BIN.")
		runScratchDirFlag = flag.String("run-scratch-dir", "runtime/mcp-runs", "Directory wsl_run writes inline/one-off WSL source to before executing it.")
		showVersion       = flag.Bool("version", false, "Print version / engine build info and exit.")
		logLevel          = flag.String("log-level", envOr("KUETIX_MCP_LOG_LEVEL", "info"), "Log verbosity: debug|info|warn|error. Also $KUETIX_MCP_LOG_LEVEL.")
		logFile           = flag.String("log-file", os.Getenv("KUETIX_MCP_LOG_FILE"), "Write logs to this file instead of stderr. Also $KUETIX_MCP_LOG_FILE.")
	)
	flag.Parse()
	runnerBin = *runnerBinFlag
	runScratchDir = *runScratchDirFlag

	if *showVersion {
		fmt.Print(collectBuildInfo().String())
		return
	}

	logger, closeLog, err := newLogger(*logLevel, *logFile)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "mcp-server: %v\n", err)
		os.Exit(2)
	}
	defer func() { _ = closeLog() }()
	slog.SetDefault(logger)
	bi := collectBuildInfo()
	logger.Info("mcp-server starting",
		"version", bi.Version, "engine_version", bi.EngineVersion,
		"build_time", bi.BuildTime, "go", bi.GoVersion,
		"vcs_revision", bi.VCSRevision, "vcs_modified", bi.VCSModified,
		"pid", os.Getpid())

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
		server.WithRecovery(),
		server.WithToolHandlerMiddleware(toolLoggingMiddleware(logger)),
	)

	registerTools(s)
	logger.Info("tools registered", "count", len(registeredToolNames), "tools", registeredToolNames)

	exit := func(code int) {
		if *pidFile != "" {
			_ = os.Remove(*pidFile)
		}
		os.Exit(code)
	}

	if *httpAddr == "" {
		activeTransport = "stdio"
		logger.Info("serving", "transport", "stdio")
		if err := server.ServeStdio(s); err != nil {
			logger.Error("stdio transport stopped", "err", err)
			exit(1)
		}
		logger.Info("mcp-server stopped", "transport", "stdio")
		return
	}

	var (
		srv    httpTransport
		banner string
	)
	switch *transport {
	case "sse":
		activeTransport = "sse"
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
		activeTransport = "http"
		opts := []server.StreamableHTTPOption{
			server.WithEndpointPath(*endpointPath),
			server.WithStateLess(*stateless),
		}
		srv = server.NewStreamableHTTPServer(s, opts...)
		banner = fmt.Sprintf("streamable HTTP %s", *endpointPath)
	default:
		logger.Error("unknown transport", "transport", *transport, "want", "sse|http")
		exit(2)
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("serving", "transport", activeTransport, "addr", *httpAddr, "endpoint", banner)
		errCh <- srv.Start(*httpAddr)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			logger.Error("transport stopped", "err", err)
			exit(1)
		}
	case sig := <-sigCh:
		logger.Info("signal received, shutting down", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("shutdown error", "err", err)
			exit(1)
		}
	}
	logger.Info("mcp-server stopped", "transport", activeTransport)
	if *pidFile != "" {
		_ = os.Remove(*pidFile)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
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
