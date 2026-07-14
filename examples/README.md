# tabsmith examples

This directory contains `shipctl`, a deliberately completion-less demo
CLI (a plain POSIX sh script): a root command with global flags and three
subcommands, one nested a level deeper. Every level answers `--help`,
which is all tabsmith needs. Everything runs offline.

## 1. Probe a binary and generate all three shells

```bash
cd examples
tabsmith gen --out completions ./shipctl
ls completions
# _shipctl  shipctl.bash  shipctl.fish
```

tabsmith ran `./shipctl --help`, saw `deploy`, `logs` and `status` in the
Commands section, probed each with `--help`, found `deploy history`
nested below, and emitted one script per shell.

## 2. Try the completions

```bash
source completions/shipctl.bash
shipctl dep<Tab>                    # → deploy
shipctl deploy --strategy <Tab>     # → bluegreen canary rolling
shipctl deploy history --<Tab>      # → --json --limit
```

The strategy values were mined from the phrase
`one of: rolling, canary, bluegreen` in the help text; `--env` got its
values from clap-style `[possible values: staging, prod]`; `--manifest
FILE` completes real files.

## 3. Look at what the parser saw

```bash
tabsmith inspect ./shipctl | less
```

The JSON tree is the exact input the generators work from — when a
completion looks wrong, this shows whether the parser or the generator
is to blame.

## 4. Work from saved help text (no binary needed)

```bash
./shipctl --help > shipctl-help.txt
tabsmith gen --from shipctl-help.txt --name shipctl --shell fish
```

`--from` never executes anything, so it also works on help text copied
out of documentation for a tool you have not installed. Only the root
level is available this way — probing subcommands needs the binary.
