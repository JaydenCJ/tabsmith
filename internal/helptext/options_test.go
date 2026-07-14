// Tests for single-line option parsing across the dialects tabsmith
// supports: GNU getopt, argparse, cobra, clap, click, BusyBox and the Go
// flag package. Each case documents a real-world formatting habit.
package helptext

import (
	"reflect"
	"testing"

	"github.com/JaydenCJ/tabsmith/internal/spec"
)

func mustOption(t *testing.T, line string) spec.Flag {
	t.Helper()
	f, _, ok := tryParseOption(line)
	if !ok {
		t.Fatalf("expected option line to parse: %q", line)
	}
	return f
}

func TestOptionSpellingVariants(t *testing.T) {
	cases := []struct {
		line        string
		short, long string
		desc        string
	}{
		{"  -v, --verbose          print progress details", "-v", "--verbose", "print progress details"},
		{"      --dry-run   plan only", "", "--dry-run", "plan only"},
		{"  -q          be quiet", "-q", "", "be quiet"},
	}
	for _, c := range cases {
		f := mustOption(t, c.line)
		if f.Short != c.short || f.Long != c.long || f.Desc != c.desc {
			t.Fatalf("%q: got %+v", c.line, f)
		}
		if f.TakesValue() {
			t.Fatalf("%q: boolean flag must not take a value", c.line)
		}
	}
}

func TestOptionAlternateSeparators(t *testing.T) {
	// Spellings joined with " | " (git) or "/" (mercurial-style helps)
	// must parse the same as the comma convention.
	for _, line := range []string{
		"  -p | --patch    show the diff",
		"  -p/--patch    show the diff",
	} {
		f := mustOption(t, line)
		if f.Short != "-p" || f.Long != "--patch" {
			t.Fatalf("%q: got %+v", line, f)
		}
	}
}

func TestOptionInlineEqualsPlaceholder(t *testing.T) {
	f := mustOption(t, "      --output=FILE   write to FILE")
	if f.Long != "--output" || f.Arg != "FILE" || f.ArgOptional {
		t.Fatalf("got %+v", f)
	}
	if f.Hint != spec.HintFile {
		t.Fatalf("FILE placeholder should hint file, got %q", f.Hint)
	}
}

func TestOptionOptionalArgumentBrackets(t *testing.T) {
	// GNU style: --color[=WHEN] takes an attached, optional argument.
	f := mustOption(t, "      --color[=WHEN]   colorize the output")
	if f.Long != "--color" || f.Arg != "WHEN" || !f.ArgOptional {
		t.Fatalf("got %+v", f)
	}
}

func TestOptionSeparateAngledPlaceholder(t *testing.T) {
	f := mustOption(t, "  -e, --env <env>        target environment")
	if f.Short != "-e" || f.Long != "--env" || f.Arg != "<env>" {
		t.Fatalf("got %+v", f)
	}
}

func TestChoicesFromPlaceholders(t *testing.T) {
	// argparse {a,b,c} braces and clap/git (a|b) pipes both enumerate.
	f := mustOption(t, "  --format {json,xml,table}   output format")
	if !reflect.DeepEqual(f.Choices, []string{"json", "xml", "table"}) {
		t.Fatalf("got choices %v", f.Choices)
	}
	f = mustOption(t, "      --when <auto|always|never>   when to act")
	if !reflect.DeepEqual(f.Choices, []string{"auto", "always", "never"}) {
		t.Fatalf("got choices %v", f.Choices)
	}
}

func TestOptionGoFlagDialect(t *testing.T) {
	// Go's flag package prints "-min int" with the description on the
	// next line; the type word marks the flag as value-taking. Multi-rune
	// single-dash names are old-style longs, never shorts.
	f := mustOption(t, "  -min int")
	if f.Long != "-min" || f.Arg != "int" {
		t.Fatalf("got %+v", f)
	}
	f = mustOption(t, "  -json    emit machine-readable output")
	if f.Long != "-json" || f.Short != "" || f.TakesValue() {
		t.Fatalf("got %+v", f)
	}
}

func TestOptionGetoptDoubleSpelledValue(t *testing.T) {
	// Classic getopt help repeats the placeholder on both spellings.
	f := mustOption(t, "  -o FILE, --output=FILE   write the archive to FILE")
	if f.Short != "-o" || f.Long != "--output" || f.Arg != "FILE" {
		t.Fatalf("got %+v", f)
	}
}

func TestOptionThreeSpellingsBecomeAliases(t *testing.T) {
	f := mustOption(t, "  -f, --force, --overwrite    replace existing files")
	if f.Short != "-f" || f.Long != "--force" ||
		!reflect.DeepEqual(f.Aliases, []string{"--overwrite"}) {
		t.Fatalf("got %+v", f)
	}
}

func TestOptionSloppySingleSpaceDescription(t *testing.T) {
	// Hand-written helps sometimes separate flag and description with a
	// single space; the tail must become the description, not an arg.
	f := mustOption(t, "  --force overwrite the target")
	if f.Long != "--force" || f.TakesValue() {
		t.Fatalf("got %+v", f)
	}
	if f.Desc != "overwrite the target" {
		t.Fatalf("got desc %q", f.Desc)
	}
}

func TestOptionRejectsProseAndBullets(t *testing.T) {
	for _, line := range []string{
		"  plain prose about the tool",
		"  - a markdown bullet",
		"  ---",
		"  --",
		"",
	} {
		if _, _, ok := tryParseOption(line); ok {
			t.Fatalf("line must not parse as option: %q", line)
		}
	}
}

func TestChoicesFromDescriptions(t *testing.T) {
	// The three prose enumeration habits: clap's "[possible values: …]",
	// hand-written "one of: …", and GNU's quoted "'a', 'b', or 'c'".
	cases := []struct {
		desc string
		want []string
	}{
		{"what to show [possible values: full, diff, none]", []string{"full", "diff", "none"}},
		{"rollout strategy, one of: rolling, canary, bluegreen", []string{"rolling", "canary", "bluegreen"}},
		{"colorize; WHEN is 'always', 'never', or 'auto'", []string{"always", "never", "auto"}},
	}
	for _, c := range cases {
		if got := choicesFromDesc(c.desc); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("%q: got %v", c.desc, got)
		}
	}
}

func TestChoicesRejectInvalidLists(t *testing.T) {
	// A wrong enum silently hides valid values from the user, so any
	// suspicious candidate list is discarded wholesale.
	for _, desc := range []string{
		"pick one of the following files listed below",
		"one of: averyveryverylongchoicevaluename, b",
		"one of: <pattern>, other",
	} {
		if got := choicesFromDesc(desc); got != nil {
			t.Fatalf("%q must not parse as choices, got %v", desc, got)
		}
	}
}

func TestHintClassification(t *testing.T) {
	cases := []struct {
		arg, long string
		want      spec.ValueHint
	}{
		{"FILE", "", spec.HintFile},
		{"<path>", "", spec.HintFile},
		{"DIR", "", spec.HintDir},
		{"DIRECTORY", "", spec.HintDir},
		{"NUM", "", spec.HintNone},
		{"X", "--config-file", spec.HintFile},
		{"X", "--cache-dir", spec.HintDir},
	}
	for _, c := range cases {
		if got := hintFor(c.arg, c.long); got != c.want {
			t.Fatalf("hintFor(%q, %q) = %q, want %q", c.arg, c.long, got, c.want)
		}
	}
}

func TestSplitSpellingsRespectsBraces(t *testing.T) {
	got := splitSpellings("--format {json,xml}, --fmt {json,xml}")
	want := []string{"--format {json,xml}", "--fmt {json,xml}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}
