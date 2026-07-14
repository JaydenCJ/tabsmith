// Section-header recognition. Help screens group content under headers
// like "Options:", "Available Commands:" or "EXAMPLES"; the header kind
// decides how the lines beneath it are interpreted.
package helptext

import (
	"regexp"
	"strings"
)

type sectionKind int

const (
	secNone sectionKind = iota // before any header
	secOptions
	secCommands
	secPositionals
	secSkip  // examples, environment, exit status … never mine these
	secOther // unknown group header; option lines are still honored
)

// headerRe matches a plausible section header: shallow indent, a short
// title, an optional trailing colon and nothing after it. "Usage: tool …"
// does not match because content follows the colon.
var headerRe = regexp.MustCompile(`^ {0,2}([A-Za-z][A-Za-z0-9 /_()-]{1,60}?):?$`)

// skipWords flags sections whose bodies look option-ish but are not
// (an example invocation is full of dashes). Mining them would inject
// garbage flags, so they are skipped wholesale.
var skipWords = []string{
	"example", "environment", "exit status", "exit code", "see also",
	"learn more", "note", "author", "copyright", "report", "bug",
	"description", "synopsis",
}

// classifyHeader maps a header title to a section kind. ok is false when
// the line is not a header at all.
func classifyHeader(line string) (sectionKind, bool) {
	m := headerRe.FindStringSubmatch(line)
	if m == nil {
		return secNone, false
	}
	title := m[1]
	// A header is either "Title:" or an ALL-CAPS man-style "OPTIONS".
	if !strings.HasSuffix(strings.TrimRight(line, " "), ":") &&
		title != strings.ToUpper(title) {
		return secNone, false
	}
	t := strings.ToLower(title)
	for _, w := range skipWords {
		if strings.Contains(t, w) {
			return secSkip, true
		}
	}
	switch {
	case strings.Contains(t, "positional"):
		return secPositionals, true
	case strings.Contains(t, "command") || strings.Contains(t, "subcommand"):
		return secCommands, true
	case strings.Contains(t, "option") || strings.Contains(t, "flag") ||
		strings.Contains(t, "switch"):
		return secOptions, true
	case t == "arguments" || t == "args" || strings.Contains(t, "argument"):
		return secPositionals, true
	case t == "usage":
		return secOther, true
	default:
		// "Miscellaneous:", "Output control:", … — group headers whose
		// bodies are usually more options. Option lines are still parsed
		// inside secOther; command lines are not.
		return secOther, true
	}
}
