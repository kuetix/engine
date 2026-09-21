package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// newLogger builds the server's slog logger. It writes to stderr by default
// (stdout is reserved for the MCP protocol on the stdio transport) or to
// logFile when set. The returned closer flushes/closes any opened file.
func newLogger(level, logFile string) (*slog.Logger, func() error, error) {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "info":
		lvl = slog.LevelInfo
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return nil, nil, fmt.Errorf("invalid -log-level %q (want debug|info|warn|error)", level)
	}

	var w io.Writer = os.Stderr
	closer := func() error { return nil }
	if strings.TrimSpace(logFile) != "" {
		if dir := filepath.Dir(logFile); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, nil, fmt.Errorf("log-file: %w", err)
			}
		}
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, nil, fmt.Errorf("log-file: %w", err)
		}
		w = f
		closer = f.Close
	}

	logger := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl}))
	return logger, closer, nil
}

// toolLoggingMiddleware logs one line per tool call: the tool name at debug
// with the argument keys, then a completion line with duration and outcome.
// Errors (both transport errors and mcp "isError" results) log at warn.
func toolLoggingMiddleware(logger *slog.Logger) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name := req.Params.Name
			logger.Debug("tool call", "tool", name, "args", argKeys(req.GetArguments()))

			start := time.Now()
			res, err := next(ctx, req)
			dur := time.Since(start)

			switch {
			case err != nil:
				logger.Warn("tool failed", "tool", name, "ms", dur.Milliseconds(), "err", err.Error())
			case res != nil && res.IsError:
				logger.Warn("tool returned error", "tool", name, "ms", dur.Milliseconds(), "detail", firstText(res))
			default:
				logger.Info("tool ok", "tool", name, "ms", dur.Milliseconds())
			}
			return res, err
		}
	}
}

func argKeys(args map[string]interface{}) []string {
	if len(args) == 0 {
		return nil
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	return keys
}

func firstText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			t := tc.Text
			if len(t) > 200 {
				t = t[:200] + "…"
			}
			return t
		}
	}
	return ""
}
