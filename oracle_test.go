// Copyright (c) the go-ruby-toml/toml authors
//
// SPDX-License-Identifier: BSD-3-Clause

package toml

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// rubyBin locates a usable `ruby` with toml-rb once. The oracle tests skip
// themselves when it (or the gem) is absent — the qemu cross-arch lanes and the
// Windows lane — so the deterministic suite alone drives the 100% gate there.
func rubyBin(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("ruby not on PATH; skipping toml-rb oracle")
	}
	check := exec.Command(path, "-e", "require 'toml-rb'")
	if err := check.Run(); err != nil {
		t.Skip("toml-rb gem not installed; skipping oracle")
	}
	return path
}

// rubyParseJSON runs toml-rb on src and returns its result as canonical JSON. The
// script $stdout.binmode's so Windows text-mode never pollutes the bytes.
func rubyParseJSON(t *testing.T, bin, src string) string {
	t.Helper()
	script := `$stdout.binmode
require 'toml-rb'
require 'json'
require 'time'
src = STDIN.read
v = TomlRB.parse(src)
def norm(x)
  case x
  when Hash then x.transform_values { |e| norm(e) }
  when Array then x.map { |e| norm(e) }
  when Time then "TIME:" + x.strftime('%Y-%m-%dT%H:%M:%S.%9N%:z')
  when Float
    if x.infinite? == 1 then "INF" elsif x.infinite? == -1 then "-INF" elsif x.nan? then "NAN" else x end
  else x
  end
end
print JSON.generate(norm(v))
`
	cmd := exec.Command(bin, "-e", script)
	cmd.Stdin = strings.NewReader(src)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("toml-rb parse error: %v\nsrc:\n%s\nout:\n%s", err, src, out)
	}
	return string(out)
}

// goParseJSON parses src here and renders it through the same canonical scheme as
// the Ruby side so the two JSON strings are directly comparable.
func goParseJSON(t *testing.T, src string) string {
	t.Helper()
	m, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	b, err := json.Marshal(canon(m))
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

// canon converts a parsed value into the canonical comparison form: tables become
// plain maps, datetimes become the "TIME:<rfc3339nano>" string toml-rb's local
// projection would print, and non-finite floats become sentinel strings.
func canon(v Value) any {
	switch x := v.(type) {
	case *Map:
		out := map[string]any{}
		for _, pr := range x.pairs {
			out[pr.Key] = canon(pr.Val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = canon(e)
		}
		return out
	case OffsetDateTime:
		return "TIME:" + x.Time.Format("2006-01-02T15:04:05.000000000-07:00")
	case LocalDateTime, LocalDate, LocalTime:
		// Compared structurally in a dedicated test, not via the JSON oracle.
		return fmt.Sprintf("LOCAL:%v", v)
	case float64:
		switch {
		case math.IsInf(x, 1):
			return "INF"
		case math.IsInf(x, -1):
			return "-INF"
		case math.IsNaN(x):
			return "NAN"
		}
		return x
	default:
		return v
	}
}

// TestOracleParse parses a corpus both here and in toml-rb and asserts the two
// canonical JSON renderings agree. Local datetimes/dates/times are excluded from
// this comparison (toml-rb projects them onto the host's local zone, which this
// model deliberately does not) and covered structurally in TestDateTimes.
func TestOracleParse(t *testing.T) {
	bin := rubyBin(t)
	corpus := []string{
		`title = "TOML Example"`,
		"a = 1\nb = 2.5\nc = true\nd = false",
		`s = "é\t\né"`,
		"i = 0xFF\nj = 0o17\nk = 0b101\nl = 1_000_000",
		"f = 3.14\ng = -0.01\nh = 6.626e-34\nq = 5e+22",
		"inf = inf\nninf = -inf\nn = nan",
		"arr = [1, 2, 3]\nmixed = [1, \"x\", 3.0]\nnested = [[1], [2, 3]]",
		"pt = { x = 1, y = 2, label = \"o\" }",
		"odt = 1979-05-27T07:32:00Z\nodt2 = 1979-05-27T00:32:00-07:00",
		"[server]\nip = \"10.0.0.1\"\n[server.opts]\ntimeout = 30",
		"[a.b]\nc = 1\n[a]\nd = 2",
		"[[fruit]]\nname = \"apple\"\n[[fruit]]\nname = \"banana\"",
		"[[p]]\nname = \"x\"\n[[p.v]]\nk = 1\n[[p.v]]\nk = 2\n[[p]]\nname = \"y\"",
		"dotted.a.b = 1\ndotted.a.c = 2",
		"ml = \"\"\"\nroses\nviolets\"\"\"",
		"lit = 'C:\\path\\no\\escape'",
		"mll = '''\nraw\nlines'''",
		"trail = [1, 2, ]\nempty = []\nemptytbl = {}",
		"# comment only doc\nx = 1 # trailing",
	}
	for i, src := range corpus {
		t.Run(fmt.Sprintf("case%02d", i), func(t *testing.T) {
			want := rubyParseJSON(t, bin, src)
			got := goParseJSON(t, src)
			if !jsonEqual(t, got, want) {
				t.Errorf("oracle mismatch for src:\n%s\n go:   %s\n ruby: %s", src, got, want)
			}
		})
	}
}

// jsonEqual compares two JSON documents for structural equality (key order and
// float formatting independent).
func jsonEqual(t *testing.T, a, b string) bool {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal([]byte(a), &av); err != nil {
		t.Fatalf("bad go json %q: %v", a, err)
	}
	if err := json.Unmarshal([]byte(b), &bv); err != nil {
		t.Fatalf("bad ruby json %q: %v", b, err)
	}
	return deepJSONEqual(av, bv)
}

// deepJSONEqual compares two decoded JSON trees, allowing tiny float drift.
func deepJSONEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		keys := make([]string, 0, len(av))
		for k := range av {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if !deepJSONEqual(av[k], bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !deepJSONEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case float64:
		bf, ok := b.(float64)
		if !ok {
			return false
		}
		if av == bf {
			return true
		}
		return math.Abs(av-bf) <= 1e-9*math.Max(1, math.Abs(av))
	default:
		return a == b
	}
}

// TestOracleSpecExample round-trips the canonical TOML spec example through both
// implementations and asserts agreement (offset datetimes only; local stays out).
func TestOracleSpecExample(t *testing.T) {
	bin := rubyBin(t)
	src := `# canonical example
title = "TOML Example"

[owner]
name = "Tom Preston-Werner"
dob = 1979-05-27T07:32:00-08:00

[database]
enabled = true
ports = [ 8000, 8001, 8002 ]
data = [ ["delta", "phi"], [3.14] ]
temp_targets = { cpu = 79.5, case = 72.0 }

[servers]

[servers.alpha]
ip = "10.0.0.1"
role = "frontend"

[servers.beta]
ip = "10.0.0.2"
role = "backend"

[[products]]
name = "Hammer"
sku = 738594937

[[products]]  # empty table within the array

[[products]]
name = "Nail"
sku = 284758393
color = "gray"
`
	want := rubyParseJSON(t, bin, src)
	got := goParseJSON(t, src)
	if !jsonEqual(t, got, want) {
		t.Errorf("spec example mismatch\n go:   %s\n ruby: %s", got, want)
	}
}

// TestOracleErrors checks that documents toml-rb rejects also fail here. Inputs
// where the two intentionally diverge (e.g. toml-rb's leniency toward leading
// zeros) are excluded.
func TestOracleErrors(t *testing.T) {
	bin := rubyBin(t)
	bad := []string{
		"a = 1\na = 2",
		"[x]\ny = 1\n[x]\nz = 2",
		"t = {k = 1, k = 2}",
		"a = .7",
		"a = [1, 2",
		"a 1",
		"a = ",
		"a = 1\na.b = 2",
		"[[a]]\nx = 1\n[a]\ny = 2",
	}
	for i, src := range bad {
		t.Run(fmt.Sprintf("bad%02d", i), func(t *testing.T) {
			if rubyAccepts(t, bin, src) {
				t.Skipf("toml-rb accepts %q; skipping divergent case", src)
			}
			if _, err := Parse(src); err == nil {
				t.Errorf("Parse(%q) succeeded; toml-rb rejects it", src)
			}
		})
	}
}

// rubyAccepts reports whether toml-rb parses src without raising.
func rubyAccepts(t *testing.T, bin, src string) bool {
	t.Helper()
	script := "$stdout.binmode\nrequire 'toml-rb'\nbegin; TomlRB.parse(STDIN.read); print 'OK'; rescue => e; print 'ERR'; end\n"
	cmd := exec.Command(bin, "-e", script)
	cmd.Stdin = strings.NewReader(src)
	out, _ := cmd.Output()
	return string(out) == "OK"
}

// TestOracleLoadFile parses a temp file both ways and compares.
func TestOracleLoadFile(t *testing.T) {
	bin := rubyBin(t)
	src := "title = \"x\"\n[t]\nk = 1\n"
	f, err := os.CreateTemp(t.TempDir(), "*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(src); err != nil {
		t.Fatal(err)
	}
	f.Close()
	m, err := LoadFile(f.Name())
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	b, _ := json.Marshal(canon(m))
	want := rubyParseJSON(t, bin, src)
	if !jsonEqual(t, string(b), want) {
		t.Errorf("LoadFile mismatch: go=%s ruby=%s", b, want)
	}
}
