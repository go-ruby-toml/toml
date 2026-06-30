// Copyright (c) the go-ruby-toml/toml authors
//
// SPDX-License-Identifier: BSD-3-Clause

package toml

import "os"

// ParseError reports a TOML syntax error. Line and Col are 1-based positions of
// the offending byte; Offset is its 0-based index in the document.
type ParseError struct {
	Msg    string
	Line   int
	Col    int
	Offset int
}

// Error renders the message with its source position, mirroring the location
// detail TomlRB::ParseError carries.
func (e *ParseError) Error() string { return e.Msg }

// OverwriteError reports a key defined more than once — the condition toml-rb
// raises as TomlRB::ValueOverwriteError. Key is the offending key path component.
type OverwriteError struct {
	Key string
}

// Error renders the toml-rb message verbatim.
func (e *OverwriteError) Error() string {
	return "Key \"" + e.Key + "\" is defined more than once"
}

// Parse parses a TOML v1.0.0 document into a Ruby Hash (an insertion-ordered
// [*Map]), matching TomlRB.parse / TOML.parse. A syntax error returns a
// [*ParseError]; a duplicate key returns a [*OverwriteError].
func Parse(s string) (*Map, error) {
	return parse(s)
}

// LoadFile reads path and parses it as a TOML document, matching
// TomlRB.load_file / TOML.load_file.
func LoadFile(path string) (*Map, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parse(string(b))
}

// Dump renders a Ruby value (a [*Map] or Go map for the document root) to a TOML
// v1.0.0 string, matching TomlRB.dump. A value outside the supported model
// returns an error rather than panicking.
func Dump(v Value) (string, error) {
	return dump(v)
}
