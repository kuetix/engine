package wsl

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Expression sub-language.
//
// WSL `when` / `if` conditions and (later) action arguments are small
// expressions: comparisons, boolean logic, arithmetic, string building, and
// paths into the workflow context. The main WSL grammar captures them as raw
// text (see parser.go parseExprUntilArrow etc.); this file parses that raw text
// into a typed tree that the engine evaluates at runtime.
//
// It has its own tiny lexer rather than reusing the WSL lexer: the WSL lexer
// treats '-' and '/' as identifier characters (to support qualified action
// names like `ns/mod.Do`), which would make `a - b` and `a / b` unlexable as
// arithmetic. Inside an expression those characters are always operators.

// ExprNode is one node of a parsed expression tree.
type ExprNode interface{ isExprNode() }

// LitExpr is a literal: int, float, string, bool, or null.
type LitExpr struct {
	// Kind is "int" | "float" | "string" | "bool" | "null".
	Kind  string
	Int   int64
	Float float64
	Str   string
	Bool  bool
}

// PathExpr is a reference into the evaluation scope, e.g. `result.ok` or
// `constants.version`. A `<<...>>` token lowers to a PathExpr too.
type PathExpr struct{ Path string }

// UnaryExpr is `!x` or `-x`.
type UnaryExpr struct {
	Op string // "!" | "-"
	X  ExprNode
}

// BinaryExpr is any infix operator.
type BinaryExpr struct {
	Op   string // + - * / % == != < <= > >= && || ??
	L, R ExprNode
}

// CallExpr is a call to one of the whitelisted pure builtins.
type CallExpr struct {
	Name string
	Args []ExprNode
}

// ListExpr is `[a, b, c]`.
type ListExpr struct{ Elems []ExprNode }

// MapExpr is `{k: v, ...}` (string keys only).
type MapExpr struct {
	Keys []string
	Vals []ExprNode
}

// IndexExpr is `x[i]` (list index or map key).
type IndexExpr struct {
	X     ExprNode
	Index ExprNode
}

// InterpExpr is a string literal containing `${expr}` holes. Parts alternate
// freely between literal string chunks (LitExpr Kind "string") and expressions.
type InterpExpr struct{ Parts []ExprNode }

func (*LitExpr) isExprNode()    {}
func (*PathExpr) isExprNode()   {}
func (*UnaryExpr) isExprNode()  {}
func (*BinaryExpr) isExprNode() {}
func (*CallExpr) isExprNode()   {}
func (*ListExpr) isExprNode()   {}
func (*MapExpr) isExprNode()    {}
func (*IndexExpr) isExprNode()  {}
func (*InterpExpr) isExprNode() {}

// ExprBuiltins is the whitelist of pure functions callable from an expression.
// They have no side effects and cannot reach service transitions.
var ExprBuiltins = map[string]bool{
	"len": true, "lower": true, "upper": true, "contains": true,
	"startsWith": true, "endsWith": true, "int": true, "float": true,
	"string": true, "bool": true, "default": true, "has": true,
}

// ParseExpr parses raw expression text into a tree. The text is what the WSL
// parser collected between markers (e.g. `result.ok == true && count > 0`).
func ParseExpr(raw string) (ExprNode, error) {
	toks, err := lexExpr(raw)
	if err != nil {
		return nil, err
	}
	p := &exprParser{toks: toks, src: raw}
	node, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	if p.cur().kind != etEOF {
		return nil, fmt.Errorf("expression %q: unexpected %q after a complete expression", raw, p.cur().text)
	}
	return node, nil
}

// --- expression lexer ---

type exprTokKind int

const (
	etEOF exprTokKind = iota
	etNumber
	etString // raw string content, quotes stripped, escapes NOT yet processed
	etIdent
	etPath // content of a <<...>> token
	etOp   // any operator or punctuator, text holds it
)

type exprTok struct {
	kind exprTokKind
	text string
}

func lexExpr(src string) ([]exprTok, error) {
	var toks []exprTok
	i := 0
	n := len(src)
	for i < n {
		c := src[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		// <<path>> token
		if c == '<' && i+1 < n && src[i+1] == '<' {
			end := strings.Index(src[i+2:], ">>")
			if end < 0 {
				return nil, fmt.Errorf("expression %q: unterminated '<<' token", src)
			}
			inner := strings.TrimSpace(src[i+2 : i+2+end])
			toks = append(toks, exprTok{etPath, inner})
			i += 2 + end + 2
			continue
		}
		// string literal
		if c == '"' || c == '\'' {
			quote := c
			j := i + 1
			var sb strings.Builder
			closed := false
			for j < n {
				if src[j] == '\\' && j+1 < n {
					sb.WriteByte(src[j])
					sb.WriteByte(src[j+1])
					j += 2
					continue
				}
				if src[j] == quote {
					closed = true
					j++
					break
				}
				sb.WriteByte(src[j])
				j++
			}
			if !closed {
				return nil, fmt.Errorf("expression %q: unterminated string literal", src)
			}
			toks = append(toks, exprTok{etString, sb.String()})
			i = j
			continue
		}
		// number
		if c >= '0' && c <= '9' {
			j := i
			for j < n && (src[j] >= '0' && src[j] <= '9' || src[j] == '_') {
				j++
			}
			if j < n && src[j] == '.' && j+1 < n && src[j+1] >= '0' && src[j+1] <= '9' {
				j++
				for j < n && (src[j] >= '0' && src[j] <= '9' || src[j] == '_') {
					j++
				}
			}
			toks = append(toks, exprTok{etNumber, strings.ReplaceAll(src[i:j], "_", "")})
			i = j
			continue
		}
		// two-char operators
		if i+1 < n {
			two := src[i : i+2]
			switch two {
			case "==", "!=", "<=", ">=", "&&", "||", "??":
				toks = append(toks, exprTok{etOp, two})
				i += 2
				continue
			}
		}
		// single-char operators / punctuators
		if strings.IndexByte("+-*/%<>!()[]{},:", c) >= 0 {
			toks = append(toks, exprTok{etOp, string(c)})
			i++
			continue
		}
		// identifier / path (letters, digits, '_', '$', '.')
		r, _ := utf8.DecodeRuneInString(src[i:])
		if r == '_' || r == '$' || unicode.IsLetter(r) {
			j := i
			for j < n {
				rr, sz := utf8.DecodeRuneInString(src[j:])
				if rr == '_' || rr == '$' || rr == '.' || unicode.IsLetter(rr) || unicode.IsDigit(rr) {
					j += sz
					continue
				}
				break
			}
			toks = append(toks, exprTok{etIdent, src[i:j]})
			i = j
			continue
		}
		return nil, fmt.Errorf("expression %q: unexpected character %q", src, string(r))
	}
	toks = append(toks, exprTok{etEOF, ""})
	return toks, nil
}

// --- expression parser (precedence climbing) ---

type exprParser struct {
	toks []exprTok
	pos  int
	src  string
}

func (p *exprParser) cur() exprTok { return p.toks[p.pos] }
func (p *exprParser) advance()     { p.pos++ }
func (p *exprParser) isOp(s string) bool {
	return p.cur().kind == etOp && p.cur().text == s
}

// binaryPrec maps an infix operator to its binding power. Higher binds tighter.
var binaryPrec = map[string]int{
	"??": 1,
	"||": 2,
	"&&": 3,
	"==": 4, "!=": 4,
	"<": 5, "<=": 5, ">": 5, ">=": 5,
	"+": 6, "-": 6,
	"*": 7, "/": 7, "%": 7,
}

func (p *exprParser) parseExpr(minPrec int) (ExprNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.cur()
		if t.kind != etOp {
			break
		}
		prec, ok := binaryPrec[t.text]
		if !ok || prec < minPrec {
			break
		}
		op := t.text
		p.advance()
		// all our binary operators are left-associative
		right, err := p.parseExpr(prec + 1)
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: op, L: left, R: right}
	}
	return left, nil
}

func (p *exprParser) parseUnary() (ExprNode, error) {
	if p.isOp("!") || p.isOp("-") {
		op := p.cur().text
		p.advance()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: op, X: x}, nil
	}
	return p.parsePostfix()
}

func (p *exprParser) parsePostfix() (ExprNode, error) {
	node, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.isOp("[") {
		p.advance()
		idx, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if !p.isOp("]") {
			return nil, fmt.Errorf("expression %q: expected ']' to close an index", p.src)
		}
		p.advance()
		node = &IndexExpr{X: node, Index: idx}
	}
	return node, nil
}

func (p *exprParser) parsePrimary() (ExprNode, error) {
	t := p.cur()
	switch t.kind {
	case etNumber:
		p.advance()
		if strings.Contains(t.text, ".") {
			f, err := strconv.ParseFloat(t.text, 64)
			if err != nil {
				return nil, fmt.Errorf("expression %q: bad number %q", p.src, t.text)
			}
			return &LitExpr{Kind: "float", Float: f}, nil
		}
		iv, err := strconv.ParseInt(t.text, 10, 64)
		if err != nil {
			// too large for int64 - fall back to float
			f, ferr := strconv.ParseFloat(t.text, 64)
			if ferr != nil {
				return nil, fmt.Errorf("expression %q: bad number %q", p.src, t.text)
			}
			return &LitExpr{Kind: "float", Float: f}, nil
		}
		return &LitExpr{Kind: "int", Int: iv}, nil
	case etString:
		p.advance()
		return buildStringLiteral(t.text)
	case etPath:
		p.advance()
		return &PathExpr{Path: t.text}, nil
	case etIdent:
		p.advance()
		switch t.text {
		case "true":
			return &LitExpr{Kind: "bool", Bool: true}, nil
		case "false":
			return &LitExpr{Kind: "bool", Bool: false}, nil
		case "null", "nil":
			return &LitExpr{Kind: "null"}, nil
		}
		// function call?
		if p.isOp("(") {
			if !ExprBuiltins[t.text] {
				return nil, fmt.Errorf("expression %q: %q is not a permitted builtin (allowed: see ExprBuiltins)", p.src, t.text)
			}
			p.advance()
			var args []ExprNode
			if !p.isOp(")") {
				for {
					a, err := p.parseExpr(0)
					if err != nil {
						return nil, err
					}
					args = append(args, a)
					if p.isOp(",") {
						p.advance()
						continue
					}
					break
				}
			}
			if !p.isOp(")") {
				return nil, fmt.Errorf("expression %q: expected ')' to close call to %s", p.src, t.text)
			}
			p.advance()
			return &CallExpr{Name: t.text, Args: args}, nil
		}
		return &PathExpr{Path: t.text}, nil
	case etOp:
		switch t.text {
		case "(":
			p.advance()
			inner, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			if !p.isOp(")") {
				return nil, fmt.Errorf("expression %q: expected ')'", p.src)
			}
			p.advance()
			return inner, nil
		case "[":
			p.advance()
			var elems []ExprNode
			if !p.isOp("]") {
				for {
					e, err := p.parseExpr(0)
					if err != nil {
						return nil, err
					}
					elems = append(elems, e)
					if p.isOp(",") {
						p.advance()
						continue
					}
					break
				}
			}
			if !p.isOp("]") {
				return nil, fmt.Errorf("expression %q: expected ']' to close a list", p.src)
			}
			p.advance()
			return &ListExpr{Elems: elems}, nil
		case "{":
			p.advance()
			m := &MapExpr{}
			if !p.isOp("}") {
				for {
					key := p.cur()
					if key.kind != etIdent && key.kind != etString {
						return nil, fmt.Errorf("expression %q: map key must be an identifier or string", p.src)
					}
					p.advance()
					keyStr := key.text
					if key.kind == etString {
						lit, err := buildStringLiteral(key.text)
						if err != nil {
							return nil, err
						}
						if l, ok := lit.(*LitExpr); ok {
							keyStr = l.Str
						}
					}
					if !p.isOp(":") {
						return nil, fmt.Errorf("expression %q: expected ':' after map key %q", p.src, keyStr)
					}
					p.advance()
					val, err := p.parseExpr(0)
					if err != nil {
						return nil, err
					}
					m.Keys = append(m.Keys, keyStr)
					m.Vals = append(m.Vals, val)
					if p.isOp(",") {
						p.advance()
						continue
					}
					break
				}
			}
			if !p.isOp("}") {
				return nil, fmt.Errorf("expression %q: expected '}' to close a map", p.src)
			}
			p.advance()
			return m, nil
		}
	}
	return nil, fmt.Errorf("expression %q: unexpected %q", p.src, t.text)
}

// buildStringLiteral turns raw string content (quotes already stripped) into a
// LitExpr, or an InterpExpr when it contains `${...}` holes. Backslash escapes
// (\n \t \" \\ \$) are processed here.
func buildStringLiteral(raw string) (ExprNode, error) {
	var parts []ExprNode
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			parts = append(parts, &LitExpr{Kind: "string", Str: lit.String()})
			lit.Reset()
		}
	}
	i := 0
	n := len(raw)
	for i < n {
		c := raw[i]
		if c == '\\' && i+1 < n {
			switch raw[i+1] {
			case 'n':
				lit.WriteByte('\n')
			case 't':
				lit.WriteByte('\t')
			case 'r':
				lit.WriteByte('\r')
			case '"':
				lit.WriteByte('"')
			case '\'':
				lit.WriteByte('\'')
			case '\\':
				lit.WriteByte('\\')
			case '$':
				lit.WriteByte('$')
			default:
				lit.WriteByte(raw[i+1])
			}
			i += 2
			continue
		}
		if c == '$' && i+1 < n && raw[i+1] == '{' {
			// find matching '}'
			depth := 0
			j := i + 2
			for j < n {
				if raw[j] == '{' {
					depth++
				} else if raw[j] == '}' {
					if depth == 0 {
						break
					}
					depth--
				}
				j++
			}
			if j >= n {
				return nil, fmt.Errorf("string %q: unterminated ${ ... } interpolation", raw)
			}
			inner := raw[i+2 : j]
			sub, err := ParseExpr(inner)
			if err != nil {
				return nil, err
			}
			flush()
			parts = append(parts, sub)
			i = j + 1
			continue
		}
		lit.WriteByte(c)
		i++
	}
	flush()
	if len(parts) == 0 {
		return &LitExpr{Kind: "string", Str: ""}, nil
	}
	if len(parts) == 1 {
		if l, ok := parts[0].(*LitExpr); ok {
			return l, nil
		}
	}
	return &InterpExpr{Parts: parts}, nil
}
