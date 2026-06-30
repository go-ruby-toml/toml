// Copyright (c) the go-ruby-toml/toml authors
//
// SPDX-License-Identifier: BSD-3-Clause

package toml

import (
	"strconv"
	"strings"
	"time"
)

// parser is a single-pass scanner over the document bytes. It tracks the current
// "active" table (where bare key/value lines land) and the document root.
type parser struct {
	src  string
	pos  int // 0-based byte offset
	root *Map
	cur  *Map // table that bare keys assign into
}

// parse is the package entry point behind Parse / LoadFile.
func parse(s string) (*Map, error) {
	// Strip a UTF-8 BOM if present (TOML files may carry one).
	s = strings.TrimPrefix(s, "\ufeff")
	p := &parser{src: s, root: NewMap()}
	p.cur = p.root
	if err := p.parseDocument(); err != nil {
		return nil, err
	}
	return p.root, nil
}

// errAt builds a ParseError carrying the 1-based line/col of byte off.
func (p *parser) errAt(off int, msg string) error {
	line, col := 1, 1
	for i := 0; i < off && i < len(p.src); i++ {
		if p.src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return &ParseError{Msg: msg, Line: line, Col: col, Offset: off}
}

// peek returns the byte at pos, or 0 at EOF.
func (p *parser) peek() byte {
	if p.pos < len(p.src) {
		return p.src[p.pos]
	}
	return 0
}

// at returns the byte at pos+n, or 0 past EOF.
func (p *parser) at(n int) byte {
	if p.pos+n < len(p.src) {
		return p.src[p.pos+n]
	}
	return 0
}

// eof reports whether the scanner is at end of input.
func (p *parser) eof() bool { return p.pos >= len(p.src) }

// skipInlineWS advances over spaces and tabs only.
func (p *parser) skipInlineWS() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

// skipComment consumes a `#`…EOL comment if one starts here, validating that the
// comment body holds no raw control characters (TOML forbids them).
func (p *parser) skipComment() error {
	if p.peek() != '#' {
		return nil
	}
	for p.pos < len(p.src) && p.src[p.pos] != '\n' {
		c := p.src[p.pos]
		// Tab and a CRLF's carriage return are the only control bytes allowed in a
		// comment; any other (NUL, DEL, etc.) is rejected.
		if c != '\t' && c != '\r' && c < 0x20 || c == 0x7f {
			return p.errAt(p.pos, "control character in comment")
		}
		p.pos++
	}
	return nil
}

// skipBlankAndComments advances over whitespace, comments and newlines between
// logical lines.
func (p *parser) skipBlankAndComments() error {
	for {
		p.skipInlineWS()
		switch {
		case p.peek() == '#':
			if err := p.skipComment(); err != nil {
				return err
			}
		case p.peek() == '\r' && p.at(1) == '\n':
			p.pos += 2
		case p.peek() == '\n':
			p.pos++
		default:
			return nil
		}
	}
}

// endOfLine consumes trailing inline whitespace, an optional comment, and the
// line terminator (or EOF). A non-terminator byte is a syntax error.
func (p *parser) endOfLine() error {
	p.skipInlineWS()
	if err := p.skipComment(); err != nil {
		return err
	}
	switch {
	case p.eof():
		return nil
	case p.peek() == '\n':
		p.pos++
		return nil
	case p.peek() == '\r' && p.at(1) == '\n':
		p.pos += 2
		return nil
	default:
		return p.errAt(p.pos, "expected newline after statement")
	}
}

// parseDocument is the top-level line loop.
func (p *parser) parseDocument() error {
	for {
		if err := p.skipBlankAndComments(); err != nil {
			return err
		}
		if p.eof() {
			return nil
		}
		if p.peek() == '[' {
			if err := p.parseTableHeader(); err != nil {
				return err
			}
			continue
		}
		if err := p.parseKeyValue(p.cur); err != nil {
			return err
		}
		if err := p.endOfLine(); err != nil {
			return err
		}
	}
}

// parseTableHeader handles `[a.b]` and `[[a.b]]` lines, repositioning p.cur.
func (p *parser) parseTableHeader() error {
	array := false
	p.pos++ // consume first '['
	if p.peek() == '[' {
		array = true
		p.pos++
	}
	p.skipInlineWS()
	keys, err := p.parseKeyPath()
	if err != nil {
		return err
	}
	p.skipInlineWS()
	if p.peek() != ']' {
		return p.errAt(p.pos, "expected ']' closing table header")
	}
	p.pos++
	if array {
		if p.peek() != ']' {
			return p.errAt(p.pos, "expected ']]' closing array-of-tables header")
		}
		p.pos++
	}
	if array {
		if err := p.openArrayTable(keys); err != nil {
			return err
		}
	} else {
		if err := p.openTable(keys); err != nil {
			return err
		}
	}
	return p.endOfLine()
}

// openTable walks/creates the path for a `[a.b.c]` header and makes the final
// table current, enforcing toml-rb's redefinition rules.
func (p *parser) openTable(keys []string) error {
	m := p.root
	for i, k := range keys {
		last := i == len(keys)-1
		existing, ok := m.Get(k)
		if !ok {
			child := NewMap()
			child.fromDotted = false
			if last {
				child.explicit = true
			}
			m.Set(k, child)
			m = child
			continue
		}
		switch ev := existing.(type) {
		case *Map:
			if last {
				// Re-opening an already-explicit, inline, or dotted-born table is an error.
				if ev.explicit || ev.inline || ev.fromDotted {
					return &OverwriteError{Key: k}
				}
				ev.explicit = true
			}
			m = ev
		case []any:
			// Descend into the last element of an array-of-tables.
			if len(ev) == 0 {
				return &OverwriteError{Key: k}
			}
			tbl, ok := ev[len(ev)-1].(*Map)
			if !ok {
				return &OverwriteError{Key: k}
			}
			if last {
				return &OverwriteError{Key: k}
			}
			m = tbl
		default:
			return &OverwriteError{Key: k}
		}
	}
	p.cur = m
	return nil
}

// openArrayTable handles a `[[a.b]]` header: it walks to the parent table, then
// appends a fresh element to the array at the final key.
func (p *parser) openArrayTable(keys []string) error {
	m := p.root
	// Descend through (and create) the intermediate tables.
	for _, k := range keys[:len(keys)-1] {
		existing, ok := m.Get(k)
		if !ok {
			child := NewMap()
			m.Set(k, child)
			m = child
			continue
		}
		next, err := descendInto(existing, k)
		if err != nil {
			return err
		}
		m = next
	}
	// Append a fresh element to the array at the final key.
	k := keys[len(keys)-1]
	existing, ok := m.Get(k)
	if !ok {
		elem := NewMap()
		elem.explicit = true
		m.Set(k, []any{elem})
		p.cur = elem
		return nil
	}
	arr, ok := existing.([]any)
	if !ok {
		return &OverwriteError{Key: k}
	}
	elem := NewMap()
	elem.explicit = true
	m.Set(k, append(arr, elem))
	p.cur = elem
	return nil
}

// descendInto returns the table a header path steps into for an existing value:
// a plain (non-inline) table directly, or the last element of an array-of-tables.
// Any other shape (scalar, inline table, array of non-tables) is a redefinition.
func descendInto(existing Value, k string) (*Map, error) {
	switch ev := existing.(type) {
	case *Map:
		if ev.inline {
			return nil, &OverwriteError{Key: k}
		}
		return ev, nil
	case []any:
		if len(ev) == 0 {
			return nil, &OverwriteError{Key: k}
		}
		tbl, ok := ev[len(ev)-1].(*Map)
		if !ok {
			return nil, &OverwriteError{Key: k}
		}
		return tbl, nil
	default:
		return nil, &OverwriteError{Key: k}
	}
}

// parseKeyValue parses `key = value` into dst, honouring dotted keys.
func (p *parser) parseKeyValue(dst *Map) error {
	keys, err := p.parseKeyPath()
	if err != nil {
		return err
	}
	p.skipInlineWS()
	if p.peek() != '=' {
		return p.errAt(p.pos, "expected '=' after key")
	}
	p.pos++
	p.skipInlineWS()
	val, err := p.parseValue()
	if err != nil {
		return err
	}
	return p.assign(dst, keys, val)
}

// assign places val at the dotted key path within dst, creating intermediate
// tables and enforcing the overwrite rules.
func (p *parser) assign(dst *Map, keys []string, val Value) error {
	m := dst
	// Walk (and create) the intermediate tables for all but the final component.
	for _, k := range keys[:len(keys)-1] {
		existing, ok := m.Get(k)
		if !ok {
			child := NewMap()
			child.fromDotted = true
			m.Set(k, child)
			m = child
			continue
		}
		tbl, ok := existing.(*Map)
		if !ok {
			return &OverwriteError{Key: k}
		}
		// A dotted key may only extend a table it (or a sibling dotted key) created,
		// never one frozen by inline `{ … }` syntax.
		if tbl.inline {
			return &OverwriteError{Key: k}
		}
		m = tbl
	}
	// Place the value at the final component, rejecting a redefinition.
	last := keys[len(keys)-1]
	if _, ok := m.Get(last); ok {
		return &OverwriteError{Key: last}
	}
	m.Set(last, val)
	return nil
}

// parseKeyPath parses a dotted key path (`a.b."c d"`), returning each component
// decoded.
func (p *parser) parseKeyPath() ([]string, error) {
	var keys []string
	for {
		p.skipInlineWS()
		k, err := p.parseKeyComponent()
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
		p.skipInlineWS()
		if p.peek() == '.' {
			p.pos++
			continue
		}
		return keys, nil
	}
}

// parseKeyComponent parses one bare/quoted key segment.
func (p *parser) parseKeyComponent() (string, error) {
	c := p.peek()
	switch {
	case c == '"':
		return p.parseBasicString()
	case c == '\'':
		return p.parseLiteralString()
	case isBareKeyChar(c):
		start := p.pos
		for p.pos < len(p.src) && isBareKeyChar(p.src[p.pos]) {
			p.pos++
		}
		return p.src[start:p.pos], nil
	default:
		return "", p.errAt(p.pos, "expected key")
	}
}

// isBareKeyChar reports whether c may appear in a bare key (A-Za-z0-9_-).
func isBareKeyChar(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' ||
		c >= '0' && c <= '9' || c == '_' || c == '-'
}

// parseValue dispatches on the first byte of a value.
func (p *parser) parseValue() (Value, error) {
	if p.eof() {
		return nil, p.errAt(p.pos, "expected value")
	}
	switch c := p.peek(); {
	case c == '"':
		if p.at(1) == '"' && p.at(2) == '"' {
			return p.parseMultilineBasicString()
		}
		return p.parseBasicString()
	case c == '\'':
		if p.at(1) == '\'' && p.at(2) == '\'' {
			return p.parseMultilineLiteralString()
		}
		return p.parseLiteralString()
	case c == '[':
		return p.parseArray()
	case c == '{':
		return p.parseInlineTable()
	case c == 't' || c == 'f':
		return p.parseBool()
	default:
		return p.parseNumberOrDate()
	}
}

// parseBool parses `true` / `false`.
func (p *parser) parseBool() (Value, error) {
	if strings.HasPrefix(p.src[p.pos:], "true") {
		p.pos += 4
		return true, nil
	}
	if strings.HasPrefix(p.src[p.pos:], "false") {
		p.pos += 5
		return false, nil
	}
	return nil, p.errAt(p.pos, "invalid value")
}

// parseArray parses `[ v, v, … ]`, allowing newlines, comments, and a trailing
// comma between elements.
func (p *parser) parseArray() (Value, error) {
	p.pos++ // '['
	arr := []any{}
	for {
		if err := p.skipArraySpace(); err != nil {
			return nil, err
		}
		if p.peek() == ']' {
			p.pos++
			return arr, nil
		}
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
		if err := p.skipArraySpace(); err != nil {
			return nil, err
		}
		switch p.peek() {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return arr, nil
		default:
			return nil, p.errAt(p.pos, "expected ',' or ']' in array")
		}
	}
}

// skipArraySpace skips whitespace, newlines and comments inside an array.
func (p *parser) skipArraySpace() error {
	for {
		p.skipInlineWS()
		switch {
		case p.peek() == '#':
			if err := p.skipComment(); err != nil {
				return err
			}
		case p.peek() == '\r' && p.at(1) == '\n':
			p.pos += 2
		case p.peek() == '\n':
			p.pos++
		case p.eof():
			return p.errAt(p.pos, "unterminated array")
		default:
			return nil
		}
	}
}

// parseInlineTable parses `{ k = v, … }`. Inline tables forbid newlines and a
// trailing comma per TOML v1.0.0.
func (p *parser) parseInlineTable() (Value, error) {
	p.pos++ // '{'
	m := NewMap()
	m.inline = true
	p.skipInlineWS()
	if p.peek() == '}' {
		p.pos++
		return m, nil
	}
	for {
		p.skipInlineWS()
		if err := p.parseKeyValue(m); err != nil {
			return nil, err
		}
		p.skipInlineWS()
		switch p.peek() {
		case ',':
			p.pos++
		case '}':
			p.pos++
			return m, nil
		default:
			return nil, p.errAt(p.pos, "expected ',' or '}' in inline table")
		}
	}
}

// parseNumberOrDate parses integers, floats, and the four date/time shapes,
// disambiguating by scanning the value token first.
func (p *parser) parseNumberOrDate() (Value, error) {
	start := p.pos
	tok := p.scanValueToken()
	if tok == "" {
		return nil, p.errAt(start, "expected value")
	}
	if v, ok, err := parseDateTimeToken(tok); err != nil {
		return nil, p.errAt(start, err.Error())
	} else if ok {
		p.pos = start + len(tok)
		return v, nil
	}
	v, err := parseNumberToken(tok)
	if err != nil {
		return nil, p.errAt(start, err.Error())
	}
	p.pos = start + len(tok)
	return v, nil
}

// scanValueToken reads the run of bytes that can form a number or date token,
// stopping at a value/structure delimiter. A date-time may contain a single
// internal space (`1979-05-27 07:32:00`) which scanValueToken keeps.
func (p *parser) scanValueToken() string {
	start := p.pos
	i := p.pos
	for i < len(p.src) {
		c := p.src[i]
		if c == ' ' {
			// Keep one space only when it joins a date to a time of day.
			if looksLikeDateSpace(p.src, start, i) {
				i++
				continue
			}
			break
		}
		if c == '\t' || c == '\n' || c == '\r' || c == ',' ||
			c == ']' || c == '}' || c == '#' {
			break
		}
		i++
	}
	return p.src[start:i]
}

// looksLikeDateSpace reports whether the space at index sp separates a TOML date
// from its time-of-day (the only place a space may sit inside a value token).
func looksLikeDateSpace(src string, start, sp int) bool {
	left := src[start:sp]
	if len(left) != 10 || left[4] != '-' || left[7] != '-' {
		return false
	}
	// The byte after the space must begin a time (a digit).
	return sp+1 < len(src) && src[sp+1] >= '0' && src[sp+1] <= '9'
}

// parseNumberToken decodes an integer or float token (no date forms).
func parseNumberToken(tok string) (Value, error) {
	// Special floats.
	switch tok {
	case "inf", "+inf":
		return inf(1), nil
	case "-inf":
		return inf(-1), nil
	case "nan", "+nan", "-nan":
		return nan(), nil
	}
	if isFloatToken(tok) {
		return parseFloatToken(tok)
	}
	return parseIntToken(tok)
}

// isFloatToken reports whether a non-date numeric token is a float (carries a
// fractional part or a decimal exponent) rather than an integer. Hex/oct/bin
// literals never count as floats.
func isFloatToken(tok string) bool {
	if strings.HasPrefix(tok, "0x") || strings.HasPrefix(tok, "0o") ||
		strings.HasPrefix(tok, "0b") {
		return false
	}
	return strings.ContainsAny(tok, ".") ||
		strings.ContainsAny(tok, "eE")
}

// parseIntToken decodes a decimal/hex/octal/binary integer with `_` separators.
func parseIntToken(tok string) (Value, error) {
	base := 10
	neg := false
	body := tok
	switch {
	case strings.HasPrefix(body, "0x"):
		base, body = 16, body[2:]
	case strings.HasPrefix(body, "0o"):
		base, body = 8, body[2:]
	case strings.HasPrefix(body, "0b"):
		base, body = 2, body[2:]
	default:
		if strings.HasPrefix(body, "+") {
			body = body[1:]
		} else if strings.HasPrefix(body, "-") {
			neg = true
			body = body[1:]
		}
	}
	digits, err := stripUnderscores(body, base)
	if err != nil {
		return nil, err
	}
	if base == 10 {
		if err := checkDecLeadingZero(digits); err != nil {
			return nil, err
		}
	}
	n, err := strconv.ParseInt(digits, base, 64)
	if err != nil {
		return nil, errInvalid("integer")
	}
	if neg {
		n = -n
	}
	return n, nil
}

// checkDecLeadingZero rejects a decimal integer with a redundant leading zero
// (TOML v1.0.0 forbids `01`), keeping a bare `0` valid.
func checkDecLeadingZero(digits string) error {
	if len(digits) > 1 && digits[0] == '0' {
		return errInvalid("integer")
	}
	return nil
}

// stripUnderscores removes the `_` digit separators, requiring each `_` to sit
// between two valid digits for the base (so `1__2`, `_1`, `1_` all fail).
func stripUnderscores(s string, base int) (string, error) {
	if s == "" {
		return "", errInvalid("number")
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' {
			if i == 0 || i == len(s)-1 ||
				!isBaseDigit(s[i-1], base) || !isBaseDigit(s[i+1], base) {
				return "", errInvalid("number")
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String(), nil
}

// isBaseDigit reports whether c is a valid digit (or sign for base-10 leading
// position handling) in the given base.
func isBaseDigit(c byte, base int) bool {
	switch base {
	case 16:
		return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
	case 8:
		return c >= '0' && c <= '7'
	case 2:
		return c == '0' || c == '1'
	default:
		return c >= '0' && c <= '9'
	}
}

// parseFloatToken decodes a decimal float with optional `_` separators and
// exponent, validating the digit grouping TOML requires.
func parseFloatToken(tok string) (Value, error) {
	body := tok
	if strings.HasPrefix(body, "+") || strings.HasPrefix(body, "-") {
		body = body[1:]
	}
	// A float must start and end with a digit (TOML forbids `.7` and `7.`).
	if body == "" || !isDigit(body[0]) || !isDigit(body[len(body)-1]) {
		// Exponent-terminated floats like `1e10` end in a digit already; the only
		// trailing-non-digit cases are the forbidden `7.` form.
		return nil, errInvalid("float")
	}
	// Validate underscore placement across the mantissa+exponent, then strip them.
	clean, err := stripFloatUnderscores(body)
	if err != nil {
		return nil, err
	}
	// Reject a leading zero in the integer part (`07.0`) the way TOML does, while
	// allowing `0.x`, `0e…`.
	if err := checkFloatLeadingZero(clean); err != nil {
		return nil, err
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(tok, "_", ""), 64)
	if err != nil {
		return nil, errInvalid("float")
	}
	return f, nil
}

// stripFloatUnderscores validates `_` separators inside a float body (each `_`
// between digits) and returns the body with them removed.
func stripFloatUnderscores(body string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '_' {
			if i == 0 || i == len(body)-1 || !isDigit(body[i-1]) || !isDigit(body[i+1]) {
				return "", errInvalid("float")
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String(), nil
}

// checkFloatLeadingZero rejects a redundant leading zero in a float's integer
// part (`07.5`) while keeping `0.5` and `0e2`.
func checkFloatLeadingZero(clean string) error {
	if len(clean) > 1 && clean[0] == '0' && isDigit(clean[1]) {
		return errInvalid("float")
	}
	return nil
}

// isDigit reports whether c is an ASCII decimal digit.
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// parseDateTimeToken recognises the four RFC 3339 date/time shapes. It returns
// (value, true, nil) on a match, (nil, false, nil) for a non-date token, and an
// error for a malformed date.
func parseDateTimeToken(tok string) (Value, bool, error) {
	// Local time: HH:MM:SS[.fff]
	if len(tok) >= 8 && tok[2] == ':' && tok[5] == ':' && isDigit(tok[0]) {
		lt, err := parseLocalTime(tok)
		if err != nil {
			return nil, true, err
		}
		return lt, true, nil
	}
	// Anything beginning YYYY-MM-DD.
	if len(tok) >= 10 && tok[4] == '-' && tok[7] == '-' &&
		isDigit(tok[0]) && isDigit(tok[1]) && isDigit(tok[2]) && isDigit(tok[3]) {
		v, err := parseDateLeading(tok)
		if err != nil {
			return nil, true, err
		}
		return v, true, nil
	}
	return nil, false, nil
}

// parseLocalTime decodes an HH:MM:SS[.fff] token.
func parseLocalTime(tok string) (LocalTime, error) {
	h, mi, s, ns, err := splitClock(tok)
	if err != nil {
		return LocalTime{}, err
	}
	return LocalTime{Hour: h, Minute: mi, Second: s, Nanosecond: ns}, nil
}

// parseDateLeading decodes a token beginning with a date: a bare local date, or a
// date-time joined by `T`/`t`/space, with or without an offset.
func parseDateLeading(tok string) (Value, error) {
	y, mo, d, err := splitDate(tok[:10])
	if err != nil {
		return nil, err
	}
	if len(tok) == 10 {
		return LocalDate{Year: y, Month: mo, Day: d}, nil
	}
	sep := tok[10]
	if sep != 'T' && sep != 't' && sep != ' ' {
		return nil, errInvalid("datetime")
	}
	rest := tok[11:]
	// Find an offset suffix: trailing Z/z or a ±hh:mm starting after the seconds.
	timePart, off, hasOff := splitOffset(rest)
	h, mi, s, ns, err := splitClock(timePart)
	if err != nil {
		return nil, err
	}
	if !hasOff {
		return LocalDateTime{
			Year: y, Month: mo, Day: d,
			Hour: h, Minute: mi, Second: s, Nanosecond: ns,
		}, nil
	}
	loc := off
	t := time.Date(y, time.Month(mo), d, h, mi, s, ns, loc)
	return OffsetDateTime{Time: t}, nil
}

// splitOffset separates an RFC 3339 time-of-day from its optional zone suffix,
// returning the bare clock string, the *time.Location for the zone, and whether
// an offset was present.
func splitOffset(rest string) (string, *time.Location, bool) {
	if rest == "" {
		return rest, nil, false
	}
	last := rest[len(rest)-1]
	if last == 'Z' || last == 'z' {
		return rest[:len(rest)-1], time.UTC, true
	}
	// Look for ±hh:mm at the tail (a sign preceded by digits, 6 chars).
	if len(rest) >= 6 {
		cand := rest[len(rest)-6:]
		if (cand[0] == '+' || cand[0] == '-') && cand[3] == ':' &&
			isDigit(cand[1]) && isDigit(cand[2]) && isDigit(cand[4]) && isDigit(cand[5]) {
			oh := int(cand[1]-'0')*10 + int(cand[2]-'0')
			om := int(cand[4]-'0')*10 + int(cand[5]-'0')
			secs := (oh*60 + om) * 60
			if cand[0] == '-' {
				secs = -secs
			}
			return rest[:len(rest)-6], time.FixedZone("", secs), true
		}
	}
	return rest, nil, false
}

// splitDate decodes a YYYY-MM-DD string, range-checking month and day. The caller
// (parseDateTimeToken) has already confirmed the `-` separators and length.
func splitDate(s string) (y, mo, d int, err error) {
	if !allDigits(s[0:4]) || !allDigits(s[5:7]) || !allDigits(s[8:10]) {
		return 0, 0, 0, errInvalid("date")
	}
	y = atoi(s[0:4])
	mo = atoi(s[5:7])
	d = atoi(s[8:10])
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return 0, 0, 0, errInvalid("date")
	}
	return y, mo, d, nil
}

// splitClock decodes HH:MM:SS with an optional fractional-second part, returning
// the components and the nanoseconds.
func splitClock(s string) (h, mi, sec, ns int, err error) {
	if len(s) < 8 || s[2] != ':' || s[5] != ':' {
		return 0, 0, 0, 0, errInvalid("time")
	}
	if !allDigits(s[0:2]) || !allDigits(s[3:5]) || !allDigits(s[6:8]) {
		return 0, 0, 0, 0, errInvalid("time")
	}
	h = atoi(s[0:2])
	mi = atoi(s[3:5])
	sec = atoi(s[6:8])
	if h > 23 || mi > 59 || sec > 60 { // 60 permits a leap second
		return 0, 0, 0, 0, errInvalid("time")
	}
	if len(s) > 8 {
		if s[8] != '.' {
			return 0, 0, 0, 0, errInvalid("time")
		}
		frac := s[9:]
		if frac == "" || !allDigits(frac) {
			return 0, 0, 0, 0, errInvalid("time")
		}
		ns = fracToNanos(frac)
	}
	return h, mi, sec, ns, nil
}

// fracToNanos turns a fractional-second digit string into nanoseconds, truncating
// beyond nanosecond precision.
func fracToNanos(frac string) int {
	if len(frac) > 9 {
		frac = frac[:9]
	}
	n := atoi(frac)
	for i := len(frac); i < 9; i++ {
		n *= 10
	}
	return n
}

// allDigits reports whether s (always non-empty at its call sites) is entirely
// ASCII digits.
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

// atoi converts an all-digit string to int (no error path; callers pre-validate).
func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// ---- string scalars ----

// parseBasicString parses a `"…"` single-line basic string with escapes.
func (p *parser) parseBasicString() (string, error) {
	p.pos++ // opening quote
	var b strings.Builder
	for {
		if p.eof() {
			return "", p.errAt(p.pos, "unterminated string")
		}
		c := p.src[p.pos]
		switch {
		case c == '"':
			p.pos++
			return b.String(), nil
		case c == '\n':
			return "", p.errAt(p.pos, "newline in basic string")
		case c == '\\':
			r, err := p.parseEscape(false)
			if err != nil {
				return "", err
			}
			b.WriteString(r)
		default:
			b.WriteByte(c)
			p.pos++
		}
	}
}

// parseLiteralString parses a `'…'` single-line literal string (no escapes).
func (p *parser) parseLiteralString() (string, error) {
	p.pos++ // opening quote
	start := p.pos
	for {
		if p.eof() {
			return "", p.errAt(p.pos, "unterminated literal string")
		}
		c := p.src[p.pos]
		if c == '\'' {
			s := p.src[start:p.pos]
			p.pos++
			return s, nil
		}
		if c == '\n' {
			return "", p.errAt(p.pos, "newline in literal string")
		}
		p.pos++
	}
}

// parseMultilineBasicString parses `"""…"""` with escapes and the line-ending
// backslash trim.
func (p *parser) parseMultilineBasicString() (string, error) {
	p.pos += 3
	p.trimOpeningNewline()
	var b strings.Builder
	for {
		if p.eof() {
			return "", p.errAt(p.pos, "unterminated multiline string")
		}
		if p.peek() == '"' && p.at(1) == '"' && p.at(2) == '"' {
			// The closing delimiter is the final `"""` of the run; any quotes before
			// it are literal content (toml-rb consumes the whole run this way, so
			// `"""""""""` yields `"""`).
			run := 0
			for p.at(run) == '"' {
				run++
			}
			for i := 0; i < run-3; i++ {
				b.WriteByte('"')
			}
			p.pos += run
			return b.String(), nil
		}
		c := p.src[p.pos]
		switch {
		case c == '\\':
			if p.isLineEndingBackslash() {
				p.pos++ // consume backslash
				p.trimWhitespaceAndNewlines()
				continue
			}
			r, err := p.parseEscape(true)
			if err != nil {
				return "", err
			}
			b.WriteString(r)
		case c == '\r' && p.at(1) == '\n':
			b.WriteString("\r\n")
			p.pos += 2
		default:
			b.WriteByte(c)
			p.pos++
		}
	}
}

// parseMultilineLiteralString parses `”'…”'` (no escapes, opening-newline trim).
func (p *parser) parseMultilineLiteralString() (string, error) {
	p.pos += 3
	p.trimOpeningNewline()
	start := p.pos
	for {
		if p.eof() {
			return "", p.errAt(p.pos, "unterminated multiline literal string")
		}
		if p.peek() == '\'' && p.at(1) == '\'' && p.at(2) == '\'' {
			s := p.src[start:p.pos]
			p.pos += 3
			return s, nil
		}
		p.pos++
	}
}

// trimOpeningNewline drops a newline immediately after the opening `"""`/`”'`.
func (p *parser) trimOpeningNewline() {
	if p.peek() == '\r' && p.at(1) == '\n' {
		p.pos += 2
	} else if p.peek() == '\n' {
		p.pos++
	}
}

// isLineEndingBackslash reports whether the backslash at pos is followed only by
// inline whitespace then a newline (the multiline line-continuation form).
func (p *parser) isLineEndingBackslash() bool {
	i := p.pos + 1
	for i < len(p.src) && (p.src[i] == ' ' || p.src[i] == '\t' || p.src[i] == '\r') {
		i++
	}
	return i < len(p.src) && p.src[i] == '\n'
}

// trimWhitespaceAndNewlines consumes the run of whitespace and newlines that a
// line-ending backslash elides.
func (p *parser) trimWhitespaceAndNewlines() {
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			p.pos++
			continue
		}
		return
	}
}

// parseEscape decodes a backslash escape. In multiline strings the line-ending
// backslash is handled by the caller; here multiline only relaxes nothing extra.
func (p *parser) parseEscape(multiline bool) (string, error) {
	_ = multiline
	if p.pos+1 >= len(p.src) {
		return "", p.errAt(p.pos, "dangling escape")
	}
	e := p.src[p.pos+1]
	switch e {
	case 'n':
		p.pos += 2
		return "\n", nil
	case 't':
		p.pos += 2
		return "\t", nil
	case 'r':
		p.pos += 2
		return "\r", nil
	case '"':
		p.pos += 2
		return "\"", nil
	case '\\':
		p.pos += 2
		return "\\", nil
	case 'b':
		p.pos += 2
		return "\b", nil
	case 'f':
		p.pos += 2
		return "\f", nil
	case 'e': // toml-rb supports \e (ESC)
		p.pos += 2
		return "\x1b", nil
	case 'u':
		return p.parseUnicodeEscape(4)
	case 'U':
		return p.parseUnicodeEscape(8)
	case 'x': // toml-rb / TOML 1.1 hex byte escape
		return p.parseHexEscape()
	default:
		return "", p.errAt(p.pos, "invalid escape \\"+string(e))
	}
}

// parseUnicodeEscape decodes \uXXXX or \UXXXXXXXX into UTF-8.
func (p *parser) parseUnicodeEscape(n int) (string, error) {
	start := p.pos
	p.pos += 2 // skip \u / \U
	if p.pos+n > len(p.src) {
		return "", p.errAt(start, "short unicode escape")
	}
	hex := p.src[p.pos : p.pos+n]
	cp, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return "", p.errAt(start, "invalid unicode escape")
	}
	if cp > 0x10FFFF || (cp >= 0xD800 && cp <= 0xDFFF) {
		return "", p.errAt(start, "invalid unicode scalar")
	}
	p.pos += n
	return string(rune(cp)), nil
}

// parseHexEscape decodes \xHH into a single byte (toml-rb extension).
func (p *parser) parseHexEscape() (string, error) {
	start := p.pos
	p.pos += 2 // skip \x
	if p.pos+2 > len(p.src) {
		return "", p.errAt(start, "short hex escape")
	}
	hex := p.src[p.pos : p.pos+2]
	b, err := strconv.ParseUint(hex, 16, 8)
	if err != nil {
		return "", p.errAt(start, "invalid hex escape")
	}
	p.pos += 2
	return string(rune(b)), nil
}

// ---- helpers ----

// errInvalid builds a simple "invalid <kind>" parse-error message.
func errInvalid(kind string) error { return &valueError{kind} }

// valueError carries a value-format failure surfaced through parseNumberOrDate.
type valueError struct{ kind string }

func (e *valueError) Error() string { return "invalid " + e.kind }
