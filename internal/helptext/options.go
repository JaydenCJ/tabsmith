// Option-line parsing: the heart of the help-text parser. One physical
// line like
//
//	-o, --output <FILE>      write the report to FILE
//
// becomes a spec.Flag with spellings, a value placeholder, a hint and a
// description. The grammar here is deliberately forgiving — it has to
// absorb getopt, argparse, cobra, clap, click and BusyBox dialects — but
// every acceptance rule is guarded so prose lines never turn into flags.
package helptext

import (
	"regexp"
	"strings"

	"github.com/JaydenCJ/tabsmith/internal/spec"
)

// flagNameRe validates one flag spelling after any "=ARG" suffix is cut:
// "-o", "--long-name", Go-style "-verbose", "--no-color", "--@special".
var flagNameRe = regexp.MustCompile(`^--?[A-Za-z0-9?@#][A-Za-z0-9._@#-]*$`)

// placeholderRe accepts value placeholders as help screens print them:
// <file>, {json,xml}, FILE, KEY=VALUE, N, (auto|never), name...
var placeholderRe = regexp.MustCompile(
	`^(<[^>]+>(\.\.\.)?|\{[^}]+\}|\([^)]+\)|[A-Z][A-Z0-9_:=,.|-]*|[a-z][a-z0-9|-]*\|[a-z0-9|-]+)$`)

// goTypeWords are the value-type names Go's flag package prints after a
// flag ("-min int"); they mark the flag as value-taking even though they
// are lowercase prose-looking words.
var goTypeWords = map[string]bool{
	"int": true, "uint": true, "float": true, "string": true,
	"duration": true, "value": true, "file": true, "path": true, "dir": true,
}

// tryParseOption parses one line as an option definition. ok is false when
// the line does not look like one; indent is the flag column, used by the
// caller to attach wrapped description lines.
func tryParseOption(line string) (f spec.Flag, indent int, ok bool) {
	indent = indentOf(line)
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "-") || trimmed == "-" || trimmed == "--" ||
		strings.HasPrefix(trimmed, "---") {
		return f, indent, false
	}
	// Markdown-ish bullet ("- item"), not an option.
	if strings.HasPrefix(trimmed, "- ") {
		return f, indent, false
	}

	flagPart, desc := splitFlagDesc(trimmed)
	pieces := splitSpellings(flagPart)
	if len(pieces) == 0 {
		return f, indent, false
	}

	for i, piece := range pieces {
		fields := strings.Fields(piece)
		if len(fields) == 0 {
			continue
		}
		name, pieceArg, pieceOpt := cutInlineArg(fields[0])
		if !flagNameRe.MatchString(name) {
			if i == 0 {
				return f, indent, false // first spelling must be a flag
			}
			continue // tolerate junk after a valid first spelling
		}
		// A separate-word placeholder: "-o FILE", "--format {json,xml}",
		// or a Go flag-package type word ("-min int").
		if pieceArg == "" && len(fields) > 1 {
			cand := strings.Join(fields[1:], " ")
			if placeholderRe.MatchString(cand) || goTypeWords[cand] {
				pieceArg = cand
				if strings.HasPrefix(cand, "[") { // "--color [WHEN]"
					pieceOpt = true
				}
			} else if desc == "" && i == len(pieces)-1 {
				// Single-space-separated description (sloppy help).
				desc = cand
			}
		}
		assignSpelling(&f, name)
		if f.Arg == "" && pieceArg != "" {
			f.Arg = strings.TrimSpace(pieceArg)
			f.ArgOptional = pieceOpt
		}
	}
	if f.Short == "" && f.Long == "" {
		return f, indent, false
	}
	f.Desc = collapse(desc)
	finishFlag(&f)
	return f, indent, true
}

// splitFlagDesc separates the flag column from the description column at
// the first run of two-or-more spaces. Tab-aligned input was expanded
// upstream, so this single rule covers every dialect seen in the wild.
func splitFlagDesc(trimmed string) (flagPart, desc string) {
	if i := strings.Index(trimmed, "  "); i >= 0 {
		return strings.TrimSpace(trimmed[:i]), strings.TrimSpace(trimmed[i:])
	}
	return trimmed, ""
}

// splitSpellings breaks "-o FILE, --output=FILE" into its spellings.
// Separators are ", ", " | " and "/" directly before a dash; commas inside
// {a,b,c} or <...> placeholders never split.
func splitSpellings(flagPart string) []string {
	// Normalize alternative separators onto the comma.
	flagPart = regexp.MustCompile(`\s*\|\s*(-)`).ReplaceAllString(flagPart, ",$1")
	flagPart = regexp.MustCompile(`/(-)`).ReplaceAllString(flagPart, ",$1")

	var out []string
	depth := 0
	start := 0
	for i, r := range flagPart {
		switch r {
		case '{', '<', '(', '[':
			depth++
		case '}', '>', ')', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(flagPart[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(flagPart[start:]))
	return out
}

// cutInlineArg splits "--color[=WHEN]" / "--output=FILE" into the flag
// name and its inline placeholder.
func cutInlineArg(token string) (name, arg string, optional bool) {
	if i := strings.Index(token, "[="); i > 0 {
		return token[:i], strings.TrimSuffix(token[i+2:], "]"), true
	}
	if i := strings.IndexByte(token, '='); i > 0 {
		return token[:i], token[i+1:], false
	}
	return token, "", false
}

// assignSpelling files a validated spelling into the right Flag slot.
func assignSpelling(f *spec.Flag, name string) {
	isShort := len(name) == 2 && !strings.HasPrefix(name, "--")
	switch {
	case isShort && f.Short == "":
		f.Short = name
	case !isShort && f.Long == "":
		f.Long = name
	case name != f.Short && name != f.Long:
		for _, a := range f.Aliases {
			if a == name {
				return
			}
		}
		f.Aliases = append(f.Aliases, name)
	}
}

// finishFlag derives choices and a value hint once all spellings are in.
// It is idempotent: the parser calls it again after wrapped description
// lines re-attach, because clap wraps "[possible values: …]" onto its own
// continuation line.
func finishFlag(f *spec.Flag) {
	if len(f.Choices) == 0 && f.Arg != "" {
		f.Choices = choicesFromPlaceholder(f.Arg)
	}
	if len(f.Choices) == 0 {
		f.Choices = choicesFromDesc(f.Desc)
	}
	if f.Arg == "" && len(f.Choices) > 0 {
		// "--color   one of: auto, always, never" with no printed
		// placeholder still takes a value.
		f.Arg = "VALUE"
	}
	if f.Arg != "" && len(f.Choices) == 0 {
		f.Hint = hintFor(f.Arg, f.Long)
	}
}

// choicesFromPlaceholder mines enumerations out of the placeholder itself:
// {json,xml} (argparse), <auto|never> and (auto|never) (clap, git),
// bare auto|never.
func choicesFromPlaceholder(arg string) []string {
	inner := strings.Trim(arg, "<>{}()[]")
	var parts []string
	switch {
	case strings.ContainsRune(inner, '|'):
		parts = strings.Split(inner, "|")
	case strings.HasPrefix(arg, "{") && strings.ContainsRune(inner, ','):
		parts = strings.Split(inner, ",")
	default:
		return nil
	}
	return validChoices(parts)
}

var possibleValuesRe = regexp.MustCompile(`(?i)possible values:\s*([^\]\n]+)`)
var oneOfRe = regexp.MustCompile(`(?i)\bone of:?\s+([A-Za-z0-9_"'.,|:\x60 -]+)`)

// quotedListRe spots GNU-style enumerations in prose: WHEN is 'always',
// 'never', or 'auto'. The mandatory trailing "or 'x'" keeps stray quoted
// words from being read as an enum.
var quotedListRe = regexp.MustCompile(
	`(?:'[^' ]{1,20}'|"[^" ]{1,20}")(?:\s*,\s*(?:'[^' ]{1,20}'|"[^" ]{1,20}"))*\s*,?\s+or\s+(?:'[^' ]{1,20}'|"[^" ]{1,20}")`)
var quotedTokenRe = regexp.MustCompile(`'([^' ]{1,20})'|"([^" ]{1,20})"`)

// choicesFromDesc mines enumerations out of the description:
// "[possible values: full, diff, none]" (clap), "one of: auto, always,
// never" (many hand-written helps) and "'always', 'never', or 'auto'"
// (GNU coreutils prose). Every candidate list is validated — prose like
// "one of the following files" is rejected wholesale.
func choicesFromDesc(desc string) []string {
	if m := possibleValuesRe.FindStringSubmatch(desc); m != nil {
		if c := validChoices(strings.Split(m[1], ",")); c != nil {
			return c
		}
	}
	if m := oneOfRe.FindStringSubmatch(desc); m != nil {
		list := regexp.MustCompile(`\s*(?:,|\bor\b|\|)\s*`).Split(m[1], -1)
		if c := validChoices(list); c != nil {
			return c
		}
	}
	if m := quotedListRe.FindString(desc); m != "" {
		var list []string
		for _, q := range quotedTokenRe.FindAllStringSubmatch(m, -1) {
			list = append(list, q[1]+q[2])
		}
		if c := validChoices(list); c != nil {
			return c
		}
	}
	return nil
}

// validChoices keeps a candidate list only when every entry looks like a
// literal value: short, no spaces, no placeholder syntax. One bad entry
// discards the whole list — a wrong enum is worse than no enum.
func validChoices(parts []string) []string {
	if len(parts) < 2 {
		return nil
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), `"'`+"`")
		p = strings.TrimSuffix(p, ".")
		if p == "" || len(p) > 20 || strings.ContainsAny(p, " <>{}$()") {
			return nil
		}
		out = append(out, p)
	}
	return out
}

// hintFor classifies a value as file- or directory-shaped from the
// placeholder first ("FILE", "<path>", "DIR"), then from the flag's own
// name ("--config-file", "--output-dir").
func hintFor(arg, long string) spec.ValueHint {
	u := strings.ToUpper(strings.Trim(arg, "<>[]{}."))
	switch {
	case strings.Contains(u, "DIRECTORY") || strings.Contains(u, "FOLDER") ||
		u == "DIR" || strings.HasSuffix(u, "_DIR") || strings.HasSuffix(u, "-DIR"):
		return spec.HintDir
	case strings.Contains(u, "FILE") || u == "PATH" || strings.HasSuffix(u, "_PATH") ||
		strings.HasSuffix(u, "-PATH"):
		return spec.HintFile
	}
	n := strings.ToLower(strings.TrimLeft(long, "-"))
	switch {
	case n == "dir" || n == "directory" || strings.HasSuffix(n, "-dir") ||
		strings.HasSuffix(n, "-directory"):
		return spec.HintDir
	case n == "file" || n == "path" || strings.HasSuffix(n, "-file") ||
		strings.HasSuffix(n, "-path"):
		return spec.HintFile
	}
	return spec.HintNone
}
