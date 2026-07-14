// Package helptext parses a CLI's --help output into a spec.Command.
//
// The parser is a line-oriented state machine: section headers switch the
// interpretation mode, option lines are recognized in any option-bearing
// section (terse tools print flags with no header at all), command lines
// only inside command sections, and wrapped description lines re-attach
// to the item above them by indentation.
package helptext

import (
	"regexp"
	"strings"

	"github.com/JaydenCJ/tabsmith/internal/spec"
)

// usageRe captures the classic "Usage: tool [OPTIONS] …" line, any casing.
var usageRe = regexp.MustCompile(`(?i)^\s*usage:\s+(\S.*)$`)

// cmdNameRe validates a subcommand name as listed in a commands section.
var cmdNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:+-]*$`)

// Parse converts one help screen into a command node named name.
// It never fails: unrecognizable lines are simply ignored, and the
// caller decides whether an empty result is an error.
func Parse(name, text string) *spec.Command {
	cmd := &spec.Command{Name: name}
	lines := normalize(text)

	section := secNone
	var lastDesc *string // wrapped-line target
	lastIndent := -1

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			lastDesc = nil
			continue
		}

		if m := usageRe.FindStringSubmatch(line); m != nil {
			if cmd.Usage == "" {
				cmd.Usage = strings.TrimSpace(m[1])
			}
			lastDesc = nil
			continue
		}

		if kind, ok := classifyHeader(line); ok {
			section = kind
			lastDesc = nil
			continue
		}

		if section == secSkip {
			continue
		}

		indent := indentOf(line)

		// Wrapped description line? Must be deeper than the item it
		// belongs to and must not itself start a new option.
		if lastDesc != nil && indent > lastIndent && !looksLikeOption(line) {
			*lastDesc = collapse(*lastDesc + " " + strings.TrimSpace(line))
			continue
		}
		lastDesc = nil

		// Option lines are honored in every minable section: bare helps
		// have no headers, and git-style helps group options under
		// arbitrary titles ("Miscellaneous:").
		if f, ind, ok := tryParseOption(line); ok {
			entry := cmd.AddFlag(f)
			lastDesc, lastIndent = &entry.Desc, ind
			continue
		}

		switch section {
		case secCommands:
			if names, desc, ind, ok := tryParseCommand(line); ok {
				primary := cmd.AddSubcommand(&spec.Command{
					Name: names[0], Summary: collapse(desc),
				})
				for _, alias := range names[1:] {
					cmd.AddSubcommand(&spec.Command{
						Name: alias, Summary: collapse(desc),
					})
				}
				lastDesc, lastIndent = &primary.Summary, ind
			}
		case secPositionals:
			parsePositionalLine(cmd, line, &lastDesc, &lastIndent)
		case secNone:
			if cmd.Summary == "" && indent == 0 {
				cmd.Summary = collapse(line)
			}
		}
	}

	// Wrapped lines may have completed a description after finishFlag ran
	// (clap wraps "[possible values: …]" onto its own line), so derive
	// choices and hints once more over the final text.
	for i := range cmd.Flags {
		finishFlag(&cmd.Flags[i])
	}
	return cmd
}

// looksLikeOption is the cheap pre-check used for wrapped-line handling.
func looksLikeOption(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "-") && !strings.HasPrefix(t, "- ")
}

// tryParseCommand parses "  name[, alias]   description" inside a
// commands section. Placeholder rows like "COMMAND" are rejected.
func tryParseCommand(line string) (names []string, desc string, indent int, ok bool) {
	indent = indentOf(line)
	if indent == 0 {
		return nil, "", 0, false // command listings are always indented
	}
	trimmed := strings.TrimSpace(line)
	namePart := trimmed
	if i := strings.Index(trimmed, "  "); i >= 0 {
		namePart = strings.TrimSpace(trimmed[:i])
		desc = strings.TrimSpace(trimmed[i:])
	}
	for _, n := range strings.Split(namePart, ",") {
		n = strings.TrimSpace(n)
		if !cmdNameRe.MatchString(n) {
			return nil, "", 0, false
		}
		if len(n) > 1 && n == strings.ToUpper(n) && strings.ToLower(n) != n {
			return nil, "", 0, false // "COMMAND" placeholder, not a name
		}
		names = append(names, n)
	}
	if len(names) == 0 {
		return nil, "", 0, false
	}
	return names, desc, indent, true
}

// parsePositionalLine handles argparse-style positional sections. The
// special row "{add,remove}" is the signature of argparse subparsers, so
// its entries become subcommand stubs (probing can then walk into them);
// ordinary rows become positionals with a file/dir hint from their name.
func parsePositionalLine(cmd *spec.Command, line string, lastDesc **string, lastIndent *int) {
	indent := indentOf(line)
	trimmed := strings.TrimSpace(line)
	namePart := trimmed
	desc := ""
	if i := strings.Index(trimmed, "  "); i >= 0 {
		namePart = strings.TrimSpace(trimmed[:i])
		desc = strings.TrimSpace(trimmed[i:])
	}

	if strings.HasPrefix(namePart, "{") && strings.HasSuffix(namePart, "}") {
		inner := strings.Trim(namePart, "{}")
		for _, n := range strings.Split(inner, ",") {
			n = strings.TrimSpace(n)
			if cmdNameRe.MatchString(n) {
				cmd.AddSubcommand(&spec.Command{Name: n})
			}
		}
		return
	}

	// argparse indents each subparser under the {…} row one level deeper;
	// those rows carry the per-command summaries.
	if sub := cmd.FindSub(namePart); sub != nil {
		sub.Summary = collapse(desc)
		*lastDesc, *lastIndent = &sub.Summary, indent
		return
	}

	if !cmdNameRe.MatchString(strings.Trim(namePart, "<>[].")) {
		return
	}
	p := spec.Positional{
		Name: namePart,
		Desc: collapse(desc),
		Hint: hintFor(namePart, ""),
	}
	cmd.Positionals = append(cmd.Positionals, p)
	last := &cmd.Positionals[len(cmd.Positionals)-1]
	*lastDesc, *lastIndent = &last.Desc, indent
}
