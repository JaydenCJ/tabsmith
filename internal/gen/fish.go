// Fish generation. Fish completions are declarative `complete` calls
// guarded by a condition; nesting deeper than one subcommand level breaks
// the stock __fish_seen_subcommand_from helper, so the script ships its
// own tiny resolver: a function that walks the words on the command line
// through the known command tree and prints the active path.
package gen

import (
	"fmt"
	"strings"

	"github.com/JaydenCJ/tabsmith/internal/spec"
)

func generateFish(root *spec.Command) string {
	nodes := collectNodes(root)
	hasSubs := len(root.Subcommands) > 0
	resolver := funcName(root.Name, nil) + "_cmd"
	at := funcName(root.Name, nil) + "_at"

	var b strings.Builder
	b.WriteString(header(Fish, root.Name))
	b.WriteString("#\n# Install: save as " + FileName(Fish, root.Name) +
		" in ~/.config/fish/completions/.\n\n")

	if hasSubs {
		writeFishResolver(&b, resolver, at, nodes)
	}

	for _, n := range nodes {
		cond := ""
		if hasSubs {
			cond = fmt.Sprintf("-n '%s \"%s\"' ", at, strings.Join(n.path, " "))
		}
		writeFishNode(&b, root.Name, cond, n)
	}
	return b.String()
}

// writeFishResolver emits the command-path resolver and its predicate.
// The resolver only follows words that are known children of the current
// node, so free-form positional arguments never derail it.
func writeFishResolver(b *strings.Builder, resolver, at string, nodes []node) {
	fmt.Fprintf(b, "function %s\n", resolver)
	b.WriteString("    set -l cmd\n")
	b.WriteString("    set -l tokens (commandline -opc)\n")
	b.WriteString("    for t in $tokens[2..-1]\n")
	b.WriteString("        if string match -q -- '-*' $t\n            continue\n        end\n")
	b.WriteString("        switch \"$cmd\"\n")
	for _, n := range nodes {
		if len(n.cmd.Subcommands) == 0 {
			continue
		}
		fmt.Fprintf(b, "            case %s\n", shQuote(strings.Join(n.path, " ")))
		fmt.Fprintf(b, "                if contains -- $t %s\n",
			strings.Join(quoteAll(subNames(n.cmd)), " "))
		if len(n.path) == 0 {
			b.WriteString("                    set cmd $t\n")
		} else {
			b.WriteString("                    set cmd \"$cmd $t\"\n")
		}
		b.WriteString("                end\n")
	}
	b.WriteString("        end\n    end\n    echo $cmd\nend\n\n")

	fmt.Fprintf(b, "function %s\n", at)
	fmt.Fprintf(b, "    set -l cur (%s)\n", resolver)
	b.WriteString("    test \"$cur\" = $argv[1]\nend\n\n")
}

// writeFishNode emits the complete calls for one command node.
func writeFishNode(b *strings.Builder, tool, cond string, n node) {
	where := "root"
	if len(n.path) > 0 {
		where = strings.Join(n.path, " ")
	}
	if len(n.cmd.Subcommands)+len(n.cmd.Flags) > 0 {
		fmt.Fprintf(b, "# %s\n", where)
	}

	for _, sub := range n.cmd.Subcommands {
		line := fmt.Sprintf("complete -c %s %s-f -a %s", tool, cond, shQuote(sub.Name))
		if d := spec.CleanDesc(sub.Summary, 72); d != "" {
			line += " -d " + shQuote(d)
		}
		b.WriteString(line + "\n")
	}

	for i := range n.cmd.Flags {
		b.WriteString(fishFlagLine(tool, cond, &n.cmd.Flags[i]) + "\n")
	}

	// Positional choices complete like subcommands; file/dir hints keep
	// fish's default file completion via an explicit -F.
	if choices := positionalChoices(n.cmd); len(choices) > 0 {
		fmt.Fprintf(b, "complete -c %s %s-f -a %s\n", tool, cond,
			shQuote(strings.Join(choices, " ")))
	}

	if len(n.cmd.Subcommands)+len(n.cmd.Flags) > 0 {
		b.WriteString("\n")
	}
}

// fishFlagLine renders one flag as a complete call. Spellings map to
// fish's native switches: -s (short), -l (GNU long), -o (old-style
// single-dash long, the Go flag package's dialect).
func fishFlagLine(tool, cond string, f *spec.Flag) string {
	parts := []string{"complete", "-c", tool}
	if cond != "" {
		parts = append(parts, strings.TrimSpace(cond))
	}
	for _, form := range f.Forms() {
		switch {
		case strings.HasPrefix(form, "--"):
			parts = append(parts, "-l", strings.TrimPrefix(form, "--"))
		case len(form) == 2:
			parts = append(parts, "-s", strings.TrimPrefix(form, "-"))
		default:
			parts = append(parts, "-o", strings.TrimPrefix(form, "-"))
		}
	}
	if f.TakesValue() && !f.ArgOptional {
		switch {
		case len(f.Choices) > 0:
			parts = append(parts, "-x", "-a",
				shQuote(strings.Join(spec.SortedChoices(f.Choices), " ")))
		case f.Hint == spec.HintFile:
			parts = append(parts, "-r", "-F")
		case f.Hint == spec.HintDir:
			parts = append(parts, "-x", "-a", "'(__fish_complete_directories)'")
		default:
			parts = append(parts, "-x")
		}
	}
	if d := spec.CleanDesc(f.Desc, 72); d != "" {
		parts = append(parts, "-d", shQuote(d))
	}
	return strings.Join(parts, " ")
}

func quoteAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = shQuote(n)
	}
	return out
}
