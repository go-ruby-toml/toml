// Copyright (c) the go-ruby-toml/toml authors
//
// SPDX-License-Identifier: BSD-3-Clause

package toml

import (
	"math"
	"reflect"
	"testing"
	"time"
)

// get walks a dotted path of string keys into a parsed document and returns the
// value, failing the test if any component is missing or not a table.
func get(t *testing.T, m *Map, path ...string) Value {
	t.Helper()
	var cur Value = m
	for _, k := range path {
		tbl, ok := cur.(*Map)
		if !ok {
			t.Fatalf("path %v: %q is not a table (%T)", path, k, cur)
		}
		v, ok := tbl.Get(k)
		if !ok {
			t.Fatalf("path %v: key %q missing", path, k)
		}
		cur = v
	}
	return cur
}

// mustParse parses src or fails the test.
func mustParse(t *testing.T, src string) *Map {
	t.Helper()
	m, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return m
}

func TestScalars(t *testing.T) {
	m := mustParse(t, `
str = "hello"
lit = 'C:\path'
i = 42
neg = -17
pos = +99
hex = 0xDEAD_BEEF
oct = 0o755
bin = 0b1010
big = 1_000_000
zero = 0
f = 3.14
e = 1e10
nf = -2.5
fu = 9_224_617.445_991
t = true
fa = false
`)
	checks := map[string]Value{
		"str":  "hello",
		"lit":  `C:\path`,
		"i":    int64(42),
		"neg":  int64(-17),
		"pos":  int64(99),
		"hex":  int64(0xDEADBEEF),
		"oct":  int64(0o755),
		"bin":  int64(0b1010),
		"big":  int64(1000000),
		"zero": int64(0),
		"f":    3.14,
		"e":    1e10,
		"nf":   -2.5,
		"fu":   9224617.445991,
		"t":    true,
		"fa":   false,
	}
	for k, want := range checks {
		got, _ := m.Get(k)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", k, got, want)
		}
	}
}

func TestSpecialFloats(t *testing.T) {
	m := mustParse(t, "a = inf\nb = +inf\nc = -inf\nd = nan\ne = +nan\nf = -nan")
	if v, _ := m.Get("a"); !math.IsInf(v.(float64), 1) {
		t.Errorf("inf = %v", v)
	}
	if v, _ := m.Get("b"); !math.IsInf(v.(float64), 1) {
		t.Errorf("+inf = %v", v)
	}
	if v, _ := m.Get("c"); !math.IsInf(v.(float64), -1) {
		t.Errorf("-inf = %v", v)
	}
	for _, k := range []string{"d", "e", "f"} {
		if v, _ := m.Get(k); !math.IsNaN(v.(float64)) {
			t.Errorf("%s nan = %v", k, v)
		}
	}
}

func TestStringEscapes(t *testing.T) {
	m := mustParse(t, `s = "é\t\n\r\"\\\b\f\U0001F600"`)
	want := "é\t\n\r\"\\\b\f\U0001F600"
	if v, _ := m.Get("s"); v != want {
		t.Errorf("escapes = %q, want %q", v, want)
	}
	// toml-rb's SPECIAL_CHARS map has no \e or \x entry, so its
	// transform_escaped_chars raises "Escape sequence reserved" for both;
	// this package rejects them identically (also matching strict v1.0.0,
	// which lists neither escape).
	for _, bad := range []string{`s = "\e"`, `s = "\x41"`, `s = "\xAg"`} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("expected rejection of reserved escape in %q", bad)
		}
	}
}

func TestMultilineBasic(t *testing.T) {
	m := mustParse(t, "s = \"\"\"\nroses\nviolets\"\"\"")
	if v, _ := m.Get("s"); v != "roses\nviolets" {
		t.Errorf("ml basic = %q", v)
	}
	// Line-ending backslash trims following whitespace/newlines.
	m = mustParse(t, "s = \"\"\"\\\n  the quick \\\n  brown\"\"\"")
	if v, _ := m.Get("s"); v != "the quick brown" {
		t.Errorf("ml backslash trim = %q", v)
	}
	// Trailing literal quotes (4 and 5 quote runs).
	m = mustParse(t, "s = \"\"\"foo\"\"\"\"")
	if v, _ := m.Get("s"); v != `foo"` {
		t.Errorf("ml one trailing quote = %q", v)
	}
	m = mustParse(t, "s = \"\"\"foo\"\"\"\"\"")
	if v, _ := m.Get("s"); v != `foo""` {
		t.Errorf("ml two trailing quotes = %q", v)
	}
	// Empty multiline strings of various quote counts (toml-rb-faithful).
	m = mustParse(t, "s = \"\"\"\"\"\"")
	if v, _ := m.Get("s"); v != "" {
		t.Errorf("ml empty = %q", v)
	}
	m = mustParse(t, "s = \"\"\"\"\"\"\"\"\"")
	if v, _ := m.Get("s"); v != `"""` {
		t.Errorf("ml nine quotes = %q", v)
	}
	// CRLF preserved inside content.
	m = mustParse(t, "s = \"\"\"a\r\nb\"\"\"")
	if v, _ := m.Get("s"); v != "a\r\nb" {
		t.Errorf("ml crlf = %q", v)
	}
}

func TestMultilineLiteral(t *testing.T) {
	m := mustParse(t, "s = '''\nline1\nline2'''")
	if v, _ := m.Get("s"); v != "line1\nline2" {
		t.Errorf("ml literal = %q", v)
	}
	// No escape processing.
	m = mustParse(t, "s = '''a\\nb'''")
	if v, _ := m.Get("s"); v != `a\nb` {
		t.Errorf("ml literal noescape = %q", v)
	}
}

func TestKeys(t *testing.T) {
	m := mustParse(t, `
bare-key_1 = 1
"quoted.key" = 2
'literal key' = 3
123 = 4
`)
	if v, _ := m.Get("bare-key_1"); v != int64(1) {
		t.Errorf("bare = %v", v)
	}
	if v, _ := m.Get("quoted.key"); v != int64(2) {
		t.Errorf("quoted = %v", v)
	}
	if v, _ := m.Get("literal key"); v != int64(3) {
		t.Errorf("literal = %v", v)
	}
	if v, _ := m.Get("123"); v != int64(4) {
		t.Errorf("intkey = %v", v)
	}
}

func TestDottedKeys(t *testing.T) {
	m := mustParse(t, "a.b.c = 1\na.b.d = 2\na.e = 3")
	if v := get(t, m, "a", "b", "c"); v != int64(1) {
		t.Errorf("a.b.c = %v", v)
	}
	if v := get(t, m, "a", "b", "d"); v != int64(2) {
		t.Errorf("a.b.d = %v", v)
	}
	if v := get(t, m, "a", "e"); v != int64(3) {
		t.Errorf("a.e = %v", v)
	}
}

func TestArrays(t *testing.T) {
	m := mustParse(t, `
nums = [1, 2, 3]
mixed = [1, "two", 3.0, true]
nested = [[1, 2], [3, 4]]
trailing = [1, 2, ]
multiline = [
  1,
  2, # comment
  3,
]
empty = []
`)
	if v, _ := m.Get("nums"); !reflect.DeepEqual(v, []any{int64(1), int64(2), int64(3)}) {
		t.Errorf("nums = %#v", v)
	}
	if v, _ := m.Get("mixed"); !reflect.DeepEqual(v, []any{int64(1), "two", 3.0, true}) {
		t.Errorf("mixed = %#v", v)
	}
	if v, _ := m.Get("nested"); !reflect.DeepEqual(v, []any{[]any{int64(1), int64(2)}, []any{int64(3), int64(4)}}) {
		t.Errorf("nested = %#v", v)
	}
	if v, _ := m.Get("trailing"); !reflect.DeepEqual(v, []any{int64(1), int64(2)}) {
		t.Errorf("trailing = %#v", v)
	}
	if v, _ := m.Get("multiline"); !reflect.DeepEqual(v, []any{int64(1), int64(2), int64(3)}) {
		t.Errorf("multiline = %#v", v)
	}
	if v, _ := m.Get("empty"); !reflect.DeepEqual(v, []any{}) {
		t.Errorf("empty = %#v", v)
	}
}

func TestInlineTables(t *testing.T) {
	m := mustParse(t, "pt = { x = 1, y = 2 }\nempty = {}\ndotted = { a.b = 1 }\nnested = { p = { q = 1 } }")
	pt := get(t, m, "pt").(*Map)
	if v, _ := pt.Get("x"); v != int64(1) {
		t.Errorf("pt.x = %v", v)
	}
	if e := get(t, m, "empty").(*Map); e.Len() != 0 {
		t.Errorf("empty inline not empty")
	}
	if v := get(t, m, "dotted", "a", "b"); v != int64(1) {
		t.Errorf("dotted inline = %v", v)
	}
	if v := get(t, m, "nested", "p", "q"); v != int64(1) {
		t.Errorf("nested inline = %v", v)
	}
}

func TestTables(t *testing.T) {
	m := mustParse(t, `
[server]
ip = "10.0.0.1"

[server.options]
timeout = 30

[client]
name = "x"
`)
	if v := get(t, m, "server", "ip"); v != "10.0.0.1" {
		t.Errorf("server.ip = %v", v)
	}
	if v := get(t, m, "server", "options", "timeout"); v != int64(30) {
		t.Errorf("timeout = %v", v)
	}
	if v := get(t, m, "client", "name"); v != "x" {
		t.Errorf("client.name = %v", v)
	}
	// Empty table.
	m = mustParse(t, "[a]")
	if e := get(t, m, "a").(*Map); e.Len() != 0 {
		t.Errorf("empty table not empty")
	}
}

func TestSuperTableOutOfOrder(t *testing.T) {
	m := mustParse(t, "[a.b]\nc = 1\n[a]\nd = 2")
	if v := get(t, m, "a", "b", "c"); v != int64(1) {
		t.Errorf("a.b.c = %v", v)
	}
	if v := get(t, m, "a", "d"); v != int64(2) {
		t.Errorf("a.d = %v", v)
	}
}

func TestArrayOfTables(t *testing.T) {
	m := mustParse(t, `
[[fruit]]
name = "apple"

[[fruit]]
name = "banana"

[[products]]
[[products]]
sku = 1
`)
	fr, _ := m.Get("fruit")
	arr := fr.([]any)
	if len(arr) != 2 {
		t.Fatalf("fruit len = %d", len(arr))
	}
	if v, _ := arr[0].(*Map).Get("name"); v != "apple" {
		t.Errorf("fruit[0] = %v", v)
	}
	if v, _ := arr[1].(*Map).Get("name"); v != "banana" {
		t.Errorf("fruit[1] = %v", v)
	}
	pr, _ := m.Get("products")
	if len(pr.([]any)) != 2 {
		t.Errorf("products len = %d", len(pr.([]any)))
	}
}

func TestNestedArrayOfTables(t *testing.T) {
	m := mustParse(t, `
[[fruit]]
name = "apple"

[[fruit.variety]]
name = "red delicious"

[[fruit.variety]]
name = "granny smith"

[[fruit]]
name = "banana"
`)
	fr := mustGet(t, m, "fruit").([]any)
	if len(fr) != 2 {
		t.Fatalf("fruit len %d", len(fr))
	}
	varr, _ := fr[0].(*Map).Get("variety")
	if len(varr.([]any)) != 2 {
		t.Errorf("variety len %d", len(varr.([]any)))
	}
}

// mustGet returns m[key] or fails.
func mustGet(t *testing.T, m *Map, key string) Value {
	t.Helper()
	v, ok := m.Get(key)
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	return v
}

func TestDateTimes(t *testing.T) {
	m := mustParse(t, `
odt1 = 1979-05-27T07:32:00Z
odt2 = 1979-05-27T00:32:00-07:00
odt3 = 1979-05-27 07:32:00Z
odt4 = 1979-05-27T00:32:00.999999-07:00
ldt = 1979-05-27T07:32:00
ldt2 = 1979-05-27T07:32:00.5
ld = 1979-05-27
lt = 07:32:00
lt2 = 00:32:00.999999
`)
	odt1 := mustGet(t, m, "odt1").(OffsetDateTime)
	if !odt1.Time.Equal(time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC)) {
		t.Errorf("odt1 = %v", odt1.Time)
	}
	odt3 := mustGet(t, m, "odt3").(OffsetDateTime)
	if !odt3.Time.Equal(time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC)) {
		t.Errorf("odt3 (space sep) = %v", odt3.Time)
	}
	odt4 := mustGet(t, m, "odt4").(OffsetDateTime)
	if odt4.Time.Nanosecond() != 999999000 {
		t.Errorf("odt4 frac = %d", odt4.Time.Nanosecond())
	}
	ldt := mustGet(t, m, "ldt").(LocalDateTime)
	if ldt != (LocalDateTime{Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32}) {
		t.Errorf("ldt = %#v", ldt)
	}
	ldt2 := mustGet(t, m, "ldt2").(LocalDateTime)
	if ldt2.Nanosecond != 500000000 {
		t.Errorf("ldt2 frac = %d", ldt2.Nanosecond)
	}
	ld := mustGet(t, m, "ld").(LocalDate)
	if ld != (LocalDate{1979, 5, 27}) {
		t.Errorf("ld = %#v", ld)
	}
	lt := mustGet(t, m, "lt").(LocalTime)
	if lt != (LocalTime{Hour: 7, Minute: 32}) {
		t.Errorf("lt = %#v", lt)
	}
	lt2 := mustGet(t, m, "lt2").(LocalTime)
	if lt2.Nanosecond != 999999000 {
		t.Errorf("lt2 frac = %d", lt2.Nanosecond)
	}
}

func TestCommentsAndBlanks(t *testing.T) {
	m := mustParse(t, "# header comment\n\n  # indented\na = 1 # trailing\n\nb = 2\n")
	if v, _ := m.Get("a"); v != int64(1) {
		t.Errorf("a = %v", v)
	}
	if v, _ := m.Get("b"); v != int64(2) {
		t.Errorf("b = %v", v)
	}
	// Comment-only document.
	empty := mustParse(t, "# just a comment\n")
	if empty.Len() != 0 {
		t.Errorf("comment-only not empty")
	}
}

func TestCRLF(t *testing.T) {
	m := mustParse(t, "a = 1\r\nb = 2\r\n")
	if v, _ := m.Get("b"); v != int64(2) {
		t.Errorf("crlf b = %v", v)
	}
}

func TestBOM(t *testing.T) {
	m := mustParse(t, "\ufeffa = 1")
	if v, _ := m.Get("a"); v != int64(1) {
		t.Errorf("bom a = %v", v)
	}
}

func TestEmptyDocument(t *testing.T) {
	m := mustParse(t, "")
	if m.Len() != 0 {
		t.Errorf("empty doc not empty")
	}
	m = mustParse(t, "   \n\n  ")
	if m.Len() != 0 {
		t.Errorf("whitespace doc not empty")
	}
}
