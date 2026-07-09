# frozen_string_literal: true
#
# Usage of TOML — the pure-Go, toml-rb-faithful TOML v1.0.0 parser and
# generator added by `require "toml"`. The module answers to both TOML and
# TomlRB. Runs under go-embedded-ruby (rbgo); see examples/README.md.

require "toml"

doc = <<~TOML
  title   = "TOML Example"
  enabled = true
  port    = 8080

  [owner]
  name = "Tom"
  tags = ["a", "b", "c"]

  [[servers]]
  host = "alpha"

  [[servers]]
  host = "beta"
TOML

# Parse: a document string becomes a plain Ruby Hash with String keys.
data = TOML.parse(doc)
p data["title"]                       # => "TOML Example"
p data["enabled"]                     # => true
p data["port"]                        # => 8080

# Nested [table] and array-of-tables [[servers]] map to Hash and Array.
p data["owner"]["name"]               # => "Tom"
p data["owner"]["tags"]               # => ["a", "b", "c"]
p data["servers"].map { |s| s["host"] } # => ["alpha", "beta"]

# TomlRB is the same module under the toml-rb gem name.
p TomlRB.parse("x = 1")["x"]          # => 1

# Dump: a Ruby Hash serialises back to a TOML document (scalars, then tables).
puts TOML.dump({ "name" => "widget", "count" => 3, "opts" => { "verbose" => true } })
# => name = "widget"
#    count = 3
#    [opts]
#    verbose = true

# Invalid syntax raises an ArgumentError carrying the parser's message.
begin
  TOML.parse("x = = 1")
rescue ArgumentError
  puts "invalid TOML rejected"       # => invalid TOML rejected
end
