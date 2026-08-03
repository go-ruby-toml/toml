// Copyright (c) the go-ruby-toml/toml authors
//
// SPDX-License-Identifier: BSD-3-Clause

package toml

import (
	"embed"
	"encoding/json"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// tomlTest is the canonical cross-implementation TOML conformance corpus
// toml-lang/toml-test (github.com/toml-lang/toml-test, tests/). Each valid case
// is a `.toml` input paired with a `.json` file giving the expected decoded value
// in the suite's type-tagged form ({"type":..., "value":...}); each invalid case
// is a `.toml` that a conformant decoder MUST reject. The vendored
// files-toml-1.0.0 manifest selects exactly the TOML v1.0.0 subset this package
// targets, so the v1.1.0-only vectors are excluded rather than forced.
//
//go:embed tomltest
var tomlTest embed.FS

// tomlScalarTypes is the set of tagged-leaf type names in toml-test .json files.
var tomlScalarTypes = map[string]bool{
	"string": true, "integer": true, "float": true, "bool": true,
	"datetime": true, "datetime-local": true, "date-local": true, "time-local": true,
}

// tomlTestKnownFailing is the frozen set of toml-test cases (keyed by the input's
// manifest path, e.g. "valid/array/array.toml") this package does not yet handle
// to the spec's verdict. It is a shrink-only conformance RATCHET: every case NOT
// listed here (and not a documented divergence, see tomlKnownDivergences) must
// produce the required result — a valid case must parse and match its tagged-JSON
// expectation byte/semantics-exact, an invalid case must be rejected — so no
// change may introduce a new divergence, and a listed case that starts passing is
// reported so the entry can be removed. Baseline captured 2026-08-03 against TOML
// v1.0.0 (files-toml-1.0.0): 658/709 cases pass (92.81%), 51 gaps. Closing the
// control-character/invalid-UTF-8 lexing gap (26) and the multi-line literal
// trailing-quote gap (3) takes the suite to 687/709 (96.90%).
//
// The remaining 20 shrinkable gaps break down as (all genuine parser behaviour):
//   - Calendar/offset validation (7): Feb 29/30 on non-leap dates and out-of-
//     range time-zone offsets are accepted (invalid/{datetime,local-date,
//     local-datetime}/feb-*, invalid/datetime/offset-overflow-*).
//   - Table/array redefinition rules (7): a few reopen/extend-after-dotted and
//     append-to-defined cases are not rejected (invalid/table/append-with-dotted-
//     keys-0{1,2,8}, invalid/array/{extending-table,tables-01}, invalid/inline-
//     table/overwrite-02).
//   - Numeric/float lexing (5): double sign, a bad hex digit and `exp-dot`
//     malformations are accepted (invalid/integer/*, invalid/float/exp-dot-0{2,3}).
//   - Valid input wrongly rejected (1): int64 min/max boundary literals
//     (valid/integer/long).
//
// Each is a dedicated gap-closing target; the set may only shrink.
var tomlTestKnownFailing = map[string]bool{
	"invalid/array/extending-table.toml":            true,
	"invalid/array/tables-01.toml":                  true,
	"invalid/datetime/feb-29.toml":                  true,
	"invalid/datetime/feb-30.toml":                  true,
	"invalid/datetime/offset-overflow-hour.toml":    true,
	"invalid/datetime/offset-overflow-minute.toml":  true,
	"invalid/float/exp-dot-02.toml":                 true,
	"invalid/float/exp-dot-03.toml":                 true,
	"invalid/inline-table/overwrite-02.toml":        true,
	"invalid/integer/double-sign-nex.toml":          true,
	"invalid/integer/double-sign-plus.toml":         true,
	"invalid/integer/invalid-hex-03.toml":           true,
	"invalid/local-date/feb-29.toml":                true,
	"invalid/local-date/feb-30.toml":                true,
	"invalid/local-datetime/feb-29.toml":            true,
	"invalid/local-datetime/feb-30.toml":            true,
	"invalid/table/append-with-dotted-keys-01.toml": true,
	"invalid/table/append-with-dotted-keys-02.toml": true,
	"invalid/table/append-with-dotted-keys-08.toml": true,
	"valid/integer/long.toml":                       true,
}

// tomlKnownDivergences records the toml-test cases this package intentionally
// does NOT resolve to toml-test's strict TOML v1.0.0 verdict because it follows
// toml-rb (the reference this go-ruby- port mirrors) instead. They are permanent,
// documented exceptions — excluded from the ratchet in both directions — not gaps
// to close:
//
//   - invalid/string/basic-byte-escapes: toml-rb accepts the `\xHH` byte escape
//     (and `\e`) as an extension; TestStringEscapes pins that behaviour, so `\x33`
//     is accepted where strict v1.0.0 rejects it.
//   - invalid/string/multiline-quotes-01: for basic multi-line strings toml-rb
//     consumes an arbitrarily long run of quotes adjacent to the delimiter as
//     literal content (the run's final three are the delimiter), so a six-quote
//     run yields three literal quotes rather than an error; TestMultilineBasic
//     ("nine quotes") pins this. The multi-line LITERAL parser has no such pin and
//     does apply the strict v1.0.0 rule (a run of six or more is rejected), so the
//     literal analogues invalid/string/literal-multiline-quotes-0{1,2} are
//     correctly rejected and stay outside this map.
var tomlKnownDivergences = map[string]bool{
	"invalid/string/basic-byte-escapes.toml":  true,
	"invalid/string/multiline-quotes-01.toml": true,
}

// TestTomlTestConformance is the differential conformance gate against the
// canonical toml-lang/toml-test corpus (TOML v1.0.0 subset). Every case outside
// tomlTestKnownFailing must reach the required verdict; a new divergence fails CI
// and a listed case that now passes is reported so the ratchet can be tightened.
func TestTomlTestConformance(t *testing.T) {
	manifest, err := tomlTest.ReadFile("tomltest/files-toml-1.0.0")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var validToml, invalidToml []string
	for _, ln := range strings.Split(strings.TrimSpace(string(manifest)), "\n") {
		switch {
		case strings.HasPrefix(ln, "valid/") && strings.HasSuffix(ln, ".toml"):
			validToml = append(validToml, ln)
		case strings.HasPrefix(ln, "invalid/") && strings.HasSuffix(ln, ".toml"):
			invalidToml = append(invalidToml, ln)
		}
	}
	if len(validToml) < 200 || len(invalidToml) < 400 {
		t.Fatalf("expected ~210 valid + ~499 invalid, got %d + %d", len(validToml), len(invalidToml))
	}

	pass, total := 0, 0
	var newFail, fixed []string
	record := func(key string, ok bool) {
		total++
		if ok {
			pass++
		}
		if tomlKnownDivergences[key] {
			// Intentional toml-rb divergence: excluded from the ratchet either way.
			return
		}
		switch {
		case ok && tomlTestKnownFailing[key]:
			fixed = append(fixed, key)
		case !ok && !tomlTestKnownFailing[key]:
			newFail = append(newFail, key)
		}
	}

	validPass, invalidPass := 0, 0
	for _, rel := range validToml {
		ok := checkValid(t, rel)
		if ok {
			validPass++
		}
		record(rel, ok)
	}
	for _, rel := range invalidToml {
		src, err := tomlTest.ReadFile("tomltest/" + rel)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		_, perr := Parse(string(src))
		ok := perr != nil // invalid input MUST be rejected
		if ok {
			invalidPass++
		}
		record(rel, ok)
	}

	t.Logf("toml-test v1.0.0: %d/%d cases pass (%.2f%%) — valid %d/%d parse+match, "+
		"invalid %d/%d rejected; %d known gaps", pass, total,
		100*float64(pass)/float64(total), validPass, len(validToml),
		invalidPass, len(invalidToml), len(tomlTestKnownFailing))
	if len(fixed) > 0 {
		sort.Strings(fixed)
		t.Errorf("cases now passing that are still listed in tomlTestKnownFailing: %v\n"+
			"remove them to tighten the ratchet", fixed)
	}
	if len(newFail) > 0 {
		sort.Strings(newFail)
		t.Errorf("REGRESSION: %d toml-test case(s) with the wrong verdict: %v",
			len(newFail), newFail)
	}
}

// checkValid parses a valid-case .toml and compares it against its tagged-JSON
// expectation.
func checkValid(t *testing.T, rel string) bool {
	src, err := tomlTest.ReadFile("tomltest/" + rel)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	got, perr := Parse(string(src))
	if perr != nil {
		return false
	}
	jsonPath := "tomltest/" + strings.TrimSuffix(rel, ".toml") + ".json"
	expRaw, err := tomlTest.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read %s: %v", jsonPath, err)
	}
	var exp any
	if err := json.Unmarshal(expRaw, &exp); err != nil {
		t.Fatalf("decode %s: %v", jsonPath, err)
	}
	return compareValue(exp, got)
}

// compareValue matches an expected toml-test tagged value against an actual
// parsed toml.Value.
func compareValue(exp any, act Value) bool {
	switch e := exp.(type) {
	case map[string]any:
		if typ, val, ok := taggedLeaf(e); ok {
			return compareLeaf(typ, val, act)
		}
		am, ok := act.(*Map)
		if !ok || am.Len() != len(e) {
			return false
		}
		for k, ev := range e {
			av, ok := am.Get(k)
			if !ok || !compareValue(ev, av) {
				return false
			}
		}
		return true
	case []any:
		aa, ok := act.([]any)
		if !ok || len(aa) != len(e) {
			return false
		}
		for i := range e {
			if !compareValue(e[i], aa[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// taggedLeaf reports whether m is a toml-test scalar leaf {"type":..,"value":..}
// and returns its type and value strings. A table never matches: its values are
// themselves tagged objects, so exp["type"]/exp["value"] would not both be
// strings.
func taggedLeaf(m map[string]any) (typ, val string, ok bool) {
	if len(m) != 2 {
		return "", "", false
	}
	tv, tok := m["type"].(string)
	vv, vok := m["value"].(string)
	if !tok || !vok || !tomlScalarTypes[tv] {
		return "", "", false
	}
	return tv, vv, true
}

// compareLeaf matches one tagged scalar against the actual Go value, applying the
// per-type semantics the reference toml-test comparator uses (numeric equality
// for floats, instant equality for offset datetimes, string equality for the
// local date/time forms).
func compareLeaf(typ, val string, act Value) bool {
	switch typ {
	case "string":
		s, ok := act.(string)
		return ok && s == val
	case "bool":
		b, ok := act.(bool)
		return ok && boolStr(b) == val
	case "integer":
		switch a := act.(type) {
		case int64:
			return strconv.FormatInt(a, 10) == val
		case int:
			return strconv.Itoa(a) == val
		case *big.Int:
			return a.String() == val
		}
		return false
	case "float":
		f, ok := act.(float64)
		if !ok {
			return false
		}
		return floatEqual(f, val)
	case "datetime":
		return instantEqual(act, val)
	case "datetime-local":
		d, ok := act.(LocalDateTime)
		return ok && d.String() == val
	case "date-local":
		d, ok := act.(LocalDate)
		return ok && d.String() == val
	case "time-local":
		tm, ok := act.(LocalTime)
		return ok && tm.String() == val
	}
	return false
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// floatEqual compares a parsed float64 against toml-test's float value string,
// handling the inf / -inf / nan spellings.
func floatEqual(f float64, val string) bool {
	switch strings.ToLower(strings.TrimPrefix(val, "+")) {
	case "inf":
		return math.IsInf(f, 1)
	case "-inf":
		return math.IsInf(f, -1)
	case "nan", "-nan":
		return math.IsNaN(f)
	}
	want, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return false
	}
	return f == want
}

// instantEqual compares an offset date-time value against the expected RFC 3339
// timestamp as the same instant.
func instantEqual(act Value, val string) bool {
	var got time.Time
	switch a := act.(type) {
	case OffsetDateTime:
		got = a.Time
	case time.Time:
		got = a
	default:
		return false
	}
	want, err := time.Parse(time.RFC3339Nano, val)
	if err != nil {
		return false
	}
	return got.Equal(want)
}
