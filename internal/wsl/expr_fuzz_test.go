package wsl

import "testing"

// FuzzParseExpr checks that the expression parser never panics, regardless of
// input. A parse error is fine; a panic is a bug.
func FuzzParseExpr(f *testing.F) {
	seeds := []string{
		"", "1", "a.b", `"s"`, "a == b && c > 0",
		"1 + 2 * 3", "[1,2,3]", "{a:1}", "len(x)", `"x${y}z"`,
		"<<a.b>>", "!(!a)", "a ?? b ?? c", "((((", "]]]]", "${",
		"1 1 1", "* * *", `"\`, "a[b][c]", "0.0.0.0",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		_, _ = ParseExpr(src) // must not panic
	})
}
