// Copyright (c) the go-ruby-toml/toml authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package toml is a pure-Go (CGO-free) TOML v1.0.0 parser and generator for the
// Ruby value model, faithful to the toml-rb gem's semantics. Parse turns a TOML
// document into a Ruby Hash (an insertion-ordered [*Map]); Dump renders such a
// value back to a TOML string, so it is the deterministic, interpreter-independent
// core of toml-rb's TomlRB.parse / TomlRB.dump — without any Ruby runtime.
//
// # Ruby value model
//
// A parsed document is an [any] drawn from a small, fixed set of Go types so a
// host (such as go-embedded-ruby) can map its own object graph to and from this
// package:
//
//	TOML                      Go (Parse returns)        Ruby (rbgo maps to)
//	----                      ------------------        -------------------
//	string                    string                    String
//	integer                   int64                     Integer
//	float / inf / nan         float64                   Float
//	boolean                   bool                      true / false
//	array                     []any                     Array
//	table / inline table      *Map (insertion order)    Hash
//	offset date-time          OffsetDateTime            Time
//	local date-time           LocalDateTime             Time
//	local date                LocalDate                 Time (00:00, local)
//	local time                LocalTime                 Time (1970-01-01, local)
//
// toml-rb collapses every TOML date/time onto Ruby's Time using the host's local
// zone for the offset-less variants; this package keeps the faithful TOML datetime
// model (an explicit local/offset distinction with no host-zone guessing) so the
// host applies its own zone exactly once when it materialises a Ruby Time. Dump
// accepts these shapes plus Go's [time.Time].
package toml

import "time"

// Value is the interface satisfied by every value this package handles. It is
// purely documentary — the public API uses any — but a host may use it to
// constrain its own adapters.
type Value = any

// OffsetDateTime is a TOML offset date-time (RFC 3339 with a `Z` or `±hh:mm`
// suffix). It is a thin alias over [time.Time], which already carries the offset.
type OffsetDateTime struct {
	Time time.Time
}

// LocalDateTime is a TOML local date-time (RFC 3339 with no offset). The fields
// are wall-clock components with no zone; the host supplies the zone when it maps
// the value to a Ruby Time.
type LocalDateTime struct {
	Year, Month, Day     int
	Hour, Minute, Second int
	Nanosecond           int
}

// LocalDate is a TOML local date (`YYYY-MM-DD`).
type LocalDate struct {
	Year, Month, Day int
}

// LocalTime is a TOML local time (`HH:MM:SS[.fff]`).
type LocalTime struct {
	Hour, Minute, Second int
	Nanosecond           int
}

// Pair is one entry of an ordered mapping.
type Pair struct {
	Key string
	Val Value
}

// Map is an insertion-ordered Ruby Hash. Parse returns tables and inline tables
// as *Map so key order round-trips; Dump accepts *Map or a plain Go map (emitted
// in sorted key order for determinism).
type Map struct {
	pairs []Pair
	index map[string]int
	// explicit marks tables created by an explicit [table] / [[array]] header
	// (as opposed to springing into being from a dotted key or a parent header),
	// so the parser can reject a later redefinition the way toml-rb does.
	explicit bool
	// inline marks a table written with the inline `{ … }` syntax, which TOML
	// freezes against later extension.
	inline bool
	// fromDotted marks a table created mid-key by a dotted assignment (`a.b = 1`),
	// which may not later be reopened as an explicit [table].
	fromDotted bool
}

// NewMap returns an empty ordered Map.
func NewMap() *Map { return &Map{index: map[string]int{}} }

// Len reports the number of entries.
func (m *Map) Len() int { return len(m.pairs) }

// Pairs returns the entries in insertion order. The slice must not be mutated.
func (m *Map) Pairs() []Pair { return m.pairs }

// Set inserts or replaces the entry for key, preserving first-insertion order.
func (m *Map) Set(key string, val Value) {
	if m.index == nil {
		m.index = map[string]int{}
	}
	if i, ok := m.index[key]; ok {
		m.pairs[i].Val = val
		return
	}
	m.index[key] = len(m.pairs)
	m.pairs = append(m.pairs, Pair{Key: key, Val: val})
}

// Get returns the value for key and whether it was present.
func (m *Map) Get(key string) (Value, bool) {
	if m.index == nil {
		return nil, false
	}
	if i, ok := m.index[key]; ok {
		return m.pairs[i].Val, true
	}
	return nil, false
}
