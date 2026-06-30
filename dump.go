// Copyright (c) the go-ruby-toml/toml authors
//
// SPDX-License-Identifier: BSD-3-Clause

package toml

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

// dumpError reports a value outside the model Dump can emit.
type dumpError struct{ msg string }

func (e *dumpError) Error() string { return e.msg }

// dump renders the document root, mirroring TomlRB.dump: simple key/value pairs
// first, then `[table]` sections, then `[[array of tables]]`, recursing.
func dump(v Value) (string, error) {
	pairs, err := toPairs(v)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := emitTable(&b, pairs, nil); err != nil {
		return "", err
	}
	return b.String(), nil
}

// kv is one normalised root/table entry in emission order.
type kv struct {
	key string
	val Value
}

// toPairs normalises a *Map or Go map into ordered key/value entries. A *Map keeps
// insertion order; a Go map is sorted by key for deterministic output.
func toPairs(v Value) ([]kv, error) {
	switch m := v.(type) {
	case *Map:
		out := make([]kv, 0, m.Len())
		for _, pr := range m.pairs {
			out = append(out, kv{pr.Key, pr.Val})
		}
		return out, nil
	case map[string]any:
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]kv, 0, len(keys))
		for _, k := range keys {
			out = append(out, kv{k, m[k]})
		}
		return out, nil
	default:
		return nil, &dumpError{fmt.Sprintf("cannot dump %T as a TOML table", v)}
	}
}

// emitTable writes one table's entries: scalar/array pairs inline, sub-tables and
// arrays-of-tables as headered sections under the given key path prefix.
func emitTable(b *strings.Builder, pairs []kv, prefix []string) error {
	var subTables, arrayTables []kv
	for _, e := range pairs {
		switch ev := e.val.(type) {
		case *Map:
			subTables = append(subTables, e)
		case map[string]any:
			subTables = append(subTables, e)
		case []any:
			if isArrayOfTables(ev) {
				arrayTables = append(arrayTables, e)
				continue
			}
			s, err := emitInlineValue(e.val)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "%s = %s\n", emitKey(e.key), s)
		default:
			s, err := emitInlineValue(e.val)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "%s = %s\n", emitKey(e.key), s)
		}
	}
	for _, e := range subTables {
		path := append(append([]string{}, prefix...), e.key)
		fmt.Fprintf(b, "[%s]\n", strings.Join(emitKeyPath(path), "."))
		// e.val is a *Map or map[string]any (it landed in subTables), so the
		// normalisation never fails here.
		sub, _ := toPairs(e.val)
		if err := emitTable(b, sub, path); err != nil {
			return err
		}
	}
	for _, e := range arrayTables {
		path := append(append([]string{}, prefix...), e.key)
		for _, elem := range e.val.([]any) {
			fmt.Fprintf(b, "[[%s]]\n", strings.Join(emitKeyPath(path), "."))
			// isArrayOfTables guaranteed every element is a table.
			sub, _ := toPairs(elem)
			if err := emitTable(b, sub, path); err != nil {
				return err
			}
		}
	}
	return nil
}

// isArrayOfTables reports whether a non-empty array holds only tables (so it is
// emitted as `[[…]]` sections rather than an inline array).
func isArrayOfTables(a []any) bool {
	if len(a) == 0 {
		return false
	}
	for _, e := range a {
		switch e.(type) {
		case *Map, map[string]any:
		default:
			return false
		}
	}
	return true
}

// emitInlineValue renders a scalar or inline array/table value.
func emitInlineValue(v Value) (string, error) {
	switch val := v.(type) {
	case nil:
		return "", &dumpError{"cannot dump nil in TOML"}
	case bool:
		if val {
			return "true", nil
		}
		return "false", nil
	case string:
		return emitString(val), nil
	case int:
		return strconv.FormatInt(int64(val), 10), nil
	case int64:
		return strconv.FormatInt(val, 10), nil
	case *big.Int:
		return val.String(), nil
	case float64:
		return emitFloat(val), nil
	case float32:
		return emitFloat(float64(val)), nil
	case time.Time:
		return val.Format(time.RFC3339Nano), nil
	case OffsetDateTime:
		return val.Time.Format(time.RFC3339Nano), nil
	case LocalDateTime:
		return val.String(), nil
	case LocalDate:
		return val.String(), nil
	case LocalTime:
		return val.String(), nil
	case []any:
		return emitArray(val)
	case *Map:
		return emitInlineMapPairs(val.pairs)
	case map[string]any:
		ps, _ := toPairs(val)
		kvp := make([]Pair, len(ps))
		for i, e := range ps {
			kvp[i] = Pair{Key: e.key, Val: e.val}
		}
		return emitInlineMapPairs(kvp)
	default:
		return "", &dumpError{fmt.Sprintf("cannot dump %T as a TOML value", v)}
	}
}

// emitArray renders `[v, v, …]`.
func emitArray(a []any) (string, error) {
	parts := make([]string, len(a))
	for i, e := range a {
		s, err := emitInlineValue(e)
		if err != nil {
			return "", err
		}
		parts[i] = s
	}
	return "[" + strings.Join(parts, ", ") + "]", nil
}

// emitInlineMapPairs renders `{ k = v, … }`.
func emitInlineMapPairs(pairs []Pair) (string, error) {
	parts := make([]string, len(pairs))
	for i, pr := range pairs {
		s, err := emitInlineValue(pr.Val)
		if err != nil {
			return "", err
		}
		parts[i] = emitKey(pr.Key) + " = " + s
	}
	return "{" + strings.Join(parts, ", ") + "}", nil
}

// emitKey renders a key segment bare when possible, else as a basic-quoted string.
func emitKey(k string) string {
	if isBareKey(k) {
		return k
	}
	return emitString(k)
}

// emitKeyPath renders each path component as a (possibly quoted) key.
func emitKeyPath(path []string) []string {
	out := make([]string, len(path))
	for i, k := range path {
		out[i] = emitKey(k)
	}
	return out
}

// isBareKey reports whether k can be written without quotes.
func isBareKey(k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		if !isBareKeyChar(k[i]) {
			return false
		}
	}
	return true
}

// emitString renders a Go string as a TOML basic (double-quoted) string with the
// minimal escaping toml-rb emits.
func emitString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\t':
			b.WriteString("\\t")
		case '\r':
			b.WriteString("\\r")
		case '\b':
			b.WriteString("\\b")
		case '\f':
			b.WriteString("\\f")
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, "\\u%04X", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// emitFloat renders a float64 in TOML form, including inf / -inf / nan.
func emitFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case math.IsNaN(f):
		return "nan"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	// TOML floats must carry a fractional or exponent part; add `.0` if integral.
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// String renders a LocalDateTime in RFC 3339 (no offset).
func (d LocalDateTime) String() string {
	base := fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d",
		d.Year, d.Month, d.Day, d.Hour, d.Minute, d.Second)
	return base + fracSuffix(d.Nanosecond)
}

// String renders a LocalDate as YYYY-MM-DD.
func (d LocalDate) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// String renders a LocalTime as HH:MM:SS[.fff].
func (t LocalTime) String() string {
	return fmt.Sprintf("%02d:%02d:%02d", t.Hour, t.Minute, t.Second) + fracSuffix(t.Nanosecond)
}

// fracSuffix renders a non-zero nanosecond count as a trimmed `.fff` suffix.
func fracSuffix(ns int) string {
	if ns == 0 {
		return ""
	}
	s := fmt.Sprintf(".%09d", ns)
	s = strings.TrimRight(s, "0")
	return s
}

// inf returns +∞ or −∞ as a float64 (sign>0 ⇒ +∞).
func inf(sign int) float64 { return math.Inf(sign) }

// nan returns a NaN float64.
func nan() float64 { return math.NaN() }
