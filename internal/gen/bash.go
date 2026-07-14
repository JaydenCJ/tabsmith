// Bash generation. The script is a single completion function that
// (1) walks the words typed so far to find the active subcommand node,
// (2) completes the value of the flag before point when the spec knows
// it (choices, files, directories), and (3) otherwise offers the node's
// flags or subcommands depending on whether the current word starts
// with a dash. Pure builtin bash — no bash-completion package required.
package gen

import (
	"fmt"
	"strings"

	"github.com/JaydenCJ/tabsmith/internal/spec"
)

func generateBash(root *spec.Command) string {
	nodes := collectNodes(root)
	fn := funcName(root.Name, nil)

	var b strings.Builder
	b.WriteString(header(Bash, root.Name))
	b.WriteString("#\n# Install: source this file from ~/.bashrc, or copy it into your\n")
	b.WriteString("# bash-completion completions directory as " + FileName(Bash, root.Name) + ".\n\n")

	fmt.Fprintf(&b, "%s()\n{\n", fn)
	b.WriteString("    local cur prev node w i\n")
	b.WriteString("    COMPREPLY=()\n")
	b.WriteString("    cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	b.WriteString("    prev=\"${COMP_WORDS[COMP_CWORD-1]}\"\n\n")

	writeBashNodeWalk(&b, nodes)
	writeBashValueCases(&b, nodes)
	writeBashFlagWords(&b, nodes)
	writeBashPositionalWords(&b, nodes)

	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "complete -F %s %s\n", fn, root.Name)
	return b.String()
}

// writeBashNodeWalk emits the loop that maps the typed words onto a node
// id. Every edge of the tree is one case arm keyed "parent-id/child-name".
func writeBashNodeWalk(b *strings.Builder, nodes []node) {
	b.WriteString("    # Resolve the subcommand path typed so far.\n")
	b.WriteString("    node=\"root\"\n")

	var edges []string
	for _, n := range nodes {
		for _, sub := range n.cmd.Subcommands {
			edges = append(edges, fmt.Sprintf("            %s) node=%s ;;\n",
				bashPat(n.id+"/"+sub.Name), shQuote(n.id+"."+sub.Name)))
		}
	}
	if len(edges) == 0 {
		b.WriteString("\n")
		return
	}
	b.WriteString("    for ((i = 1; i < COMP_CWORD; i++)); do\n")
	b.WriteString("        w=\"${COMP_WORDS[i]}\"\n")
	b.WriteString("        case \"$w\" in -*) continue ;; esac\n")
	b.WriteString("        case \"$node/$w\" in\n")
	for _, e := range edges {
		b.WriteString(e)
	}
	b.WriteString("        esac\n    done\n\n")
}

// writeBashValueCases emits per-(node, flag) value completion for flags
// that take a separate-word argument.
func writeBashValueCases(b *strings.Builder, nodes []node) {
	var arms []string
	for _, n := range nodes {
		for _, f := range valueFlags(n.cmd) {
			var pats []string
			for _, form := range f.Forms() {
				pats = append(pats, bashPat(n.id+":"+form))
			}
			arms = append(arms, fmt.Sprintf("        %s)\n%s",
				strings.Join(pats, " | "), bashValueAction(f)))
		}
	}
	if len(arms) == 0 {
		return
	}
	b.WriteString("    # Complete the value of the flag before point.\n")
	b.WriteString("    case \"$node:$prev\" in\n")
	for _, a := range arms {
		b.WriteString(a)
	}
	b.WriteString("    esac\n\n")
}

func bashValueAction(f *spec.Flag) string {
	switch {
	case len(f.Choices) > 0:
		return fmt.Sprintf("            COMPREPLY=( $(compgen -W %s -- \"$cur\") )\n            return ;;\n",
			shQuote(strings.Join(spec.SortedChoices(f.Choices), " ")))
	case f.Hint == spec.HintFile:
		return "            COMPREPLY=( $(compgen -f -- \"$cur\") )\n            return ;;\n"
	case f.Hint == spec.HintDir:
		return "            COMPREPLY=( $(compgen -d -- \"$cur\") )\n            return ;;\n"
	default:
		return "            return ;; # free-form value\n"
	}
}

// writeBashFlagWords offers the node's flags when the current word starts
// with a dash.
func writeBashFlagWords(b *strings.Builder, nodes []node) {
	b.WriteString("    if [[ \"$cur\" == -* ]]; then\n")
	b.WriteString("        case \"$node\" in\n")
	for _, n := range nodes {
		forms := flagForms(n.cmd)
		if len(forms) == 0 {
			continue
		}
		fmt.Fprintf(b, "            %s)\n                COMPREPLY=( $(compgen -W %s -- \"$cur\") ) ;;\n",
			bashPat(n.id), shQuote(strings.Join(forms, " ")))
	}
	b.WriteString("        esac\n        return\n    fi\n\n")
}

// writeBashPositionalWords offers subcommand names and positional choices,
// falling back to file/dir completion when a positional carries that hint.
func writeBashPositionalWords(b *strings.Builder, nodes []node) {
	b.WriteString("    case \"$node\" in\n")
	for _, n := range nodes {
		words := append(subNames(n.cmd), positionalChoices(n.cmd)...)
		switch {
		case len(words) > 0:
			fmt.Fprintf(b, "        %s)\n            COMPREPLY=( $(compgen -W %s -- \"$cur\") ) ;;\n",
				bashPat(n.id), shQuote(strings.Join(words, " ")))
		case positionalHint(n.cmd) == spec.HintFile:
			fmt.Fprintf(b, "        %s)\n            COMPREPLY=( $(compgen -f -- \"$cur\") ) ;;\n",
				bashPat(n.id))
		case positionalHint(n.cmd) == spec.HintDir:
			fmt.Fprintf(b, "        %s)\n            COMPREPLY=( $(compgen -d -- \"$cur\") ) ;;\n",
				bashPat(n.id))
		}
	}
	b.WriteString("    esac\n")
}

// bashPat quotes a literal string for use as a bash case pattern.
func bashPat(s string) string {
	return "\"" + strings.ReplaceAll(s, `"`, `\"`) + "\""
}
