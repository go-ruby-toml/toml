# toml examples

Runnable pure-Ruby usage of the `toml` (toml-rb-faithful) TOML v1.0.0 parser and generator, verified under the [rbgo](https://github.com/go-embedded-ruby) interpreter.

```sh
rbgo examples/toml_usage.rb
```

| File | Shows |
| --- | --- |
| `toml_usage.rb` | Parse a document with `TOML.parse` into a String-keyed Hash; read scalars, a nested `[table]`, and an array-of-tables `[[servers]]`; use the `TomlRB` alias; serialise a Hash back with `TOML.dump`; and rescue the `ArgumentError` raised on invalid syntax. |
