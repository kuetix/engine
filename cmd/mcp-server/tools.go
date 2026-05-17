package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("wsl_parse",
		mcp.WithDescription("Parse a WSL or SWSL workflow and return a structured summary (module, imports, constants, workflows, states). Provide either `source` (raw text) or `path` (file path). Format is auto-detected from extension or the `simplified` flag."),
		mcp.WithString("source", mcp.Description("Raw WSL/SWSL source text")),
		mcp.WithString("path", mcp.Description("Path to a .wsl or .swsl file (relative to working directory or absolute)")),
		mcp.WithString("filename", mcp.Description("Logical filename used for module-name inference when only `source` is provided")),
		mcp.WithBoolean("simplified", mcp.Description("Force SWSL parser. If omitted, the format is inferred from the file extension")),
	), handleParse)

	s.AddTool(mcp.NewTool("wsl_validate",
		mcp.WithDescription("Validate a WSL/SWSL workflow. Returns ok=true on success or a list of diagnostics. Same input contract as wsl_parse."),
		mcp.WithString("source", mcp.Description("Raw WSL/SWSL source text")),
		mcp.WithString("path", mcp.Description("Path to a .wsl or .swsl file")),
		mcp.WithString("filename", mcp.Description("Logical filename for SWSL module-name inference")),
		mcp.WithBoolean("simplified", mcp.Description("Force SWSL parser")),
	), handleValidate)

	s.AddTool(mcp.NewTool("wsl_list_workflows",
		mcp.WithDescription("Recursively list .wsl and .swsl files under a directory. Defaults to runtime/workflows."),
		mcp.WithString("dir", mcp.Description("Directory to scan (default: runtime/workflows)")),
	), handleListWorkflows)

	s.AddTool(mcp.NewTool("wsl_explain",
		mcp.WithDescription("Return a detailed structured explanation of a WSL/SWSL workflow: every state with its action, transitions (including when-conditions), parameters, terminal kind, and flags. Same input contract as wsl_parse."),
		mcp.WithString("source", mcp.Description("Raw WSL/SWSL source text")),
		mcp.WithString("path", mcp.Description("Path to a .wsl or .swsl file")),
		mcp.WithString("filename", mcp.Description("Logical filename for SWSL module-name inference")),
		mcp.WithBoolean("simplified", mcp.Description("Force SWSL parser")),
	), handleExplain)

	s.AddTool(mcp.NewTool("wsl_workflow_actions",
		mcp.WithDescription("Parse a WSL/SWSL workflow via the standard engine pipeline (AST + IR graphs) and return every service action it references with its module, method name, named argument names, raw arg text, state, params, and terminal kind. Also returns a deduplicated `catalog` keyed by `module.name` with the WSL import path, unique argNames, and sample args — exactly what's needed to reference the same actions from a newly generated WSL file. Accepts EITHER an on-disk file via `path` OR raw in-memory WSL/SWSL via `source` (no file required). When `source` is given, the WSL/SWSL format is auto-inferred from a `.wsl`/`.swsl` filename hint or by sniffing the source; override with `simplified`."),
		mcp.WithString("source", mcp.Description("Raw WSL/SWSL source text. Use this when the workflow is not (or not yet) on disk — e.g. generated in-memory or sent from a client. Either `source` or `path` must be provided.")),
		mcp.WithString("path", mcp.Description("Path to a .wsl or .swsl file on disk. Either `source` or `path` must be provided.")),
		mcp.WithString("filename", mcp.Description("Logical filename used for module-name inference and format auto-detection when only `source` is provided (e.g. 'order.swsl').")),
		mcp.WithBoolean("simplified", mcp.Description("Force SWSL parser. Omit to auto-detect from file/filename extension or source sniffing.")),
	), handleWorkflowActions)

	s.AddTool(mcp.NewTool("wsl_syntax_reference",
		mcp.WithDescription("Return the WSL syntax cheat sheet (grammar overview, transitions, when-expressions, variable refs, action arguments, orchestration). Use this to ground workflow generation."),
	), handleSyntaxReference)

	s.AddTool(mcp.NewTool("transition_reference",
		mcp.WithDescription("Return the kuetix Transition standards cheat sheet: folder layout, package/struct/constructor rules, FlowStepResult success/error contract, supported parameter types. Use this to ground Go transition generation."),
	), handleTransitionReference)

	s.AddTool(mcp.NewTool("transition_scaffold",
		mcp.WithDescription("Create a new kuetix transition module. Generates modules/<service>/<namespace>/transitions/<namespace>.go with the canonical package, struct, constructor, and optional method stubs. Each stub returns domain.FlowStepResult with Success=true, StatusCode=200, Response=map[string]interface{}{}. After running, call `kue update` to regenerate modules/di.go and modules/meta.go."),
		mcp.WithString("service", mcp.Description("Top-level service directory (e.g. 'billing'). Required."), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace under service; also becomes the struct/file name (e.g. 'payment'). Required."), mcp.Required()),
		mcp.WithString("base_dir", mcp.Description("Root modules directory (default: 'modules').")),
		mcp.WithString("methods", mcp.Description("Optional JSON array of initial method stubs: [{\"name\":\"ChargeCard\",\"description\":\"...\",\"params\":[{\"name\":\"amount\",\"type\":\"interface{}\"},{\"name\":\"currency\",\"type\":\"string\"}]}]. Param types default to interface{} when omitted.")),
		mcp.WithBoolean("overwrite", mcp.Description("Overwrite an existing transition file. Default false.")),
	), handleTransitionScaffold)

	s.AddTool(mcp.NewTool("transition_add_method",
		mcp.WithDescription("Append a new method to an existing kuetix transition file. The method follows the standard contract: named return `(r domain.FlowStepResult)`, success path sets r.Success=true / StatusCode=200 / Response=map[string]interface{}{}. Fails if the method name already exists. Go-formats the result."),
		mcp.WithString("path", mcp.Description("Path to the transition .go file (e.g. modules/billing/payment/transitions/payment.go). Required."), mcp.Required()),
		mcp.WithString("method_name", mcp.Description("Exported PascalCase method name (must match the WSL action method name). Required."), mcp.Required()),
		mcp.WithString("params", mcp.Description("JSON array of parameters: [{\"name\":\"amount\",\"type\":\"interface{}\"},{\"name\":\"currency\",\"type\":\"string\"}]. Names must match WSL named arguments exactly. Types default to interface{}.")),
		mcp.WithString("description", mcp.Description("Optional doc comment for the method.")),
	), handleTransitionAddMethod)

	s.AddTool(mcp.NewTool("transition_validate",
		mcp.WithDescription("Validate kuetix transition Go files against the standards: package=transitions, struct name `<namespace>Transitions` embedding workflow.BaseServiceTransition, constructor `New<Namespace>Transitions` returning interfaces.ServiceTransitions, exported methods with named return `(r domain.FlowStepResult)`, and the success contract (sets r.Success, r.StatusCode, r.Response or r.Error). Provide `path` for a single file or `dir` to walk recursively (defaults to 'modules'). Returns per-file diagnostics with severity and line numbers."),
		mcp.WithString("path", mcp.Description("Path to a single transition .go file to validate.")),
		mcp.WithString("dir", mcp.Description("Directory to walk (default: 'modules'). Only files under a 'transitions/' folder are checked.")),
	), handleTransitionValidate)
}
