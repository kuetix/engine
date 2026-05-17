package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"
)

// transitionReference is the standards cheat sheet returned by
// transition_reference. It grounds AI generation in the same rules as
// .claude/skills/write-module-transition.md.
const transitionReference = `# Kuetix Transition Standards

## Directory Layout
modules/
  <service>/
    <namespace>/
      transitions/
        <namespace>.go

WSL action path ` + "`<service>/<namespace>/<namespace>.Method`" + ` maps to
modules/<service>/<namespace>/transitions/<namespace>.go struct method ` + "`Method`" + `.

## File Skeleton
package transitions

import (
    "github.com/kuetix/engine/engine/domain"
    "github.com/kuetix/engine/engine/domain/interfaces"
    "github.com/kuetix/engine/engine/workflow"
)

type <namespace>Transitions struct {
    workflow.BaseServiceTransition
}

func New<Namespace>Transitions() interfaces.ServiceTransitions {
    return &<namespace>Transitions{}
}

## Rules
1. Package is always ` + "`transitions`" + `.
2. Struct name: camelCase ` + "`<namespace>Transitions`" + `.
3. Constructor: PascalCase ` + "`New<Namespace>Transitions`" + `, MUST return
   ` + "`interfaces.ServiceTransitions`" + ` (the code generator detects this).
4. Embed ` + "`workflow.BaseServiceTransition`" + ` for session helpers
   (S(), GetValue, SetValue, SetResponse, SetError, NewIssue).
5. Methods MUST be exported (PascalCase) — they become WSL action names.
6. Go parameter names MUST match WSL named-argument names exactly.

## Method Signature (canonical)
func (t *<namespace>Transitions) MethodName(arg1 <type>, arg2 <type>) (r domain.FlowStepResult) {
    // ... business logic ...

    r.Success    = true
    r.StatusCode = 200
    r.Response   = map[string]interface{}{
        "key": value,
    }
    return
}

## Return Value: domain.FlowStepResult
type FlowStepResult struct {
    Success    bool        // true => follow "on success", false => follow "on error"
    Next       string      // optional: override next state name
    Error      error       // populate when Success = false
    Response   interface{} // accessible downstream via $alias.field
    StatusCode int         // HTTP-style status (200 ok, 4xx/5xx error)
}

## Success / Error Contract
Success path:
    r.Success    = true
    r.StatusCode = 200
    r.Response   = map[string]interface{}{ ... }

Error path:
    r.Success    = false
    r.StatusCode = 4xx | 5xx
    r.Error      = fmt.Errorf("...")
    r.Response   = map[string]interface{}{"message": "...", "code": "..."}

## Supported Parameter Types
string, int, int64, float64, bool, interface{},
map[string]interface{}, []interface{}

Use ` + "`interface{}`" + ` when the caller may pass a structured object
(common for WSL ` + "`$Request`" + ` / ` + "`$User`" + ` style args).

## Special Parameter
A parameter named ` + "`p *workflow.WorkerSessionContext`" + ` injects the
full session context (Flow, Worker, parser, engine).

## After Changes
Run ` + "`kue update`" + ` to regenerate modules/di.go, modules/meta.go, modules.json.
`

// --- tool param parsing ------------------------------------------------------

type methodParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type methodSpec struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Params      []methodParam `json:"params,omitempty"`
}

func parseMethodSpec(args map[string]interface{}) (methodSpec, error) {
	var spec methodSpec
	spec.Name = getString(args, "method_name")
	spec.Description = getString(args, "description")
	if spec.Name == "" {
		return spec, fmt.Errorf("`method_name` is required")
	}
	if !isExportedIdent(spec.Name) {
		return spec, fmt.Errorf("`method_name` must be an exported Go identifier (PascalCase), got %q", spec.Name)
	}
	if raw := getString(args, "params"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &spec.Params); err != nil {
			return spec, fmt.Errorf("`params` must be JSON array of {name,type}: %w", err)
		}
	}
	for i, p := range spec.Params {
		if p.Name == "" {
			return spec, fmt.Errorf("params[%d].name is required", i)
		}
		if !isValidIdent(p.Name) {
			return spec, fmt.Errorf("params[%d].name %q is not a valid Go identifier", i, p.Name)
		}
		if p.Type == "" {
			spec.Params[i].Type = "interface{}"
		}
	}
	return spec, nil
}

// --- code rendering ----------------------------------------------------------

func renderTransitionFile(namespace string, methods []methodSpec) (string, error) {
	lowerNS := lowerFirst(namespace)
	upperNS := upperFirst(namespace)

	var b strings.Builder
	b.WriteString("package transitions\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"github.com/kuetix/engine/engine/domain\"\n")
	b.WriteString("\t\"github.com/kuetix/engine/engine/domain/interfaces\"\n")
	b.WriteString("\t\"github.com/kuetix/engine/engine/workflow\"\n")
	b.WriteString(")\n\n")

	_, _ = fmt.Fprintf(&b, "type %sTransitions struct {\n", lowerNS)
	b.WriteString("\tworkflow.BaseServiceTransition\n")
	b.WriteString("}\n\n")

	_, _ = fmt.Fprintf(&b, "func New%sTransitions() interfaces.ServiceTransitions {\n", upperNS)
	_, _ = fmt.Fprintf(&b, "\treturn &%sTransitions{}\n", lowerNS)
	b.WriteString("}\n")

	for _, m := range methods {
		b.WriteString("\n")
		b.WriteString(renderMethod(lowerNS, m))
	}

	formatted, err := format.Source([]byte(b.String()))
	if err != nil {
		return b.String(), fmt.Errorf("gofmt: %w", err)
	}
	return string(formatted), nil
}

func renderMethod(lowerNS string, m methodSpec) string {
	var b strings.Builder
	if m.Description != "" {
		for _, line := range strings.Split(strings.TrimSpace(m.Description), "\n") {
			_, _ = fmt.Fprintf(&b, "// %s\n", line)
		}
	} else {
		_, _ = fmt.Fprintf(&b, "// %s is a generated transition stub.\n", m.Name)
	}

	params := make([]string, 0, len(m.Params))
	for _, p := range m.Params {
		params = append(params, fmt.Sprintf("%s %s", p.Name, p.Type))
	}
	_, _ = fmt.Fprintf(&b, "func (t *%sTransitions) %s(%s) (r domain.FlowStepResult) {\n",
		lowerNS, m.Name, strings.Join(params, ", "))

	b.WriteString("\tr.Success = true\n")
	b.WriteString("\tr.StatusCode = 200\n")
	b.WriteString("\tr.Response = map[string]interface{}{}\n")
	b.WriteString("\treturn\n")
	b.WriteString("}\n")
	return b.String()
}

// --- handlers ----------------------------------------------------------------

func handleTransitionReference(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(transitionReference), nil
}

func handleTransitionScaffold(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	service := strings.Trim(getString(args, "service"), "/ ")
	namespace := strings.Trim(getString(args, "namespace"), "/ ")
	baseDir := getString(args, "base_dir")
	if baseDir == "" {
		baseDir = "modules"
	}
	overwrite := false
	if v, ok := args["overwrite"].(bool); ok {
		overwrite = v
	}

	if service == "" || namespace == "" {
		return mcp.NewToolResultError("`service` and `namespace` are required"), nil
	}
	if !isValidPathSegment(service) || !isValidPathSegment(namespace) {
		return mcp.NewToolResultError("`service` and `namespace` must be lowercase Go-identifier path segments"), nil
	}

	var methods []methodSpec
	if raw := getString(args, "methods"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &methods); err != nil {
			return mcp.NewToolResultErrorf("`methods` must be JSON array of {name,description,params}: %v", err), nil
		}
		for i, m := range methods {
			if !isExportedIdent(m.Name) {
				return mcp.NewToolResultErrorf("methods[%d].name %q must be exported (PascalCase)", i, m.Name), nil
			}
			for j, p := range m.Params {
				if !isValidIdent(p.Name) {
					return mcp.NewToolResultErrorf("methods[%d].params[%d].name %q is not a valid identifier", i, j, p.Name), nil
				}
				if p.Type == "" {
					methods[i].Params[j].Type = "interface{}"
				}
			}
		}
	}

	dir := filepath.Join(baseDir, service, namespace, "transitions")
	file := filepath.Join(dir, namespace+".go")

	if _, err := os.Stat(file); err == nil && !overwrite {
		return jsonResult(map[string]interface{}{
			"ok":    false,
			"error": fmt.Sprintf("%s already exists (pass overwrite=true to replace, or use transition_add_method)", file),
			"path":  file,
		})
	}

	source, err := renderTransitionFile(namespace, methods)
	if err != nil {
		return mcp.NewToolResultErrorf("render: %v", err), nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return mcp.NewToolResultErrorf("mkdir %s: %v", dir, err), nil
	}
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		return mcp.NewToolResultErrorf("write %s: %v", file, err), nil
	}

	return jsonResult(map[string]interface{}{
		"ok":         true,
		"path":       file,
		"dir":        dir,
		"wsl_import": fmt.Sprintf("%s/%s", service, namespace),
		"wsl_action": fmt.Sprintf("%s/%s/%s.<Method>", service, namespace, namespace),
		"methods":    len(methods),
		"next_step":  "run `kue update` to regenerate modules/di.go and modules/meta.go",
		"source":     source,
	})
}

func handleTransitionAddMethod(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	path := getString(args, "path")
	if path == "" {
		return mcp.NewToolResultError("`path` is required"), nil
	}
	spec, err := parseMethodSpec(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return mcp.NewToolResultErrorf("read %s: %v", path, err), nil
	}
	content := string(data)

	namespace := strings.TrimSuffix(filepath.Base(path), ".go")
	lowerNS := lowerFirst(namespace)

	receiverMarker := fmt.Sprintf("*%sTransitions)", lowerNS)
	if !strings.Contains(content, receiverMarker) {
		return mcp.NewToolResultErrorf(
			"could not find struct %q in %s — is this a transition file? (expected receiver %s)",
			lowerNS+"Transitions", path, receiverMarker), nil
	}

	methodMarker := fmt.Sprintf("func (t *%sTransitions) %s(", lowerNS, spec.Name)
	if strings.Contains(content, methodMarker) {
		return jsonResult(map[string]interface{}{
			"ok":    false,
			"error": fmt.Sprintf("method %s already exists in %s", spec.Name, path),
		})
	}

	snippet := renderMethod(lowerNS, spec)
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "\n" + snippet

	formatted, ferr := format.Source([]byte(content))
	if ferr != nil {
		return mcp.NewToolResultErrorf("gofmt after insert: %v", ferr), nil
	}
	if err := os.WriteFile(path, formatted, 0o644); err != nil {
		return mcp.NewToolResultErrorf("write %s: %v", path, err), nil
	}

	return jsonResult(map[string]interface{}{
		"ok":        true,
		"path":      path,
		"method":    spec.Name,
		"snippet":   snippet,
		"next_step": "run `kue update` to refresh modules/di.go and modules/meta.go",
	})
}

// --- validation --------------------------------------------------------------

type transitionDiagnostic struct {
	Path     string `json:"path"`
	Line     int    `json:"line,omitempty"`
	Severity string `json:"severity"` // "error" | "warning"
	Message  string `json:"message"`
}

type transitionFileReport struct {
	Path        string                 `json:"path"`
	OK          bool                   `json:"ok"`
	Namespace   string                 `json:"namespace,omitempty"`
	Struct      string                 `json:"struct,omitempty"`
	Constructor string                 `json:"constructor,omitempty"`
	Methods     []string               `json:"methods,omitempty"`
	Diagnostics []transitionDiagnostic `json:"diagnostics,omitempty"`
}

func validateTransitionFile(path string) transitionFileReport {
	rep := transitionFileReport{Path: path}
	add := func(sev, msg string, line int) {
		rep.Diagnostics = append(rep.Diagnostics, transitionDiagnostic{
			Path: path, Line: line, Severity: sev, Message: msg,
		})
	}

	data, err := os.ReadFile(path)
	if err != nil {
		add("error", fmt.Sprintf("read: %v", err), 0)
		return rep
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, parser.ParseComments)
	if err != nil {
		add("error", fmt.Sprintf("parse: %v", err), 0)
		return rep
	}

	if file.Name.Name != "transitions" {
		add("error", fmt.Sprintf("package must be `transitions`, got %q", file.Name.Name),
			fset.Position(file.Name.Pos()).Line)
	}

	namespace := strings.TrimSuffix(filepath.Base(path), ".go")
	rep.Namespace = namespace
	expectStruct := lowerFirst(namespace) + "Transitions"
	expectCtor := "New" + upperFirst(namespace) + "Transitions"
	rep.Struct = expectStruct
	rep.Constructor = expectCtor

	var (
		structDecl *ast.StructType
		ctorDecl   *ast.FuncDecl
	)

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Name.Name != expectStruct {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					add("error", fmt.Sprintf("%s must be a struct", expectStruct), fset.Position(ts.Pos()).Line)
					continue
				}
				structDecl = st
			}
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.Name == expectCtor {
				ctorDecl = d
			}
		}
	}

	if structDecl == nil {
		add("error", fmt.Sprintf("struct %q not found (file %s.go must define it)", expectStruct, namespace), 0)
	} else {
		embedsBase := false
		for _, f := range structDecl.Fields.List {
			if len(f.Names) != 0 {
				continue
			}
			if sel, ok := f.Type.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "workflow" && sel.Sel.Name == "BaseServiceTransition" {
					embedsBase = true
				}
			}
		}
		if !embedsBase {
			add("error", "struct must embed workflow.BaseServiceTransition", fset.Position(structDecl.Pos()).Line)
		}
	}

	if ctorDecl == nil {
		add("error", fmt.Sprintf("constructor %q not found", expectCtor), 0)
	} else {
		if ctorDecl.Type.Results == nil || len(ctorDecl.Type.Results.List) != 1 {
			add("error", fmt.Sprintf("%s must return exactly one value (interfaces.ServiceTransitions)", expectCtor),
				fset.Position(ctorDecl.Pos()).Line)
		} else {
			ret := ctorDecl.Type.Results.List[0].Type
			sel, ok := ret.(*ast.SelectorExpr)
			if !ok {
				add("error", fmt.Sprintf("%s return type must be interfaces.ServiceTransitions", expectCtor),
					fset.Position(ctorDecl.Pos()).Line)
			} else if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "interfaces" || sel.Sel.Name != "ServiceTransitions" {
				add("error", fmt.Sprintf("%s return type must be interfaces.ServiceTransitions, got %s.%s",
					expectCtor, identName(sel.X), sel.Sel.Name), fset.Position(ctorDecl.Pos()).Line)
			}
		}
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		if !isReceiverStruct(fn.Recv.List[0].Type, expectStruct) {
			continue
		}
		if !ast.IsExported(fn.Name.Name) {
			add("warning", fmt.Sprintf("method %s is unexported — WSL can only call exported methods", fn.Name.Name),
				fset.Position(fn.Pos()).Line)
			continue
		}
		rep.Methods = append(rep.Methods, fn.Name.Name)

		line := fset.Position(fn.Pos()).Line
		if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
			add("error", fmt.Sprintf("%s: must return exactly one value `(r domain.FlowStepResult)`", fn.Name.Name), line)
			continue
		}
		result := fn.Type.Results.List[0]
		if len(result.Names) == 0 {
			add("warning", fmt.Sprintf("%s: should use a named return `r domain.FlowStepResult`", fn.Name.Name), line)
		}
		sel, ok := result.Type.(*ast.SelectorExpr)
		if !ok {
			add("error", fmt.Sprintf("%s: return type must be domain.FlowStepResult", fn.Name.Name), line)
			continue
		}
		if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "domain" || sel.Sel.Name != "FlowStepResult" {
			add("error", fmt.Sprintf("%s: return type must be domain.FlowStepResult, got %s.%s",
				fn.Name.Name, identName(sel.X), sel.Sel.Name), line)
			continue
		}

		assigns := collectAssignedFields(fn.Body, "r")
		if !assigns["Success"] {
			add("warning", fmt.Sprintf("%s: does not set r.Success (required by contract — true for ok, false for error)", fn.Name.Name), line)
		}
		if !assigns["StatusCode"] {
			add("warning", fmt.Sprintf("%s: does not set r.StatusCode", fn.Name.Name), line)
		}
		if !assigns["Response"] && !assigns["Error"] {
			add("warning", fmt.Sprintf("%s: sets neither r.Response nor r.Error", fn.Name.Name), line)
		}
	}

	hasError := false
	for _, d := range rep.Diagnostics {
		if d.Severity == "error" {
			hasError = true
			break
		}
	}
	rep.OK = !hasError
	return rep
}

func identName(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return "?"
}

func isReceiverStruct(recv ast.Expr, structName string) bool {
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	id, ok := recv.(*ast.Ident)
	return ok && id.Name == structName
}

// collectAssignedFields walks a method body looking for `r.<Field> = ...`
// assignments so we can check the FlowStepResult success contract.
func collectAssignedFields(body *ast.BlockStmt, recvName string) map[string]bool {
	found := map[string]bool{}
	if body == nil {
		return found
	}
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range as.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			x, ok := sel.X.(*ast.Ident)
			if !ok || x.Name != recvName {
				continue
			}
			found[sel.Sel.Name] = true
		}
		return true
	})
	return found
}

func handleTransitionValidate(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	path := getString(args, "path")
	dir := getString(args, "dir")
	if path == "" && dir == "" {
		dir = "modules"
	}

	var targets []string
	if path != "" {
		targets = append(targets, path)
	}
	if dir != "" {
		werr := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if filepath.Ext(p) != ".go" {
				return nil
			}
			if filepath.Base(filepath.Dir(p)) != "transitions" {
				return nil
			}
			targets = append(targets, p)
			return nil
		})
		if werr != nil {
			return mcp.NewToolResultErrorf("walk %s: %v", dir, werr), nil
		}
	}

	if len(targets) == 0 {
		return jsonResult(map[string]interface{}{
			"ok":      true,
			"message": "no transition files found",
			"files":   0,
		})
	}

	reports := make([]transitionFileReport, 0, len(targets))
	allOK := true
	totalErrors, totalWarnings := 0, 0
	for _, t := range targets {
		rep := validateTransitionFile(t)
		reports = append(reports, rep)
		if !rep.OK {
			allOK = false
		}
		for _, d := range rep.Diagnostics {
			if d.Severity == "error" {
				totalErrors++
			} else {
				totalWarnings++
			}
		}
	}

	return jsonResult(map[string]interface{}{
		"ok":       allOK,
		"files":    len(reports),
		"errors":   totalErrors,
		"warnings": totalWarnings,
		"reports":  reports,
	})
}

// --- identifier helpers ------------------------------------------------------

func isValidIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && !(unicode.IsLetter(r) || r == '_') {
			return false
		}
		if i > 0 && !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			return false
		}
	}
	return true
}

func isExportedIdent(s string) bool {
	if !isValidIdent(s) {
		return false
	}
	return unicode.IsUpper(rune(s[0]))
}

func isValidPathSegment(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}
