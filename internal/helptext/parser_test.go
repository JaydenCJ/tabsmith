// Tests for whole-screen parsing: section handling, subcommand listings,
// wrapped descriptions and the dialect quirks of argparse, cobra, clap
// and click, each exercised on a faithful miniature of its real output.
package helptext

import (
	"reflect"
	"testing"
)

func TestParseUsageAndSummary(t *testing.T) {
	cmd := Parse("tar", "Usage: tar [OPTION...] [FILE]...\n\nGNU tar saves many files together.\n")
	if cmd.Usage != "tar [OPTION...] [FILE]..." {
		t.Fatalf("usage %q", cmd.Usage)
	}
	if cmd.Summary != "GNU tar saves many files together." {
		t.Fatalf("summary %q", cmd.Summary)
	}
}

func TestParseTerseHelpWithoutAnyHeaders(t *testing.T) {
	// Minimal hand-rolled helps list options with no section header at all.
	cmd := Parse("t", "usage: t [opts]\n  -v   verbose\n  -o FILE   output\n")
	if len(cmd.Flags) != 2 {
		t.Fatalf("want 2 flags, got %+v", cmd.Flags)
	}
	if cmd.Flags[1].Arg != "FILE" {
		t.Fatalf("got %+v", cmd.Flags[1])
	}
}

func TestParseCobraStyle(t *testing.T) {
	help := `A fast thing doer.

Usage:
  doer [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  run         Run the thing

Flags:
  -h, --help            help for doer
      --config string   config file (default "$HOME/.doer.yaml")
`
	cmd := Parse("doer", help)
	if got := len(cmd.Subcommands); got != 3 {
		t.Fatalf("want 3 subcommands, got %d", got)
	}
	if cmd.FindSub("run") == nil || cmd.FindSub("run").Summary != "Run the thing" {
		t.Fatalf("got %+v", cmd.Subcommands)
	}
	if len(cmd.Flags) != 2 || cmd.Flags[1].Arg != "string" {
		t.Fatalf("got %+v", cmd.Flags)
	}
}

func TestParseClickStyle(t *testing.T) {
	help := `Usage: flow [OPTIONS] COMMAND [ARGS]...

  Workflow manager.

Options:
  --version  Show the version and exit.
  --help     Show this message and exit.

Commands:
  init  Initialize a workspace.
  sync  Synchronize state.
`
	cmd := Parse("flow", help)
	if len(cmd.Subcommands) != 2 || cmd.FindSub("sync").Summary != "Synchronize state." {
		t.Fatalf("got %+v", cmd.Subcommands)
	}
}

func TestParseArgparseSubparsersBecomeSubcommands(t *testing.T) {
	help := `usage: depot [-h] {push,pull} ...

positional arguments:
  {push,pull}  command to run
    push       upload an artifact
    pull       download an artifact

options:
  -h, --help  show this help message and exit
`
	cmd := Parse("depot", help)
	if len(cmd.Subcommands) != 2 {
		t.Fatalf("want 2 subcommands, got %+v", cmd.Subcommands)
	}
	if cmd.FindSub("push").Summary != "upload an artifact" {
		t.Fatalf("summaries not attached: %+v", cmd.Subcommands)
	}
	if len(cmd.Positionals) != 0 {
		t.Fatalf("subparser row must not also become a positional: %+v", cmd.Positionals)
	}
}

func TestParseCommandAliases(t *testing.T) {
	help := "Usage: pkg <command>\n\nCommands:\n  install, i   add a dependency\n  remove, rm   drop a dependency\n"
	cmd := Parse("pkg", help)
	for _, name := range []string{"install", "i", "remove", "rm"} {
		if cmd.FindSub(name) == nil {
			t.Fatalf("alias %q missing: %+v", name, cmd.Subcommands)
		}
	}
	if cmd.FindSub("i").Summary != "add a dependency" {
		t.Fatalf("alias summary lost: %+v", cmd.FindSub("i"))
	}
}

func TestParseWrappedFlagDescription(t *testing.T) {
	help := `Options:
      --strategy KIND    rollout strategy used for the
                         new release, defaults to rolling
`
	cmd := Parse("x", help)
	want := "rollout strategy used for the new release, defaults to rolling"
	if cmd.Flags[0].Desc != want {
		t.Fatalf("got %q", cmd.Flags[0].Desc)
	}

	// git-style: each description entirely on its own deeper line.
	cmd = Parse("x", "OPTIONS\n    -p, --patch\n        Generate patch.\n")
	if cmd.Flags[0].Desc != "Generate patch." || cmd.Flags[0].Short != "-p" {
		t.Fatalf("got %+v", cmd.Flags[0])
	}
}

func TestParseClapWrappedPossibleValues(t *testing.T) {
	// clap wraps "[possible values: …]" onto its own continuation line;
	// choices must still be mined after the wrap re-attaches.
	help := `Options:
      --show <MODE>
          what to display
          [possible values: full, diff, none]
`
	cmd := Parse("x", help)
	if !reflect.DeepEqual(cmd.Flags[0].Choices, []string{"full", "diff", "none"}) {
		t.Fatalf("got %+v", cmd.Flags[0])
	}
}

func TestParseSectionRouting(t *testing.T) {
	// Example sections are skipped wholesale (their dashes are not
	// flags), while unknown group headers ("Miscellaneous:") still have
	// their option lines mined — the git-help layout.
	help := `Options:
  -v   verbose

Examples:
  tool -x --fake-flag value

Miscellaneous:
  -z   terminate entries with NUL

Output control:
  --stat   show diffstat
`
	cmd := Parse("tool", help)
	if len(cmd.Flags) != 3 {
		t.Fatalf("example lines leaked or groups missed: %+v", cmd.Flags)
	}
	for _, f := range cmd.Flags {
		if f.Short == "-x" || f.Long == "--fake-flag" || f.Long == "--another-fake" {
			t.Fatalf("example flag leaked: %+v", f)
		}
	}
}

func TestParseRejectsBogusCommandRows(t *testing.T) {
	// Word-pairs in prose sections and ALL-CAPS placeholder rows must
	// never be mistaken for subcommands.
	help := `Usage: x <cmd>

Whatever:
  wibble   not a command

Commands:
  COMMAND   the command to run
  real      a real command
`
	cmd := Parse("x", help)
	if len(cmd.Subcommands) != 1 || cmd.Subcommands[0].Name != "real" {
		t.Fatalf("got %+v", cmd.Subcommands)
	}
}

func TestParsePositionalWithFileHint(t *testing.T) {
	help := "Usage: x FILE\n\nArguments:\n  <FILE>   the input file\n"
	cmd := Parse("x", help)
	if len(cmd.Positionals) != 1 || cmd.Positionals[0].Hint != "file" {
		t.Fatalf("got %+v", cmd.Positionals)
	}
}

func TestParseDeduplicatesRepeatedFlags(t *testing.T) {
	// The same flag listed under two group headers must appear once.
	help := "Options:\n  -v, --verbose   noisy\n\nGlobal Flags:\n  -v, --verbose   noisy\n"
	cmd := Parse("x", help)
	if len(cmd.Flags) != 1 {
		t.Fatalf("got %+v", cmd.Flags)
	}
}

func TestParseManStyleUppercaseHeaders(t *testing.T) {
	help := "SYNOPSIS\n  x [flags]\n\nOPTIONS\n  -a   all\n\nEXAMPLES\n  x -broken\n"
	cmd := Parse("x", help)
	if len(cmd.Flags) != 1 || cmd.Flags[0].Short != "-a" {
		t.Fatalf("got %+v", cmd.Flags)
	}
}

func TestParseColoredHelp(t *testing.T) {
	help := "\x1b[33mUsage:\x1b[0m x [OPTIONS]\n\n\x1b[33mOptions:\x1b[0m\n  \x1b[32m-v\x1b[0m   verbose\n"
	cmd := Parse("x", help)
	if cmd.Usage != "x [OPTIONS]" || len(cmd.Flags) != 1 {
		t.Fatalf("got %+v", cmd)
	}
}

func TestParseEmptyTextYieldsEmptyCommand(t *testing.T) {
	cmd := Parse("x", "")
	if len(cmd.Flags)+len(cmd.Subcommands)+len(cmd.Positionals) != 0 {
		t.Fatalf("got %+v", cmd)
	}
	if cmd.Name != "x" {
		t.Fatalf("name %q", cmd.Name)
	}
}
