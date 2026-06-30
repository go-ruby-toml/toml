// Copyright (c) the go-ruby-toml/toml authors
//
// SPDX-License-Identifier: BSD-3-Clause

package toml

import (
	"math"
	"math/big"
	"strings"
	"testing"
	"time"
)

// mustDump dumps v or fails.
func mustDump(t *testing.T, v Value) string {
	t.Helper()
	s, err := Dump(v)
	if err != nil {
		t.Fatalf("Dump(%#v): %v", v, err)
	}
	return s
}

func TestDumpScalars(t *testing.T) {
	m := NewMap()
	m.Set("s", "hello")
	m.Set("quote", "a\"b\\c\nd\te")
	m.Set("i", int(7))
	m.Set("i64", int64(42))
	m.Set("big", big.NewInt(1000))
	m.Set("f", 3.5)
	m.Set("f32", float32(2.0))
	m.Set("intf", 4.0)
	m.Set("b", true)
	m.Set("bf", false)
	out := mustDump(t, m)
	for _, want := range []string{
		`s = "hello"`,
		`quote = "a\"b\\c\nd\te"`,
		`i = 7`,
		`i64 = 42`,
		`big = 1000`,
		`f = 3.5`,
		`f32 = 2.0`,
		`intf = 4.0`,
		`b = true`,
		`bf = false`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dump missing %q in:\n%s", want, out)
		}
	}
}

func TestDumpSpecialFloats(t *testing.T) {
	m := NewMap()
	m.Set("inf", math.Inf(1))
	m.Set("ninf", math.Inf(-1))
	m.Set("nan", math.NaN())
	out := mustDump(t, m)
	for _, w := range []string{"inf = inf", "ninf = -inf", "nan = nan"} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in %s", w, out)
		}
	}
}

func TestDumpArraysAndInline(t *testing.T) {
	m := NewMap()
	m.Set("nums", []any{1, 2, 3})
	m.Set("empty", []any{})
	inner := NewMap()
	inner.Set("x", 1)
	m.Set("pt", inner)
	out := mustDump(t, m)
	if !strings.Contains(out, "nums = [1, 2, 3]") {
		t.Errorf("array dump: %s", out)
	}
	if !strings.Contains(out, "empty = []") {
		t.Errorf("empty array dump: %s", out)
	}
	// A *Map value becomes a [pt] section.
	if !strings.Contains(out, "[pt]") || !strings.Contains(out, "x = 1") {
		t.Errorf("table section dump: %s", out)
	}
}

func TestDumpInlineArrayOfNonTables(t *testing.T) {
	m := NewMap()
	m.Set("data", []any{[]any{"a", "b"}, []any{1}})
	out := mustDump(t, m)
	if !strings.Contains(out, `data = [["a", "b"], [1]]`) {
		t.Errorf("nested array dump: %s", out)
	}
}

func TestDumpTablesAndAOT(t *testing.T) {
	root := NewMap()
	root.Set("title", "x")
	srv := NewMap()
	srv.Set("ip", "10.0.0.1")
	opts := NewMap()
	opts.Set("timeout", 30)
	srv.Set("opts", opts)
	root.Set("server", srv)
	f1 := NewMap()
	f1.Set("name", "apple")
	f2 := NewMap()
	f2.Set("name", "banana")
	root.Set("fruit", []any{f1, f2})
	out := mustDump(t, root)
	for _, w := range []string{"title = \"x\"", "[server]", "ip = \"10.0.0.1\"", "[server.opts]", "timeout = 30", "[[fruit]]", `name = "apple"`, `name = "banana"`} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
}

func TestDumpDateTimes(t *testing.T) {
	m := NewMap()
	m.Set("t", time.Date(2026, 6, 30, 1, 2, 3, 0, time.UTC))
	m.Set("odt", OffsetDateTime{Time: time.Date(2026, 6, 30, 1, 2, 3, 0, time.UTC)})
	m.Set("ldt", LocalDateTime{Year: 2026, Month: 6, Day: 30, Hour: 1, Minute: 2, Second: 3})
	m.Set("ldtf", LocalDateTime{Year: 2026, Month: 6, Day: 30, Hour: 1, Minute: 2, Second: 3, Nanosecond: 500000000})
	m.Set("ld", LocalDate{2026, 6, 30})
	m.Set("lt", LocalTime{Hour: 1, Minute: 2, Second: 3})
	m.Set("ltf", LocalTime{Hour: 1, Minute: 2, Second: 3, Nanosecond: 123000000})
	out := mustDump(t, m)
	for _, w := range []string{
		"t = 2026-06-30T01:02:03Z",
		"odt = 2026-06-30T01:02:03Z",
		"ldt = 2026-06-30T01:02:03",
		"ldtf = 2026-06-30T01:02:03.5",
		"ld = 2026-06-30",
		"lt = 01:02:03",
		"ltf = 01:02:03.123",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
}

func TestDumpQuotedKeys(t *testing.T) {
	m := NewMap()
	m.Set("a.b", 1)
	m.Set("", 2)
	out := mustDump(t, m)
	if !strings.Contains(out, `"a.b" = 1`) {
		t.Errorf("quoted key dump: %s", out)
	}
	if !strings.Contains(out, `"" = 2`) {
		t.Errorf("empty key dump: %s", out)
	}
}

func TestDumpGoMapSorted(t *testing.T) {
	out := mustDump(t, map[string]any{"b": 2, "a": 1, "c": 3})
	ai := strings.Index(out, "a = 1")
	bi := strings.Index(out, "b = 2")
	ci := strings.Index(out, "c = 3")
	if !(ai < bi && bi < ci) {
		t.Errorf("go map not sorted:\n%s", out)
	}
}

func TestDumpGoMapNestedTableAndAOT(t *testing.T) {
	out := mustDump(t, map[string]any{
		"t":   map[string]any{"k": 1},
		"arr": []any{map[string]any{"n": "a"}, map[string]any{"n": "b"}},
	})
	if !strings.Contains(out, "[t]") || !strings.Contains(out, "[[arr]]") {
		t.Errorf("go map nested dump:\n%s", out)
	}
}

func TestDumpInlineNestedMap(t *testing.T) {
	// A *Map nested inside an array is emitted inline.
	inner := NewMap()
	inner.Set("x", 1)
	gomapInner := map[string]any{"y": 2}
	m := NewMap()
	m.Set("arr", []any{inner, gomapInner})
	// arr is an array-of-tables (all elements are tables) → [[arr]] sections.
	out := mustDump(t, m)
	if !strings.Contains(out, "[[arr]]") {
		t.Errorf("aot from mixed map kinds:\n%s", out)
	}
}

func TestDumpErrors(t *testing.T) {
	if _, err := Dump(nil); err == nil {
		t.Error("Dump(nil) should error")
	}
	if _, err := Dump(42); err == nil {
		t.Error("Dump(scalar root) should error")
	}
	// nil inside a table.
	m := NewMap()
	m.Set("x", nil)
	if _, err := Dump(m); err == nil {
		t.Error("Dump with nil value should error")
	}
	// unsupported value type inside a table.
	m2 := NewMap()
	m2.Set("x", make(chan int))
	if _, err := Dump(m2); err == nil {
		t.Error("Dump unsupported type should error")
	}
	// unsupported inside an array.
	m3 := NewMap()
	m3.Set("x", []any{make(chan int)})
	if _, err := Dump(m3); err == nil {
		t.Error("Dump array with bad elem should error")
	}
	// unsupported inside an inline map nested in array (not an AOT).
	bad := NewMap()
	bad.Set("y", make(chan int))
	m4 := NewMap()
	m4.Set("x", []any{int64(1), bad}) // mixed: not an AOT → inline emission
	if _, err := Dump(m4); err == nil {
		t.Error("Dump inline map with bad value should error")
	}
	// bad value inside an explicit sub-table.
	sub := NewMap()
	sub.Set("y", make(chan int))
	m5 := NewMap()
	m5.Set("t", sub)
	if _, err := Dump(m5); err == nil {
		t.Error("Dump sub-table with bad value should error")
	}
	// bad value inside an array-of-tables element.
	at := NewMap()
	at.Set("y", make(chan int))
	m6 := NewMap()
	m6.Set("a", []any{at})
	if _, err := Dump(m6); err == nil {
		t.Error("Dump AOT with bad value should error")
	}
	// bad value inside a go-map nested table.
	if _, err := Dump(map[string]any{"t": map[string]any{"y": make(chan int)}}); err == nil {
		t.Error("Dump go-map sub-table bad value should error")
	}
	// bad value inside a go-map AOT.
	if _, err := Dump(map[string]any{"a": []any{map[string]any{"y": make(chan int)}}}); err == nil {
		t.Error("Dump go-map AOT bad value should error")
	}
	// inline map (in array) carrying a go-map with bad value.
	gm := map[string]any{"z": make(chan int)}
	m7 := NewMap()
	m7.Set("x", []any{int64(1), gm})
	if _, err := Dump(m7); err == nil {
		t.Error("Dump inline go-map bad value should error")
	}
}

func TestDumpInlineMapFromGoMap(t *testing.T) {
	// A go-map appearing where inline emission is required (array of mixed types).
	m := NewMap()
	m.Set("x", []any{int64(1), map[string]any{"k": 2}})
	out := mustDump(t, m)
	if !strings.Contains(out, "{k = 2}") {
		t.Errorf("inline go-map dump: %s", out)
	}
}

func TestRoundTrip(t *testing.T) {
	src := `title = "x"
n = 42
f = 3.5
b = true
arr = [1, 2, 3]

[server]
ip = "10.0.0.1"

[[items]]
name = "a"

[[items]]
name = "b"
`
	m, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	out := mustDump(t, m)
	m2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse: %v\n%s", err, out)
	}
	if v := get(t, m2, "server", "ip"); v != "10.0.0.1" {
		t.Errorf("round-trip server.ip = %v", v)
	}
	items := mustGet(t, m2, "items").([]any)
	if len(items) != 2 {
		t.Errorf("round-trip items = %d", len(items))
	}
}
