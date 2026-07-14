# Help-text dialects tabsmith understands

tabsmith has no grammar files: everything it knows comes from parsing the
help screens themselves. This document records the dialect features the
parser recognizes, with the shapes it keys on, so parser changes can be
reviewed against the inventory they must not break.

## Normalization (applies to every dialect)

Before any line is classified, the raw output is flattened:

| Input artifact | Source | Handling |
| --- | --- | --- |
| ANSI CSI sequences (colors) | clap 4, click ≥8.1, many Rust/Go tools | stripped |
| ANSI OSC sequences (hyperlinks) | clap 4 `--help` | stripped |
| Overstrike (`o\bo`, `_\bo`) | help piped through `man`/pagers | resolved |
| Tabs | BusyBox, old GNU tools | expanded at 8-column stops |
| CRLF, trailing spaces, BOM | Windows builds, sloppy heredocs | trimmed |

Column-accurate tab expansion matters: option/description splitting keys
on runs of **two or more spaces**, which only exist after expansion.

## Sections

A header is a shallow-indented `Title:` line (or ALL-CAPS `TITLE`).
Classification decides how the body is read:

| Header contains | Kind | Body treatment |
| --- | --- | --- |
| option, flag, switch | options | option lines parsed |
| command, subcommand | commands | `name  desc` rows become subcommands |
| positional, argument(s) | positionals | rows become positionals; `{a,b}` rows become subcommands (argparse subparsers) |
| example, environment, exit …, see also, notes | skip | ignored wholesale — example invocations are full of dashes that are not flags |
| anything else (`Miscellaneous:`, `Output control:`) | other | option lines still parsed (the git-help layout); command rows are **not** |

Terse tools with no headers at all still parse: option lines are honored
in every minable section, including before the first header.

## Option lines

One line, one flag. Recognized spellings and separators:

```text
-v, --verbose                  # comma
-p | --patch                   # pipe
-n/--dry-run                   # slash
-f, --force, --overwrite       # extra spellings become aliases
-json                          # Go flag package: old-style long
```

Value placeholders, in decreasing specificity:

```text
--output=FILE      --output FILE      --output <file>
--color[=WHEN]                        # optional, attached-only
--format {json,xml,table}             # argparse: also an enum
--when (auto|always|never)            # clap/git: also an enum
-min int                              # Go flag package type word
```

Descriptions start after two or more spaces, or on deeper-indented
continuation lines (git's layout); wrapped lines re-attach by indent.

## Enum and path mining

Choices are extracted from placeholders (`{a,b}`, `<a|b>`, `(a|b)`) and
from description prose:

- clap: `[possible values: full, diff, none]`
- hand-written: `one of: rolling, canary, bluegreen`
- GNU prose: `WHEN is 'always', 'never', or 'auto'`

Every candidate list is validated (short, spaceless, literal entries);
one bad entry discards the whole list, because a wrong enum hides valid
values from the user. `FILE`/`PATH` placeholders, `DIR`-shaped
placeholders and `--*-file` / `--*-dir` flag names map to the shell's
native file or directory completion.

## Probing behavior on real binaries

- Help spellings tried in order: `--help`, `-h`, `help`; stdout preferred,
  stderr accepted (getopt-era tools print usage there, often exiting 1).
- Output must be help-shaped (usage line, known header, or option lines);
  error banners are rejected.
- Subcommands are walked recursively (`tool sub --help`) to `--depth`
  levels under a total probe budget; every screen is fingerprinted so a
  tool that ignores unknown arguments and re-prints its root help yields
  a leaf, not an infinite tree.
- tabsmith never executes a bare subcommand — every probe argv ends in a
  help spelling — and each invocation runs under a hard timeout.

## Known limits (v0.1.0)

- `--flag=<Tab>` value completion inside the same word is not generated
  for bash (zsh and fish handle the attached form natively).
- Interspersed multi-line usage strings are recorded but not mined for
  positional structure.
- Man pages are only consumed via help output that happens to be one;
  `tabsmith gen --from <(man -P cat tool)` works when the OPTIONS section
  follows the shapes above.
