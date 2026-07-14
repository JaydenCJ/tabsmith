# Contributing to tabsmith

Issues, discussions and pull requests are all welcome.

## Getting started

You need Go 1.22 or newer; there are no other dependencies of any kind.

```bash
git clone https://github.com/JaydenCJ/tabsmith.git
cd tabsmith
go build ./...
go test ./...
bash scripts/smoke.sh
```

`scripts/smoke.sh` builds the binary, probes the bundled demo CLI, generates
all three shell scripts and drives real bash completions against them; it
must finish by printing `SMOKE OK`.

## Before you open a pull request

1. `gofmt -l .` reports nothing (formatting is enforced).
2. `go vet ./...` passes with no findings.
3. `go test ./...` passes (all 91 tests).
4. `bash scripts/smoke.sh` prints `SMOKE OK`.
5. Add tests for behavior changes; keep logic in pure, unit-testable
   packages (`helptext`, `spec`, `gen`) rather than in the CLI layer.

## Ground rules

- Zero runtime dependencies is a core feature: the `go.mod` require list
  stays empty. Adding a dependency needs strong justification in the PR.
- tabsmith only ever runs the target tool with help-shaped arguments
  (`--help`, `-h`, `help`) — never a bare subcommand. Keep it that way.
- No network calls, ever. No telemetry.
- Parser rules must be conservative: a new heuristic needs a real help
  screen it fixes and evidence it does not misread prose as flags — a
  wrong completion is worse than a missing one.
- New help dialects come with a faithful miniature of the real output in
  the tests, so the shape being parsed stays visible in review.
- Code comments and doc comments are written in English.

## Reporting bugs

Please include the output of `tabsmith version`, the exact command line,
and the tool's raw help text (`thetool --help | cat`) — that text is the
whole input, so with it every parse bug reproduces offline via
`tabsmith inspect --from help.txt`.

## Security

Please do not open public issues for security problems; use GitHub's
private vulnerability reporting on this repository instead.
