package wsl

import (
	"testing"
)

func TestLexer_Tokens_Basics(t *testing.T) {
	src := `// comment line
module demo.mod
# hash comment
workflow wf {
  start: s1
  state s1 {
    action http.get(a, "str", 123) as res
    on success -> s2
    on (res >= 100) -> end_ok
  }
  state end_ok { end ok code="OK" }
}`

	lx := NewLexer(src)
	var kinds []TokenKind
	var lexemes []string
	for {
		tok := lx.Next()
		kinds = append(kinds, tok.Kind)
		lexemes = append(lexemes, tok.Lexeme)
		if tok.Kind == TokEOF {
			break
		}
	}

	// Basic sanity: should include key keywords and punctuators, comments skipped

	// Since the exact tokenization of complex expr is implementation-specific here, we only check the subset and order of major markers.
	// Validate selected checkpoints
	checkpoints := []struct {
		idx  int
		kind TokenKind
	}{
		{0, TokModule},
		{2, TokWorkflow},
		{5, TokStart},
		{8, TokState},
	}
	for _, c := range checkpoints {
		if c.idx >= len(kinds) {
			t.Fatalf("token stream too short, want index %d", c.idx)
		}
		if kinds[c.idx] != c.kind {
			t.Fatalf("unexpected token at %d: got %s, want %s", c.idx, kinds[c.idx], c.kind)
		}
	}
}

func TestLexer_StringEscapes(t *testing.T) {
	src := `module m
workflow w { start: s state s { end ok msg="a\\n\\t\"b\"" } }`
	lx := NewLexer(src)
	foundString := false
	for {
		tok := lx.Next()
		if tok.Kind == TokString {
			foundString = true
			expected := `a\\n\\t\"b\"`
			if tok.Lexeme != expected { // raw token content without quotes
				t.Fatalf("unexpected string lexeme: got %q, want %q", tok.Lexeme, expected)
			}
		}
		if tok.Kind == TokEOF {
			break
		}
	}
	if !foundString {
		t.Fatal("no string token found")
	}
}

func TestLexer_ArrayBrackets(t *testing.T) {
	src := `[1, 2, [3, 4]]`
	lx := NewLexer(src)
	var tokens []Token
	for {
		tok := lx.Next()
		tokens = append(tokens, tok)
		if tok.Kind == TokEOF {
			break
		}
	}

	// Expected sequence: [ 1 , 2 , [ 3 , 4 ] ] EOF
	expected := []TokenKind{
		TokLBrack, TokNumber, TokComma, TokNumber, TokComma,
		TokLBrack, TokNumber, TokComma, TokNumber, TokRBrack, TokRBrack, TokEOF,
	}

	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}

	for i, exp := range expected {
		if tokens[i].Kind != exp {
			t.Errorf("token %d: expected %s, got %s", i, exp, tokens[i].Kind)
		}
	}
}

func TestLexer_ComparisonAndLogicalOperators(t *testing.T) {
	// Each input must lex to exactly the expected token kinds (excluding EOF).
	cases := []struct {
		src  string
		want []TokenKind
	}{
		{`==`, []TokenKind{TokEqEq}},
		{`!=`, []TokenKind{TokNeq}},
		{`>=`, []TokenKind{TokGte}},
		{`<=`, []TokenKind{TokLte}},
		{`&&`, []TokenKind{TokAndAnd}},
		{`||`, []TokenKind{TokOrOr}},
		// single-char operators still lex as before
		{`=`, []TokenKind{TokEqual}},
		{`>`, []TokenKind{TokGt}},
		{`<`, []TokenKind{TokLt}},
		{`!`, []TokenKind{TokBang}},
		// arrows must still win over <= / >= adjacency
		{`->`, []TokenKind{TokArrow}},
		{`<-`, []TokenKind{TokLeftArrow}},
		// realistic expression: a == b && c >= 1
		{`a == b && c >= 1`, []TokenKind{TokIdent, TokEqEq, TokIdent, TokAndAnd, TokIdent, TokGte, TokNumber}},
		// not-equal against identifier, no spaces
		{`x!=y`, []TokenKind{TokIdent, TokNeq, TokIdent}},
		// attribute assignment still uses single '='
		{`code = "OK"`, []TokenKind{TokIdent, TokEqual, TokString}},
		// bang followed by identifier (logical not) stays TokBang
		{`!ready`, []TokenKind{TokBang, TokIdent}},
	}

	for _, c := range cases {
		lx := NewLexer(c.src)
		var got []TokenKind
		for {
			tok := lx.Next()
			if tok.Kind == TokEOF {
				break
			}
			if tok.Kind == TokIllegal {
				t.Fatalf("%q: illegal token %q", c.src, tok.Lexeme)
			}
			got = append(got, tok.Kind)
		}
		if len(got) != len(c.want) {
			t.Fatalf("%q: got %d tokens %v, want %d %v", c.src, len(got), got, len(c.want), c.want)
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("%q token %d: got %s, want %s", c.src, i, got[i], c.want[i])
			}
		}
	}
}

func TestLexer_SpecialDollarVariables(t *testing.T) {
	src := `module m
workflow w { start: s
state s {
  action svc.Run(v1: $@, v2: $?, v3: $^, v4: $lastResponse, v5: $var!@#)
  end fail
}}`
	lx := NewLexer(src)
	seen := map[string]bool{}
	for {
		tok := lx.Next()
		if tok.Kind == TokIdent {
			switch tok.Lexeme {
			case "$@", "$?", "$^", "$lastResponse", "$var!@#":
				seen[tok.Lexeme] = true
			}
		}
		if tok.Kind == TokEOF {
			break
		}
		if tok.Kind == TokIllegal {
			t.Fatalf("unexpected illegal token: %q", tok.Lexeme)
		}
	}
	for _, k := range []string{"$@", "$?", "$^", "$lastResponse", "$var!@#"} {
		if !seen[k] {
			t.Fatalf("expected to lex identifier %q", k)
		}
	}
}
