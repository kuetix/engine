package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/engine/internal/wsl"
)

// runnerResult mirrors the JSON object github.com/kuetix/runner's cmd/runner
// always prints to stdout (see runner/cmd/runner/main.go's `result` type).
type runnerResult struct {
	Success    bool        `json:"success"`
	StatusCode int         `json:"status_code,omitempty"`
	Response   interface{} `json:"response,omitempty"`
	Error      string      `json:"error,omitempty"`
	DurationMs int64       `json:"duration_ms"`
}

// runnerActions is the exact set of "<module>/<class>" action groups the
// runner sandbox has registered (std-core + std-http + std-ai — see
// runner/modules/modules.go), read off their generated di.go files. The
// engine PANICS the whole process (not a recoverable FlowStepResult error)
// when a workflow calls an action outside this set, and that panic
// originates inside an engine-internal goroutine we cannot recover from out
// here — so unresolvable actions must be caught before ever invoking the
// runner. Keep this in sync if std-core/std-http/std-ai's registered
// modules change.
var runnerActions = map[string]bool{
	"converse/speak":                true,
	"services/common/assert":        true,
	"services/common/context":       true,
	"services/common/datetimeAt":    true,
	"services/common/debug":         true,
	"services/common/entity":        true,
	"services/common/errors":        true,
	"services/common/log":           true,
	"services/common/orchestration": true,
	"services/common/pagination":    true,
	"services/common/response":      true,
	"services/common/workflow":      true,
	"api/http/http":                 true,
	"ai/agent":                      true,
	"ai/external":                   true,
	"ai/prompt":                     true,
}

func handleRun(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := loadWSLInput(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Never hand unparseable source to the runner.
	mod, graphs, perr := parseModule(in)
	if perr != nil {
		return jsonResult(map[string]interface{}{
			"ok":          false,
			"diagnostics": []string{perr.Error()},
		})
	}

	// Nor source that calls actions the runner sandbox doesn't have — the
	// engine crashes the whole process on an unresolved action rather than
	// returning a clean error, so this has to be caught here.
	if unknown := unresolvableActions(mod, graphs); len(unknown) > 0 {
		return jsonResult(map[string]interface{}{
			"ok": false,
			"error": fmt.Sprintf(
				"workflow calls action(s) not available in the runner sandbox (std-core + std-http + std-ai only): %s. Check with wsl_workflow_actions, or scaffold a real transition with transition_scaffold if this needs custom logic.",
				strings.Join(unknown, ", "),
			),
		})
	}

	if strings.TrimSpace(runnerBin) == "" {
		return mcp.NewToolResultError("wsl_run is not configured: pass -runner-bin or set KUETIX_RUNNER_BIN to the path of the kuetix runner binary (github.com/kuetix/runner)"), nil
	}

	args := req.GetArguments()
	runArgs := stringArgs(args["args"])
	recordHistory := true
	if v, ok := args["record_history"].(bool); ok {
		recordHistory = v
	}

	res, execErr := runWSL(in, runArgs)
	if execErr != nil {
		return jsonResult(map[string]interface{}{
			"ok":    false,
			"error": execErr.Error(),
		})
	}

	out := map[string]interface{}{
		"ok": true,
		"execution": map[string]interface{}{
			"success":    res.Success,
			"statusCode": res.StatusCode,
			"response":   res.Response,
			"error":      nullableString(res.Error),
			"durationMs": res.DurationMs,
		},
	}

	if recordHistory {
		out["history"] = recordRun(in, runArgs, res)
	} else {
		out["history"] = map[string]interface{}{"recorded": false, "reason": "record_history=false"}
	}

	return jsonResult(out)
}

// unresolvableActions returns the deduplicated "module.name" refs of every
// action the workflow calls that isn't in runnerActions.
func unresolvableActions(mod *wsl.Module, graphs map[string]*wsl.Graph) []string {
	seen := map[string]bool{}
	var unknown []string
	for _, a := range workflow.ExtractActionsFromGraphs(mod, graphs) {
		if runnerActions[a.Module] {
			continue
		}
		ref := a.Module + "." + a.Name
		if !seen[ref] {
			seen[ref] = true
			unknown = append(unknown, ref)
		}
	}
	return unknown
}

// runWSL writes in.Source to a fresh scratch directory and executes it via
// the configured runner binary, returning its parsed JSON result.
func runWSL(in wslInput, runArgs map[string]string) (runnerResult, error) {
	if err := os.MkdirAll(runScratchDir, 0o755); err != nil {
		return runnerResult{}, fmt.Errorf("create scratch dir %s: %w", runScratchDir, err)
	}
	runDir, err := os.MkdirTemp(runScratchDir, "run-")
	if err != nil {
		return runnerResult{}, fmt.Errorf("create run dir: %w", err)
	}
	defer os.RemoveAll(runDir)

	// The engine's module-path resolution insists on finding a "modules"
	// directory relative to the working directory even though this runner
	// registers no transitions of its own beyond the standard library.
	if err := os.MkdirAll(filepath.Join(runDir, "modules"), 0o755); err != nil {
		return runnerResult{}, fmt.Errorf("create modules dir: %w", err)
	}

	filename := in.Filename
	if filename == "" {
		if in.Simplified {
			filename = "inline.swsl"
		} else {
			filename = "inline.wsl"
		}
	}
	filename = filepath.Base(filename)
	if err := os.WriteFile(filepath.Join(runDir, filename), []byte(in.Source), 0o644); err != nil {
		return runnerResult{}, fmt.Errorf("write workflow source: %w", err)
	}

	cmdArgs := []string{filename}
	for k, v := range runArgs {
		cmdArgs = append(cmdArgs, k+"="+v)
	}

	cmd := exec.Command(runnerBin, cmdArgs...)
	cmd.Dir = runDir
	stdout, runErr := cmd.Output()
	if len(stdout) == 0 {
		if runErr != nil {
			return runnerResult{}, fmt.Errorf("runner failed to start: %w", runErr)
		}
		return runnerResult{}, fmt.Errorf("runner produced no output")
	}

	var res runnerResult
	if jsonErr := json.Unmarshal(stdout, &res); jsonErr != nil {
		return runnerResult{}, fmt.Errorf("runner produced unparseable output: %s", string(stdout))
	}
	return res, nil
}

// stringArgs coerces the `args` tool input (a JSON object of arbitrary
// scalar values) into the key=value string pairs the runner's positional
// args and engine.RunWorkflow's Args convention expect.
func stringArgs(v interface{}) map[string]string {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		if s, ok := val.(string); ok {
			out[k] = s
			continue
		}
		b, err := json.Marshal(val)
		if err != nil {
			continue
		}
		out[k] = string(b)
	}
	return out
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// recordRun best-effort POSTs a self-documenting history record (the full
// source + args + result of this run) to the kuetix API. A recording
// failure (not logged in, API unreachable) never fails the wsl_run call —
// execution already happened and its result is the primary contract.
func recordRun(in wslInput, runArgs map[string]string, res runnerResult) map[string]interface{} {
	client, err := newAPIClient()
	if err != nil {
		return map[string]interface{}{"recorded": false, "reason": err.Error()}
	}

	args := make(map[string]interface{}, len(runArgs))
	for k, v := range runArgs {
		args[k] = v
	}

	payload := map[string]interface{}{
		"workflow_name":  in.Filename,
		"format":         formatLabel(in.Simplified),
		"content":        in.Source,
		"args":           args,
		"success":        res.Success,
		"status_code":    res.StatusCode,
		"response":       res.Response,
		"error_message":  res.Error,
		"duration_ms":    res.DurationMs,
		"runner_version": serverVersion,
	}

	status, body, err := client.post("/run", payload)
	if err != nil {
		return map[string]interface{}{"recorded": false, "reason": err.Error()}
	}
	if status < 200 || status >= 300 {
		return map[string]interface{}{"recorded": false, "reason": fmt.Sprintf("API returned %d: %s", status, string(body))}
	}

	created := unwrapEnvelope(body)
	created["recorded"] = true
	return created
}

// unwrapEnvelope flattens the API's uniform HTTP response shape
// ({"success":bool,"data":<payload>} or {"success":false,"errors":[...]})
// down to the payload map, since every kuetix API response is wrapped this
// way regardless of endpoint.
func unwrapEnvelope(body []byte) map[string]interface{} {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return map[string]interface{}{}
	}
	if data, ok := raw["data"].(map[string]interface{}); ok {
		return data
	}
	return raw
}

func handleHistoryList(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := newAPIClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	args := req.GetArguments()
	q := "?"
	if limit, ok := args["limit"]; ok {
		q += fmt.Sprintf("limit=%v&", limit)
	}
	if cursor := getString(args, "cursor"); cursor != "" {
		q += "cursor=" + cursor
	}
	status, body, err := client.get("/run" + strings.TrimSuffix(q, "&"))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return passthroughJSON(status, body)
}

func handleHistoryGet(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getString(req.GetArguments(), "id")
	if id == "" {
		return mcp.NewToolResultError("`id` is required"), nil
	}
	client, err := newAPIClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	status, body, err := client.get("/run/get?id=" + id)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return passthroughJSON(status, body)
}

func passthroughJSON(status int, body []byte) (*mcp.CallToolResult, error) {
	return jsonResult(map[string]interface{}{
		"ok":         status >= 200 && status < 300,
		"statusCode": status,
		"result":     unwrapEnvelope(body),
	})
}
