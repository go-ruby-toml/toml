// Copyright (c) the go-ruby-toml/toml authors
//
// SPDX-License-Identifier: BSD-3-Clause

package toml

import "testing"

// TestUnderscoresInBases exercises the octal/binary digit validators.
func TestUnderscoresInBases(t *testing.T) {
	m := mustParse(t, "o = 0o7_5_5\nb = 0b1_0_1_0\nh = 0xA_B")
	if v, _ := m.Get("o"); v != int64(0o755) {
		t.Errorf("oct underscores = %v", v)
	}
	if v, _ := m.Get("b"); v != int64(0b1010) {
		t.Errorf("bin underscores = %v", v)
	}
	if v, _ := m.Get("h"); v != int64(0xAB) {
		t.Errorf("hex underscores = %v", v)
	}
	// Bad underscore placement in each base.
	for _, s := range []string{"a = 0o7__5", "a = 0b1__0", "a = 0xA__B", "a = 0o_7", "a = 0b1_", "a = 0xA_"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) should fail", s)
		}
	}
}

// TestCRLFBetweenLines covers the CRLF cases in the top-level and array skippers.
func TestCRLFBetweenLines(t *testing.T) {
	m := mustParse(t, "\r\na = 1\r\n\r\nb = 2\r\n")
	if v, _ := m.Get("b"); v != int64(2) {
		t.Errorf("crlf doc b = %v", v)
	}
	// CRLF and comment inside an array.
	m = mustParse(t, "a = [\r\n  1,\r\n  # note\r\n  2\r\n]")
	if v, _ := m.Get("a"); len(v.([]any)) != 2 {
		t.Errorf("crlf array = %#v", v)
	}
}

// TestCommentErrorInSkip covers a control char in a leading (pre-line) comment.
func TestCommentErrorInSkip(t *testing.T) {
	if _, err := Parse("\n#\x01bad\na = 1"); err == nil {
		t.Error("control char in leading comment should fail")
	}
	// Control char in an array comment.
	if _, err := Parse("a = [1, # \x01\n2]"); err == nil {
		t.Error("control char in array comment should fail")
	}
}

// TestMultilineEscapeAndCRLF covers escape handling and CRLF inside a multiline
// basic string, plus the opening-CRLF trim.
func TestMultilineEscapeAndCRLF(t *testing.T) {
	m := mustParse(t, "s = \"\"\"a\\tb\"\"\"")
	if v, _ := m.Get("s"); v != "a\tb" {
		t.Errorf("ml escape = %q", v)
	}
	// Bad escape inside multiline.
	if _, err := Parse("s = \"\"\"a\\qb\"\"\""); err == nil {
		t.Error("bad ml escape should fail")
	}
	// Opening CRLF is trimmed.
	m = mustParse(t, "s = \"\"\"\r\nhello\"\"\"")
	if v, _ := m.Get("s"); v != "hello" {
		t.Errorf("ml opening crlf = %q", v)
	}
	// A backslash NOT at line end is a normal escape (covers isLineEndingBackslash
	// false branch).
	m = mustParse(t, "s = \"\"\"x\\ny\"\"\"")
	if v, _ := m.Get("s"); v != "x\ny" {
		t.Errorf("ml mid escape = %q", v)
	}
}

// TestHeaderIntoAOT covers a [table] header descending into the last element of an
// existing array-of-tables, and the redefinition errors around AOT/table mixing.
func TestHeaderIntoAOT(t *testing.T) {
	// [a.b] where a is an AOT: descend into the last element of a.
	m := mustParse(t, "[[a]]\nx = 1\n[a.b]\ny = 2")
	arr := mustGet(t, m, "a").([]any)
	last := arr[len(arr)-1].(*Map)
	bt, _ := last.Get("b")
	if v, _ := bt.(*Map).Get("y"); v != int64(2) {
		t.Errorf("header into AOT = %v", v)
	}
	// Dotted key path through an AOT's last element via header.
	m = mustParse(t, "[[a]]\n[a.b.c]\nz = 3")
	arr = mustGet(t, m, "a").([]any)
}

// TestArrayTableIntermediates covers openArrayTable's intermediate descent cases.
func TestArrayTableIntermediates(t *testing.T) {
	// Nested AOT under an existing AOT: [[a]] then [[a.b]].
	m := mustParse(t, "[[a]]\n[[a.b]]\nx = 1\n[[a.b]]\ny = 2")
	a := mustGet(t, m, "a").([]any)
	b, _ := a[0].(*Map).Get("b")
	if len(b.([]any)) != 2 {
		t.Errorf("nested AOT len = %d", len(b.([]any)))
	}
	// AOT intermediate through a plain table: [a]\n[[a.b]].
	m = mustParse(t, "[a]\nk = 1\n[[a.b]]\nx = 1")
	if v := get(t, m, "a", "k"); v != int64(1) {
		t.Errorf("a.k = %v", v)
	}
	// AOT whose intermediate is a scalar → error.
	if _, err := Parse("a = 1\n[[a.b]]\nx = 2"); err == nil {
		t.Error("AOT through scalar should fail")
	}
	// AOT intermediate through an inline (frozen) table → error.
	if _, err := Parse("a = {b = 1}\n[[a.c]]\nx = 2"); err == nil {
		t.Error("AOT through inline table should fail")
	}
	// AOT final key over an existing non-array → error.
	if _, err := Parse("[a]\nb = 1\n[[a.b]]\nx = 2"); err == nil {
		t.Error("AOT over scalar key should fail")
	}
	// AOT intermediate stepping into an empty inline array → error.
	if _, err := Parse("a = []\n[[a.b]]\nx = 1"); err == nil {
		t.Error("AOT through empty array should fail")
	}
	// AOT intermediate stepping into an array of non-tables → error.
	if _, err := Parse("a = [1, 2]\n[[a.b]]\nx = 1"); err == nil {
		t.Error("AOT through scalar array should fail")
	}
}

// TestHeaderThroughArrays covers openTable descending through inline arrays at a
// key: empty array and array-of-non-tables both reject; a header onto an AOT's
// final key rejects.
func TestHeaderThroughArrays(t *testing.T) {
	if _, err := Parse("a = []\n[a.b]\nx = 1"); err == nil {
		t.Error("table header through empty array should fail")
	}
	if _, err := Parse("a = [1, 2]\n[a.b]\nx = 1"); err == nil {
		t.Error("table header through scalar array should fail")
	}
	// A [table] header whose final key is an existing AOT → error.
	if _, err := Parse("[[a]]\nx = 1\n[a]\ny = 2"); err == nil {
		t.Error("table header onto AOT key should fail")
	}
	// A header onto an existing scalar key → error (default case).
	if _, err := Parse("a = 1\n[a]\nx = 2"); err == nil {
		t.Error("table header onto scalar should fail")
	}
}

// TestDottedExtendExplicit covers assign's explicit-table interaction: a dotted
// key inside an explicit [table] header may extend that table.
func TestDottedExtendExplicit(t *testing.T) {
	m := mustParse(t, "[a]\nb.c = 1\nb.d = 2")
	if v := get(t, m, "a", "b", "c"); v != int64(1) {
		t.Errorf("a.b.c = %v", v)
	}
	if v := get(t, m, "a", "b", "d"); v != int64(2) {
		t.Errorf("a.b.d = %v", v)
	}
}

// TestDatetimeEdgeTokens covers the empty-rest datetime branch and a few more.
func TestDatetimeEdgeTokens(t *testing.T) {
	// `T` with nothing after it → malformed time.
	if _, err := Parse("a = 1979-05-27T"); err == nil {
		t.Error("trailing T should fail")
	}
	// Bad offset format falls through to local datetime parse (which then fails on
	// the bogus time), covering splitOffset's non-offset tail return.
	if _, err := Parse("a = 1979-05-27T07:32:00+5"); err == nil {
		t.Error("malformed offset should fail")
	}
	// Value token starting with a delimiter yields the empty-token error.
	if _, err := Parse("a = ]"); err == nil {
		t.Error("delimiter value should fail")
	}
	if _, err := Parse("a = #c"); err == nil {
		t.Error("comment-as-value should fail")
	}
}

// TestEmitStringControls covers \r \b \f and the \uXXXX control fallback.
func TestEmitStringControls(t *testing.T) {
	m := NewMap()
	m.Set("s", "x\ry\bz\f\x01\x7f")
	out := mustDump(t, m)
	for _, w := range []string{`\r`, `\b`, `\f`, `\u0001`, `\u007F`} {
		if !contains(out, w) {
			t.Errorf("emit string missing %q in %q", w, out)
		}
	}
}

// contains is a tiny strings.Contains wrapper to keep imports minimal.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestArrayElementError covers a parse error on an array element.
func TestArrayElementError(t *testing.T) {
	for _, s := range []string{"a = [@]", "a = [1, .7]", "a = [\"x]"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) should fail", s)
		}
	}
}

// TestFloatParseFailure covers a token that passes the cheap validators but Go's
// ParseFloat rejects (a second decimal point).
func TestFloatParseFailure(t *testing.T) {
	if _, err := Parse("a = 1.2.3"); err == nil {
		t.Error("1.2.3 should fail")
	}
}

// TestLineEndingBackslashWithSpace covers the whitespace-before-newline loop in
// the multiline line-continuation detector.
func TestLineEndingBackslashWithSpace(t *testing.T) {
	m := mustParse(t, "s = \"\"\"the quick \\   \n  brown fox\"\"\"")
	if v, _ := m.Get("s"); v != "the quick brown fox" {
		t.Errorf("backslash+space trim = %q", v)
	}
	// Backslash + spaces + CRLF.
	m = mustParse(t, "s = \"\"\"a \\ \t\r\n  b\"\"\"")
	if v, _ := m.Get("s"); v != "a b" {
		t.Errorf("backslash+space+crlf = %q", v)
	}
}

// TestDumpErrorType exercises dumpError.Error directly.
func TestDumpErrorType(t *testing.T) {
	e := &dumpError{"boom"}
	if e.Error() != "boom" {
		t.Errorf("dumpError = %q", e.Error())
	}
}
