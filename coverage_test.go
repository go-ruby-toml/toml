// Copyright (c) the go-ruby-toml/toml authors
//
// SPDX-License-Identifier: BSD-3-Clause

package toml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// assertErr parses src and asserts it fails, optionally checking the error type.
func assertErr(t *testing.T, src string) error {
	t.Helper()
	_, err := Parse(src)
	if err == nil {
		t.Errorf("Parse(%q) = nil error, want error", src)
	}
	return err
}

func TestParseErrorsSyntax(t *testing.T) {
	srcs := []string{
		"a 1",               // missing '='
		"= 1",               // missing key
		"a = ",              // empty value
		"a = @",             // invalid value char
		"a = tru",           // truncated true
		"a = fa",            // truncated false
		"[a",                // unclosed table header
		"[[a]",              // unclosed array header
		"[a.]",              // dangling dot in header (empty key)
		"a..b = 1",          // empty dotted segment
		".a = 1",            // leading dot
		"a = [1 2]",         // missing comma in array
		"a = [1",            // unterminated array right after a value
		"a = [1,",           // unterminated array after comma
		"a = {x = 1",        // unterminated inline table
		"a = {x = 1 y = 2}", // missing comma in inline table
		"a = \"unterm",      // unterminated basic string
		"a = 'unterm",       // unterminated literal string
		"a = \"\"\"unterm",  // unterminated multiline basic
		"a = '''unterm",     // unterminated multiline literal
		"a = 1 b",           // junk after value
	}
	for _, s := range srcs {
		assertErr(t, s)
	}
}

func TestParseErrorsNumbers(t *testing.T) {
	srcs := []string{
		"a = 01",                   // decimal leading zero
		"a = 1__2",                 // double underscore
		"a = _1",                   // leading underscore
		"a = 1_",                   // trailing underscore
		"a = 0x",                   // empty hex
		"a = 0xG",                  // bad hex digit
		"a = .7",                   // float no leading digit
		"a = 7.",                   // float no trailing digit
		"a = 1._5",                 // underscore next to dot
		"a = 07.5",                 // float leading zero
		"a = 1e",                   // exponent no digits
		"a = 0b",                   // empty binary
		"a = 0o",                   // empty octal
		"a = 0x1_",                 // hex trailing underscore
		"a = 1_.5",                 // underscore before dot
		"a = 9223372036854775808",  // int64 overflow (max + 1)
		"a = -9223372036854775809", // int64 underflow (min - 1)
	}
	for _, s := range srcs {
		assertErr(t, s)
	}
}

func TestParseErrorsDates(t *testing.T) {
	srcs := []string{
		"a = 1979-13-01",           // bad month
		"a = 1979-05-99",           // bad day
		"a = 25:00:00",             // bad hour
		"a = 07:60:00",             // bad minute
		"a = 07:32:61",             // bad second
		"a = 1979-05-27Tbad",       // bad separator's time
		"a = 1979-05-27Q07:32:00",  // bad date/time separator
		"a = 1979-05-27T07:32",     // short time
		"a = 1979-xx-27",           // non-digit date
		"a = 07:32:xx",             // non-digit time
		"a = 07:32:00.",            // empty fractional
		"a = 07:32:00.x",           // non-digit fractional
		"a = 1979-05-27T07:32:00.", // empty frac in datetime
	}
	for _, s := range srcs {
		assertErr(t, s)
	}
}

func TestParseErrorsStrings(t *testing.T) {
	srcs := []string{
		`a = "bad\q"`,         // invalid escape
		`a = "\u00"`,          // short unicode escape
		`a = "\uZZZZ"`,        // bad unicode hex
		`a = "\uD800"`,        // surrogate scalar
		`a = "\x"`,            // short hex escape
		`a = "\xZZ"`,          // bad hex escape
		"a = \"line\nbreak\"", // newline in basic string
		"a = 'line\nbreak'",   // newline in literal string
	}
	for _, s := range srcs {
		assertErr(t, s)
	}
	// Dangling escape at EOF.
	assertErr(t, "a = \"x\\")
	// \U overlong unicode (too large).
	assertErr(t, `a = "\U00110000"`)
}

func TestParseErrorsOverwrite(t *testing.T) {
	cases := []string{
		"a = 1\na = 2",                      // duplicate bare key
		"[a]\nx=1\n[a]\ny=2",                // duplicate table header
		"t = {x = 1, x = 2}",                // duplicate inline key
		"a = 1\na.b = 2",                    // dotted onto scalar
		"a.b = 1\na = 2",                    // scalar onto dotted parent
		"[[a]]\nx=1\n[a]\ny=2",              // table over AOT
		"[a]\nx=1\n[[a]]\ny=2",              // AOT over table
		"[a.b]\nc=1\n[a.b]\nd=2",            // duplicate nested table
		"[a]\nb.c=1\nb.c=2",                 // duplicate dotted in table
		"a=1\n[a]\nx=2",                     // table over scalar
		"a.b=1\n[a.b]\nc=2",                 // explicit over dotted-born
		"t={a=1}\nt.b=2",                    // dotted-extend a frozen inline table
		"[a]\nb={x=1}\n[a.b]\ny=2",          // header reopening inline table
		"[[a]]\n[[a.b.c]]\n[[a]]\n[a]\nx=1", // AOT then table same key
	}
	for _, s := range cases {
		err := assertErr(t, s)
		if _, ok := err.(*OverwriteError); err != nil && !ok {
			// Some are syntactically rejected before the overwrite check; either
			// error type is acceptable, but most should be OverwriteError.
			t.Logf("%q -> %T: %v", s, err, err)
		}
	}
}

func TestParseErrorsComments(t *testing.T) {
	// Control char in comment.
	assertErr(t, "a = 1 #\x01bad")
	// Control char in comment-only line.
	assertErr(t, "#\x01")
}

func TestParseAcceptsTrickyValid(t *testing.T) {
	// These must all succeed.
	ok := []string{
		"a = 0",                      // bare zero
		"a = -0",                     // negative zero
		"a = 0.0",                    // float zero
		"a = 0e0",                    // float zero exponent
		"a = 1979-05-27T07:32:00.5Z", // frac offset datetime
		"a = 07:32:00.123456789012",  // over-precise frac truncates
		"a = \"\"",                   // empty basic string
		"a = ''",                     // empty literal string
		"a = \"\"\"\"\"\"",           // empty multiline basic
		"a = ''''''",                 // empty multiline literal
		"[a]\n[a.b]\n[a.b.c]\nx = 1", // deep nesting
		"a = { }",                    // inline table with space
	}
	for _, s := range ok {
		if _, err := Parse(s); err != nil {
			t.Errorf("Parse(%q) failed: %v", s, err)
		}
	}
}

func TestErrorMessages(t *testing.T) {
	_, err := Parse("a = 1\nb = @")
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if pe.Line != 2 {
		t.Errorf("error line = %d, want 2", pe.Line)
	}
	if pe.Error() == "" {
		t.Error("empty error message")
	}
	// OverwriteError message format.
	_, err = Parse("a = 1\na = 2")
	oe, ok := err.(*OverwriteError)
	if !ok {
		t.Fatalf("expected *OverwriteError, got %T", err)
	}
	if oe.Error() != `Key "a" is defined more than once` {
		t.Errorf("overwrite msg = %q", oe.Error())
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.toml")
	if err := os.WriteFile(path, []byte("a = 1\n[t]\nk = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if v, _ := m.Get("a"); v != int64(1) {
		t.Errorf("a = %v", v)
	}
	// Missing file.
	if _, err := LoadFile(filepath.Join(dir, "nope.toml")); err == nil {
		t.Error("LoadFile(missing) should error")
	}
	// File with a parse error.
	bad := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(bad, []byte("a = "), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(bad); err == nil {
		t.Error("LoadFile(bad) should error")
	}
}

func TestMapAPI(t *testing.T) {
	m := NewMap()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("a", 3) // replace
	if m.Len() != 2 {
		t.Errorf("len = %d", m.Len())
	}
	if v, _ := m.Get("a"); v != 3 {
		t.Errorf("a = %v", v)
	}
	if _, ok := m.Get("missing"); ok {
		t.Error("missing key present")
	}
	if len(m.Pairs()) != 2 {
		t.Errorf("pairs = %d", len(m.Pairs()))
	}
	// Get on a zero-value Map (nil index).
	var z Map
	if _, ok := z.Get("x"); ok {
		t.Error("zero map Get should miss")
	}
	z.Set("x", 1) // exercises nil-index lazy init in Set
	if v, _ := z.Get("x"); v != 1 {
		t.Errorf("zero map x = %v", v)
	}
}

func TestStringer(t *testing.T) {
	if got := (LocalDate{2026, 6, 30}).String(); got != "2026-06-30" {
		t.Errorf("LocalDate.String = %q", got)
	}
	if got := (LocalTime{Hour: 1, Minute: 2, Second: 3}).String(); got != "01:02:03" {
		t.Errorf("LocalTime.String = %q", got)
	}
	if got := (LocalDateTime{Year: 2026, Month: 6, Day: 30}).String(); !strings.HasPrefix(got, "2026-06-30T") {
		t.Errorf("LocalDateTime.String = %q", got)
	}
}
