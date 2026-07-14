// Text normalization for raw --help output: ANSI colors, tab stops and
// man-style overstrike all get flattened before any line is classified,
// so the rest of the parser only ever sees plain, column-aligned text.
package helptext

import (
	"regexp"
	"strings"
)

// ansiRe matches CSI sequences (colors, cursor moves) and OSC sequences
// (hyperlinks, titles). Tools increasingly color their --help even when
// piped; completions must not inherit the escape bytes.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// stripOverstrike removes man(1)-style bold ("o\bo") and underline
// ("_\bo") backspace sequences, which show up when a tool's --help is a
// piped man page.
func stripOverstrike(s string) string {
	if !strings.ContainsRune(s, '\b') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\b' {
			// Drop the backspace and the character it would overwrite.
			cur := b.String()
			if cur != "" {
				runes := []rune(cur)
				b.Reset()
				b.WriteString(string(runes[:len(runes)-1]))
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// expandTabs replaces tabs with spaces at 8-column stops. Option/desc
// splitting relies on "two or more spaces", so tab-aligned help (BusyBox,
// old GNU tools) must be expanded first, column-accurately.
func expandTabs(line string) string {
	if !strings.ContainsRune(line, '\t') {
		return line
	}
	var b strings.Builder
	col := 0
	for _, r := range line {
		if r == '\t' {
			n := 8 - col%8
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

// normalize splits help text into clean lines: ANSI stripped, overstrike
// resolved, tabs expanded, trailing whitespace and CR removed.
func normalize(text string) []string {
	text = strings.TrimPrefix(text, "\uFEFF")
	text = stripANSI(text)
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		l = strings.TrimRight(l, "\r")
		l = stripOverstrike(l)
		l = expandTabs(l)
		lines[i] = strings.TrimRight(l, " ")
	}
	return lines
}

// indentOf counts leading spaces (tabs are already expanded).
func indentOf(line string) int {
	n := 0
	for _, r := range line {
		if r != ' ' {
			break
		}
		n++
	}
	return n
}

// collapse joins wrapped description fragments with single spaces.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
