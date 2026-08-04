<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-toml/brand/main/social/go-ruby-toml-toml.png" alt="go-ruby-toml/toml" width="720"></p>

# toml — go-ruby-toml

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-toml.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)
[![toml-test](https://img.shields.io/badge/toml--test-708%2F709%20(99.86%25)-1a7f37)](#conformance-notes)

**A pure-Go (no cgo) TOML v1.0.0 parser and generator for the Ruby value model**,
faithful to the [`toml-rb`](https://github.com/emancu/toml-rb) gem. `Parse` turns a
TOML document into a Ruby `Hash` (an insertion-ordered `*Map`); `Dump` renders such
a value back to a TOML string — so it is the deterministic, interpreter-independent
core of `TomlRB.parse` / `TomlRB.dump`, **without any Ruby runtime**.

It is the TOML backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module with no dependency on the Ruby runtime — a sibling
of [go-ruby-yaml](https://github.com/go-ruby-yaml/yaml) (the Psych engine),
[go-ruby-regexp](https://github.com/go-ruby-regexp/regexp) (the Onigmo engine) and
[go-ruby-erb](https://github.com/go-ruby-erb/erb) (the ERB compiler).

> **What it is — and isn't.** Lexing and parsing TOML (the bare/quoted/dotted key
> grammar, the four string styles, integers/floats, the RFC 3339 date/time shapes,
> arrays, inline tables, `[table]` and `[[array of tables]]` headers, and the
> redefinition rules) is fully deterministic and needs **no interpreter**, so it
> lives here as pure Go. Binding the result to live Ruby objects — materialising a
> `Time` in the host's zone, wrapping a `Hash` — is the host's job; this library
> hands back a small, explicit value model the host maps to and from its own
> objects.

## Features

Faithful port of `toml-rb`'s parse + dump, validated against the gem on every
supported platform:

- **Keys** — bare (`A-Za-z0-9_-`), basic-quoted, literal-quoted, and **dotted**
  (`a.b.c`), in key/value lines, table headers, and inline tables.
- **Strings** — basic (`"…"`), literal (`'…'`), multiline basic (`"""…"""`) and
  multiline literal (`'''…'''`), with the `\n \t \r \" \\ \b \f \e` escapes,
  `\uXXXX` / `\UXXXXXXXX` and the `\xHH` byte escape, the opening-newline trim, and
  the line-ending-backslash whitespace trim.
- **Numbers** — decimal / `0x` hex / `0o` octal / `0b` binary integers with `_`
  separators (leading-zero and bad-separator rejection), and floats with
  fractions, exponents, and `inf` / `-inf` / `nan`.
- **Date/time** — offset date-time, local date-time, local date, and local time
  (RFC 3339), kept as an explicit value model (`OffsetDateTime`, `LocalDateTime`,
  `LocalDate`, `LocalTime`) the host maps to `Time` / `Date`.
- **Arrays** — mixed-type, nested, with newlines, comments, and trailing commas.
- **Tables** — `[table]`, `[a.b.c]` super-tables (out-of-order allowed), inline
  `{ … }` tables, and `[[array of tables]]` (including nested), with the full
  **redefinition / overwrite error** rules (`TomlRB::ValueOverwriteError`).
- **Dump** — `Dump(*Map | map[string]any)` emits scalar pairs, then `[table]`
  sections, then `[[array]]` sections, recursively — round-tripping `Parse`.

CGO-free, dependency-free, **100% test coverage**, `gofmt` + `go vet` clean, and
green across the six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le,
s390x) on Linux, macOS, and Windows.

## Install

```sh
go get github.com/go-ruby-toml/toml
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/go-ruby-toml/toml"
)

func main() {
	doc, _ := toml.Parse(`
title = "TOML Example"

[server]
ip = "10.0.0.1"
ports = [8000, 8001]

[[product]]
name = "Hammer"
`)
	// Parse returns an insertion-ordered *toml.Map (a Ruby Hash).
	srv, _ := doc.Get("server")
	ip, _ := srv.(*toml.Map).Get("ip")
	fmt.Println(ip) // 10.0.0.1

	// Dump renders a value tree back to TOML (TomlRB.dump).
	m := toml.NewMap()
	m.Set("name", "weft")
	m.Set("ports", []any{80, 443})
	out, _ := toml.Dump(m)
	fmt.Print(out)
	// name = "weft"
	// ports = [80, 443]
}
```

## Ruby value model

`Parse` returns an `any` drawn from a small, fixed set of Go types, so a host can
map its own object graph to and from this package:

| TOML                  | Go (`Parse` returns)         | Ruby (`rbgo` maps to)        |
| --------------------- | ---------------------------- | ---------------------------- |
| string                | `string`                     | `String`                     |
| integer               | `int64`                      | `Integer`                    |
| float / `inf` / `nan` | `float64`                    | `Float`                      |
| boolean               | `bool`                       | `true` / `false`             |
| array                 | `[]any`                      | `Array`                      |
| table / inline table  | `*toml.Map` (ordered)        | `Hash`                       |
| offset date-time      | `toml.OffsetDateTime`        | `Time`                       |
| local date-time       | `toml.LocalDateTime`         | `Time` (host zone)           |
| local date            | `toml.LocalDate`             | `Time` at `00:00` (host zone)|
| local time            | `toml.LocalTime`             | `Time` on `1970-01-01`       |

`toml-rb` collapses every TOML date/time onto Ruby's `Time`, projecting the
offset-less variants onto the host's local zone. This package keeps the faithful
TOML distinction (local vs. offset, with no host-zone guessing) so the host applies
its zone exactly once when it materialises a Ruby `Time`. `Dump` also accepts Go's
`time.Time` and `map[string]any` (emitted in sorted-key order).

## API

```go
// Parse parses a TOML v1.0.0 document into a Ruby Hash (TomlRB.parse / TOML.parse).
func Parse(s string) (*Map, error)

// LoadFile reads and parses a file (TomlRB.load_file / TOML.load_file).
func LoadFile(path string) (*Map, error)

// Dump renders a *Map or map[string]any to a TOML string (TomlRB.dump).
func Dump(v any) (string, error)

type Map struct { /* insertion-ordered Hash */ }
func NewMap() *Map
func (m *Map) Set(key string, val any)
func (m *Map) Get(key string) (any, bool)
func (m *Map) Pairs() []Pair
func (m *Map) Len() int

type OffsetDateTime struct{ Time time.Time }
type LocalDateTime  struct{ Year, Month, Day, Hour, Minute, Second, Nanosecond int }
type LocalDate      struct{ Year, Month, Day int }
type LocalTime      struct{ Hour, Minute, Second, Nanosecond int }

type ParseError     struct{ Msg string; Line, Col, Offset int } // TomlRB::ParseError
type OverwriteError struct{ Key string }                        // TomlRB::ValueOverwriteError
```

## Conformance notes

Against the canonical [toml-lang/toml-test](https://github.com/toml-lang/toml-test)
corpus for **TOML v1.0.0**, this package resolves **708 of 709 cases (99.86%)** to
the same verdict as the reference comparator — all 210 valid cases parse and match,
and 498 of 499 invalid cases are rejected. Because the engine is **toml-rb-faithful**
rather than a strict-spec validator, this is its *faithful ceiling*, not a gap:

- The single case outside that count, `invalid/string/multiline-quotes-01`
  (`a = """6 quotes: """"""`), is one that toml-test marks invalid but that
  **`toml-rb` itself accepts** — its grammar consumes the extra closing quotes
  (`"""""""""` ⇒ `"""`). This library matches `toml-rb` here **by design**; doing
  otherwise would make it *less* faithful to the gem it ports. It is recorded as a
  known, intentional divergence, not a failure.
- Validated against `toml-rb` 4.x with a differential oracle too (a corpus and the
  canonical TOML spec example are parsed both here and by the gem, and the
  canonicalised results are compared); rejected documents are cross-checked.
- In the other direction, where `toml-rb` is laxer than the TOML v1.0.0 spec
  (e.g. it accepts a decimal integer with a redundant leading zero such as `01`),
  this library follows the **spec** and rejects it; those cases are skipped in the
  gem oracle.

## Tests & coverage

The suite pairs deterministic, ruby-free tests (which alone hold coverage at 100%,
so the qemu cross-arch and Windows lanes pass the gate) with a **differential
`toml-rb` oracle** that runs on the ubuntu/macOS lanes (skipped where the gem is
absent). The oracle scripts `$stdout.binmode` so Windows text-mode never pollutes
the bytes.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-toml/toml authors.

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```
