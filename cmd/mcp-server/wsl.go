package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/engine/internal/wsl"
)

// wslInput is the common shape accepted by parse/validate/explain tools.
type wslInput struct {
	Source     string
	Filename   string
	Simplified bool
}

// loadWSLInput resolves the source text + filename + format from a tool request.
func loadWSLInput(req mcp.CallToolRequest) (wslInput, error) {
	args := req.GetArguments()
	in := wslInput{
		Source:   getString(args, "source"),
		Filename: getString(args, "filename"),
	}
	path := getString(args, "path")
	simplifiedSet := false
	if v, ok := args["simplified"].(bool); ok {
		in.Simplified = v
		simplifiedSet = true
	}

	if in.Source == "" && path == "" {
		return in, fmt.Errorf("either `source` or `path` is required")
	}

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return in, fmt.Errorf("read %s: %w", path, err)
		}
		in.Source = string(data)
		if in.Filename == "" {
			in.Filename = filepath.Base(path)
		}
		if !simplifiedSet {
			in.Simplified = strings.EqualFold(filepath.Ext(path), ".swsl")
			simplifiedSet = true
		}
	}

	// Inline source: infer `simplified` from filename extension when given,
	// otherwise sniff the source text. This lets callers pass raw WSL/SWSL
	// without a file on disk and without having to set `simplified`.
	if !simplifiedSet {
		if in.Filename != "" {
			ext := strings.ToLower(filepath.Ext(in.Filename))
			if ext == ".swsl" {
				in.Simplified = true
				simplifiedSet = true
			} else if ext == ".wsl" {
				in.Simplified = false
				simplifiedSet = true
			}
		}
		if !simplifiedSet && in.Source != "" {
			in.Simplified = looksLikeSWSL(in.Source)
		}
	}

	if in.Filename == "" {
		if in.Simplified {
			in.Filename = "inline.swsl"
		} else {
			in.Filename = "inline.wsl"
		}
	}
	return in, nil
}

// looksLikeSWSL heuristically distinguishes SWSL from WSL source. SWSL
// uses implicit chaining with `->` and terminal `.` and never declares
// `state` / `end ok` / `end fail`, which are required markers in WSL.
func looksLikeSWSL(src string) bool {
	lower := src
	hasState := strings.Contains(lower, "\nstate ") || strings.HasPrefix(lower, "state ")
	hasEndOk := strings.Contains(lower, "end ok") || strings.Contains(lower, "end fail")
	if hasState || hasEndOk {
		return false
	}
	if strings.Contains(lower, "->") {
		return true
	}
	return false
}

func parseModule(in wslInput) (*wsl.Module, map[string]*wsl.Graph, error) {
	if in.Simplified {
		return wsl.ParseAllSimplifiedWithFilename(in.Source, in.Filename)
	}
	return wsl.ParseAll(in.Source, in.Filename)
}

// --- summaries ---------------------------------------------------------------

type moduleSummary struct {
	Module    string            `json:"module"`
	Filename  string            `json:"filename"`
	Format    string            `json:"format"`
	Imports   []importSummary   `json:"imports,omitempty"`
	Constants []string          `json:"constants,omitempty"`
	Workflows []workflowSummary `json:"workflows"`
}

type importSummary struct {
	Path string `json:"path"`
	As   string `json:"as,omitempty"`
}

type workflowSummary struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Start  string   `json:"start"`
	States []string `json:"states"`
}

func summarize(in wslInput, mod *wsl.Module) moduleSummary {
	out := moduleSummary{
		Module:   mod.Name,
		Filename: in.Filename,
		Format:   formatLabel(in.Simplified),
	}
	for _, imp := range mod.Imports {
		out.Imports = append(out.Imports, importSummary{Path: imp.Path, As: imp.As})
	}
	for _, c := range mod.Constants {
		out.Constants = append(out.Constants, c.Name)
	}
	for _, wf := range mod.Workflows {
		ws := workflowSummary{Name: wf.Name, Type: wf.Type, Start: wf.Start}
		for name := range wf.States {
			ws.States = append(ws.States, name)
		}
		out.Workflows = append(out.Workflows, ws)
	}
	return out
}

// --- detailed explain --------------------------------------------------------

type moduleExplain struct {
	Module    string                 `json:"module"`
	Filename  string                 `json:"filename"`
	Format    string                 `json:"format"`
	Imports   []importSummary        `json:"imports,omitempty"`
	Constants map[string]interface{} `json:"constants,omitempty"`
	Workflows []workflowExplain      `json:"workflows"`
}

type workflowExplain struct {
	Name   string         `json:"name"`
	Type   string         `json:"type"`
	Start  string         `json:"start"`
	States []stateExplain `json:"states"`
}

type stateExplain struct {
	Name           string              `json:"name"`
	Params         []string            `json:"params,omitempty"`
	Action         *actionExplain      `json:"action,omitempty"`
	Transitions    []transitionExplain `json:"transitions,omitempty"`
	IfExpr         string              `json:"if,omitempty"`
	ContinueOnFail bool                `json:"continueOnFail,omitempty"`
	SkipTo         bool                `json:"skipTo,omitempty"`
	Terminal       string              `json:"terminal,omitempty"` // "ok" | "fail" | ""
}

type actionExplain struct {
	Module string   `json:"module,omitempty"`
	Name   string   `json:"name"`
	Args   []string `json:"args,omitempty"`
	As     string   `json:"as,omitempty"`
}

type transitionExplain struct {
	On     string   `json:"on"` // "success" | "error" | "else" | "expr"
	When   string   `json:"when,omitempty"`
	Target string   `json:"target"`
	Args   []string `json:"args,omitempty"`
}

func explain(in wslInput, mod *wsl.Module) moduleExplain {
	out := moduleExplain{
		Module:   mod.Name,
		Filename: in.Filename,
		Format:   formatLabel(in.Simplified),
	}
	for _, imp := range mod.Imports {
		out.Imports = append(out.Imports, importSummary{Path: imp.Path, As: imp.As})
	}
	if len(mod.Constants) > 0 {
		out.Constants = map[string]interface{}{}
		for _, c := range mod.Constants {
			out.Constants[c.Name] = c.Value
		}
	}
	for _, wf := range mod.Workflows {
		we := workflowExplain{Name: wf.Name, Type: wf.Type, Start: wf.Start}
		for _, st := range wf.States {
			we.States = append(we.States, explainState(st))
		}
		out.Workflows = append(out.Workflows, we)
	}
	return out
}

func explainState(st *wsl.State) stateExplain {
	out := stateExplain{
		Name:           st.Name,
		Params:         st.Params,
		ContinueOnFail: st.ContinueOnFail,
		SkipTo:         st.SkipTo,
	}
	if st.IfExpr != nil {
		out.IfExpr = st.IfExpr.Raw
	}
	if st.Action != nil {
		out.Action = &actionExplain{
			Module: st.Action.Module,
			Name:   st.Action.Name,
			Args:   exprList(st.Action.Args),
			As:     st.Action.As,
		}
	}
	for _, tr := range st.Transitions {
		te := transitionExplain{
			On:     tr.Condition.Kind,
			Target: tr.Target,
			Args:   exprList(tr.Args),
		}
		if tr.WhenExpr != nil {
			te.When = tr.WhenExpr.Raw
		}
		out.Transitions = append(out.Transitions, te)
	}
	if st.End != nil {
		out.Terminal = st.End.Kind
	}
	return out
}

func exprList(in []wsl.Expr) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, e := range in {
		out[i] = e.Raw
	}
	return out
}

func formatLabel(simplified bool) string {
	if simplified {
		return "swsl"
	}
	return "wsl"
}

// --- handlers ----------------------------------------------------------------

func handleParse(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := loadWSLInput(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	mod, _, perr := parseModule(in)
	if perr != nil {
		return jsonResult(map[string]interface{}{
			"ok":    false,
			"error": perr.Error(),
		})
	}
	return jsonResult(map[string]interface{}{
		"ok":      true,
		"summary": summarize(in, mod),
	})
}

func handleValidate(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := loadWSLInput(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	mod, _, perr := parseModule(in)
	if perr != nil {
		return jsonResult(map[string]interface{}{
			"ok":          false,
			"diagnostics": []string{perr.Error()},
		})
	}
	return jsonResult(map[string]interface{}{
		"ok":        true,
		"module":    mod.Name,
		"workflows": len(mod.Workflows),
		"format":    formatLabel(in.Simplified),
	})
}

func handleExplain(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := loadWSLInput(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	mod, _, perr := parseModule(in)
	if perr != nil {
		return jsonResult(map[string]interface{}{
			"ok":    false,
			"error": perr.Error(),
		})
	}
	return jsonResult(map[string]interface{}{
		"ok":      true,
		"explain": explain(in, mod),
	})
}

func handleWorkflowActions(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := loadWSLInput(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	mod, graphs, perr := parseModule(in)
	if perr != nil {
		return jsonResult(map[string]interface{}{
			"ok":    false,
			"error": perr.Error(),
		})
	}

	actions := workflow.ExtractActionsFromGraphs(mod, graphs)

	// Group by workflow so consumers can reason per-workflow when a
	// module defines multiple workflows.
	byWorkflow := map[string][]workflow.WorkflowAction{}
	for _, a := range actions {
		byWorkflow[a.Workflow] = append(byWorkflow[a.Workflow], a)
	}

	// Collect a unique action catalog keyed by "module/name" — this is
	// what a generator needs to reference the same action from another
	// WSL file (import path + action method + expected arg names).
	type catalogEntry struct {
		Module     string   `json:"module,omitempty"`
		Name       string   `json:"name"`
		Ref        string   `json:"ref"`
		Import     string   `json:"import,omitempty"`
		ArgNames   []string `json:"argNames,omitempty"`
		UsedBy     []string `json:"usedBy,omitempty"`
		SampleArgs []string `json:"sampleArgs,omitempty"`
	}
	catalog := map[string]*catalogEntry{}
	for _, a := range actions {
		key := a.Module + "." + a.Name
		entry, ok := catalog[key]
		if !ok {
			ref := a.Name
			if a.Module != "" {
				ref = a.Module + "." + a.Name
			}
			entry = &catalogEntry{
				Module: a.Module,
				Name:   a.Name,
				Ref:    ref,
				Import: a.Module,
			}
			catalog[key] = entry
		}
		for _, n := range a.ArgNames {
			if !containsString(entry.ArgNames, n) {
				entry.ArgNames = append(entry.ArgNames, n)
			}
		}
		usage := a.Workflow + "." + a.State
		if !containsString(entry.UsedBy, usage) {
			entry.UsedBy = append(entry.UsedBy, usage)
		}
		if len(entry.SampleArgs) == 0 {
			for _, ar := range a.Args {
				entry.SampleArgs = append(entry.SampleArgs, ar.Raw)
			}
		}
	}
	catalogList := make([]*catalogEntry, 0, len(catalog))
	for _, e := range catalog {
		catalogList = append(catalogList, e)
	}

	return jsonResult(map[string]interface{}{
		"ok":         true,
		"module":     mod.Name,
		"filename":   in.Filename,
		"format":     formatLabel(in.Simplified),
		"count":      len(actions),
		"actions":    actions,
		"byWorkflow": byWorkflow,
		"catalog":    catalogList,
	})
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func handleListWorkflows(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dir := getString(req.GetArguments(), "dir")
	if dir == "" {
		dir = "runtime/workflows"
	}
	info, err := os.Stat(dir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("stat %s: %v", dir, err)), nil
	}
	if !info.IsDir() {
		return mcp.NewToolResultError(fmt.Sprintf("%s is not a directory", dir)), nil
	}

	type entry struct {
		Path   string `json:"path"`
		Format string `json:"format"`
	}
	var files []entry
	walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".wsl":
			files = append(files, entry{Path: path, Format: "wsl"})
		case ".swsl":
			files = append(files, entry{Path: path, Format: "swsl"})
		}
		return nil
	})
	if walkErr != nil {
		return mcp.NewToolResultError(fmt.Sprintf("walk %s: %v", dir, walkErr)), nil
	}
	return jsonResult(map[string]interface{}{
		"ok":    true,
		"dir":   dir,
		"count": len(files),
		"files": files,
	})
}

func handleSyntaxReference(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(syntaxReference), nil
}

// --- helpers -----------------------------------------------------------------

func getString(args map[string]interface{}, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func jsonResult(payload interface{}) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return mcp.NewToolResultErrorf("marshal: %v", err), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}
