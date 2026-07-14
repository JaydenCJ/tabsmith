// Zsh generation. One `_arguments`-based function per command node, a
// `_describe` block for subcommand menus (with descriptions), and the
// urfave-style words-rescope dispatch: after `*::arg:->args`, zsh rewrites
// $words so the subcommand sits at words[1], which lets each node's
// function run `_arguments` as if its subcommand were the command.
package gen

import (
	"fmt"
	"strings"

	"github.com/JaydenCJ/tabsmith/internal/spec"
)

func generateZsh(root *spec.Command) string {
	nodes := collectNodes(root)

	var b strings.Builder
	fmt.Fprintf(&b, "#compdef %s\n", root.Name)
	b.WriteString(header(Zsh, root.Name))
	b.WriteString("#\n# Install: save as " + FileName(Zsh, root.Name) +
		" in a directory on $fpath, then run compinit.\n\n")

	for _, n := range nodes {
		writeZshNode(&b, root.Name, n)
	}

	fmt.Fprintf(&b, "%s \"$@\"\n", funcName(root.Name, nil))
	return b.String()
}

func writeZshNode(b *strings.Builder, tool string, n node) {
	fn := funcName(tool, n.path)
	hasSubs := len(n.cmd.Subcommands) > 0

	fmt.Fprintf(b, "%s() {\n", fn)
	if hasSubs {
		b.WriteString("    local curcontext=\"$curcontext\" state line\n")
		b.WriteString("    local -a commands\n")
	}

	b.WriteString("    _arguments -s -C \\\n")
	var specs []string
	for i := range n.cmd.Flags {
		specs = append(specs, zshFlagSpec(&n.cmd.Flags[i]))
	}
	if hasSubs {
		specs = append(specs, "'1: :->cmnds'", "'*::arg:->args'")
	} else {
		specs = append(specs, zshPositionalSpecs(n.cmd)...)
	}
	for i, s := range specs {
		sep := " \\\n"
		if i == len(specs)-1 {
			sep = "\n"
		}
		b.WriteString("        " + s + sep)
	}

	if hasSubs {
		b.WriteString("    case \"$state\" in\n        cmnds)\n            commands=(\n")
		for _, sub := range n.cmd.Subcommands {
			entry := sub.Name
			if d := spec.CleanDesc(sub.Summary, 72); d != "" {
				entry += ":" + strings.ReplaceAll(d, ":", `\:`)
			}
			b.WriteString("                " + shQuote(entry) + "\n")
		}
		fmt.Fprintf(b, "            )\n            _describe -t commands '%s command' commands\n            ;;\n    esac\n",
			strings.Join(append([]string{tool}, n.path...), " "))
		b.WriteString("    case \"${words[1]}\" in\n")
		for _, sub := range n.cmd.Subcommands {
			fmt.Fprintf(b, "        %s)\n            %s\n            ;;\n",
				shQuote(sub.Name), funcName(tool, append(n.path, sub.Name)))
		}
		b.WriteString("    esac\n")
	}
	b.WriteString("}\n\n")
}

// zshFlagSpec renders one flag as an _arguments optspec. Multi-spelling
// flags use an exclusion group plus brace expansion, the standard idiom:
//
//	'(-o --output)'{-o+,--output=}'[write to FILE]:file:_files'
func zshFlagSpec(f *spec.Flag) string {
	forms := f.Forms()
	value := zshValueSpec(f)
	desc := "[" + zshDescEscape(spec.CleanDesc(f.Desc, 72)) + "]"
	if f.Desc == "" {
		desc = ""
	}

	if len(forms) == 1 {
		return shQuote(forms[0] + zshArgSuffix(forms[0], f) + desc + value)
	}
	var braced []string
	for _, form := range forms {
		braced = append(braced, form+zshArgSuffix(form, f))
	}
	return shQuote("("+strings.Join(forms, " ")+")") +
		"{" + strings.Join(braced, ",") + "}" +
		shQuote(desc+value)
}

// zshArgSuffix returns the optspec suffix declaring how the value is
// attached: short flags take `+` (same or next word), long flags `=`
// (--flag=v or --flag v). Boolean flags take nothing.
func zshArgSuffix(form string, f *spec.Flag) string {
	if !f.TakesValue() {
		return ""
	}
	if strings.HasPrefix(form, "--") || len(form) > 2 {
		return "="
	}
	return "+"
}

// zshValueSpec renders the `:message:action` tail for a value flag.
// Optional arguments get `::` (zsh's own optionality marker); flags with
// no usable hint get an empty candidate list so zsh shows the message
// instead of wrongly offering files.
func zshValueSpec(f *spec.Flag) string {
	if !f.TakesValue() {
		return ""
	}
	sep := ":"
	if f.ArgOptional {
		sep = "::"
	}
	label := zshLabel(f.Arg)
	switch {
	case len(f.Choices) > 0:
		return sep + label + ":(" + strings.Join(spec.SortedChoices(f.Choices), " ") + ")"
	case f.Hint == spec.HintFile:
		return sep + label + ":_files"
	case f.Hint == spec.HintDir:
		return sep + label + ":_files -/"
	default:
		return sep + label + ":( )"
	}
}

// zshPositionalSpecs renders positional slots for leaf commands; slots
// with no choices and no hint are omitted (zsh then completes nothing,
// which is honest).
func zshPositionalSpecs(cmd *spec.Command) []string {
	var out []string
	for i, p := range cmd.Positionals {
		label := zshLabel(p.Name)
		switch {
		case len(p.Choices) > 0:
			out = append(out, shQuote(fmt.Sprintf("%d:%s:(%s)", i+1, label,
				strings.Join(spec.SortedChoices(p.Choices), " "))))
		case p.Hint == spec.HintFile:
			out = append(out, shQuote(fmt.Sprintf("%d:%s:_files", i+1, label)))
		case p.Hint == spec.HintDir:
			out = append(out, shQuote(fmt.Sprintf("%d:%s:_files -/", i+1, label)))
		}
	}
	if len(out) == 0 && len(cmd.Positionals) == 0 && len(cmd.Subcommands) == 0 {
		// Nothing known about arguments: fall back to default completion.
		out = append(out, shQuote("*: :_default"))
	}
	return out
}

// zshLabel derives the value message from a placeholder ("<FILE>" → file).
func zshLabel(arg string) string {
	l := strings.ToLower(strings.Trim(arg, "<>[]{}."))
	l = strings.ReplaceAll(l, ":", "-")
	if l == "" {
		l = "value"
	}
	return l
}

// zshDescEscape protects an _arguments description: it lives inside
// [brackets] inside single quotes.
func zshDescEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `[`, `\[`)
	s = strings.ReplaceAll(s, `]`, `\]`)
	return s
}
