# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-07-13

### Added

- `tabsmith gen <tool>`: probe a binary's `--help` output (with `-h` and
  `help` fallbacks, stdout or stderr) and generate completion scripts for
  bash, zsh and fish — one shell to stdout, or all three files via `--out`.
- Recursive subcommand probing: every listed command is walked with
  `tool sub --help` down to `--depth` levels, capped by a probe budget and
  a screen fingerprint that detects tools which ignore unknown arguments.
- A help-text parser covering the GNU getopt, argparse, cobra, clap, click,
  BusyBox and Go `flag` dialects: sections, grouped options, wrapped
  descriptions, tab alignment, ANSI colors and man-style overstrike.
- Value intelligence: enum choices mined from `{a,b}` / `<a|b>` placeholders,
  clap's `[possible values: …]`, "one of: …" and quoted GNU prose; FILE/DIR
  placeholders and `--*-file`/`--*-dir` names map to native file completion.
- Optional-argument handling: `--color[=WHEN]` completes its values but
  never swallows the next word in any generated shell.
- Go-style single-dash long flags (`-json`, `-min int`) recognized and
  emitted with each shell's native old-option syntax.
- `tabsmith inspect <tool>`: the parsed command tree as pretty JSON.
- `--from <file|->` to generate from saved or piped help text, fully
  offline; `--name` overrides the completed tool name.
- Deterministic generators: byte-identical output for identical input, so
  generated completions can be committed and diffed.
- 91 deterministic offline tests (`go test ./...`) — including live bash
  completion runs — and an end-to-end `scripts/smoke.sh` that prints
  `SMOKE OK`.

[0.1.0]: https://github.com/JaydenCJ/tabsmith/releases/tag/v0.1.0
