// Package spec defines the shell-agnostic command model that sits between
// the help-text parser and the per-shell generators.
//
// A Command is a tree: the root is the tool itself, children are
// subcommands discovered from "Commands:" sections (and, when probing,
// from recursive `tool sub --help` runs). Flags carry their original
// spellings — dashes included — so generators can tell POSIX shorts,
// GNU longs and Go-style single-dash longs apart without re-guessing.
package spec

import (
	"sort"
	"strings"
)

// ValueHint says what kind of value a flag or positional expects, when the
// help text gave us enough to tell. Generators map it to the shell's
// native file/directory completion.
type ValueHint string

const (
	// HintNone means we know nothing about the value.
	HintNone ValueHint = ""
	// HintFile means the value is a file path.
	HintFile ValueHint = "file"
	// HintDir means the value is a directory path.
	HintDir ValueHint = "dir"
)

// Flag is one option of a command. Spellings keep their leading dashes:
// Short is like "-o", Long is like "--output" or Go-style "-output".
type Flag struct {
	Short       string    `json:"short,omitempty"`
	Long        string    `json:"long,omitempty"`
	Aliases     []string  `json:"aliases,omitempty"`
	Arg         string    `json:"arg,omitempty"`         // placeholder as printed, e.g. "FILE" or "<when>"
	ArgOptional bool      `json:"argOptional,omitempty"` // --color[=WHEN] style
	Choices     []string  `json:"choices,omitempty"`
	Hint        ValueHint `json:"hint,omitempty"`
	Desc        string    `json:"desc,omitempty"`
}

// TakesValue reports whether the flag expects an argument.
func (f *Flag) TakesValue() bool { return f.Arg != "" }

// Forms returns every spelling of the flag, shortest first, dashes intact.
func (f *Flag) Forms() []string {
	var out []string
	if f.Short != "" {
		out = append(out, f.Short)
	}
	if f.Long != "" {
		out = append(out, f.Long)
	}
	out = append(out, f.Aliases...)
	return out
}

// key identifies a flag for de-duplication: the long spelling wins,
// falling back to the short one.
func (f *Flag) key() string {
	if f.Long != "" {
		return f.Long
	}
	return f.Short
}

// Positional is a non-flag argument slot of a command.
type Positional struct {
	Name    string    `json:"name"`
	Desc    string    `json:"desc,omitempty"`
	Choices []string  `json:"choices,omitempty"`
	Hint    ValueHint `json:"hint,omitempty"`
}

// Command is one node of the command tree.
type Command struct {
	Name        string       `json:"name"`
	Summary     string       `json:"summary,omitempty"`
	Usage       string       `json:"usage,omitempty"`
	Flags       []Flag       `json:"flags,omitempty"`
	Positionals []Positional `json:"positionals,omitempty"`
	Subcommands []*Command   `json:"subcommands,omitempty"`
}

// AddFlag appends f unless an equivalent flag is already present, in which
// case the two are merged (the earlier entry keeps its slot; missing
// spellings, args, choices and descriptions are filled in from f). Help
// screens routinely list the same flag under several group headers, so
// this keeps the model — and the generated scripts — free of duplicates.
// The returned pointer addresses the merged entry and is valid until the
// next mutation of c.Flags.
func (c *Command) AddFlag(f Flag) *Flag {
	for i := range c.Flags {
		e := &c.Flags[i]
		if sameFlag(e, &f) {
			mergeFlag(e, &f)
			return e
		}
	}
	c.Flags = append(c.Flags, f)
	return &c.Flags[len(c.Flags)-1]
}

func sameFlag(a, b *Flag) bool {
	if a.Long != "" && a.Long == b.Long {
		return true
	}
	if a.Short != "" && a.Short == b.Short {
		return true
	}
	return a.key() != "" && a.key() == b.key()
}

func mergeFlag(dst, src *Flag) {
	if dst.Short == "" {
		dst.Short = src.Short
	}
	if dst.Long == "" {
		dst.Long = src.Long
	}
	for _, al := range src.Aliases {
		if !contains(dst.Aliases, al) && al != dst.Short && al != dst.Long {
			dst.Aliases = append(dst.Aliases, al)
		}
	}
	if dst.Arg == "" {
		dst.Arg = src.Arg
		dst.ArgOptional = src.ArgOptional
	}
	if len(dst.Choices) == 0 {
		dst.Choices = src.Choices
	}
	if dst.Hint == HintNone {
		dst.Hint = src.Hint
	}
	if dst.Desc == "" {
		dst.Desc = src.Desc
	}
}

// AddSubcommand appends sub unless a subcommand with the same name exists;
// then the existing node absorbs whatever the new one knows.
func (c *Command) AddSubcommand(sub *Command) *Command {
	if got := c.FindSub(sub.Name); got != nil {
		if got.Summary == "" {
			got.Summary = sub.Summary
		}
		for _, f := range sub.Flags {
			got.AddFlag(f)
		}
		for _, s := range sub.Subcommands {
			got.AddSubcommand(s)
		}
		return got
	}
	c.Subcommands = append(c.Subcommands, sub)
	return sub
}

// FindSub returns the direct subcommand called name, or nil.
func (c *Command) FindSub(name string) *Command {
	for _, s := range c.Subcommands {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// Walk visits the command and every descendant, depth-first, handing each
// visit the path from the root (path[0] is the root itself).
func (c *Command) Walk(fn func(path []*Command)) {
	var rec func(path []*Command)
	rec = func(path []*Command) {
		fn(path)
		for _, s := range path[len(path)-1].Subcommands {
			rec(append(path, s))
		}
	}
	rec([]*Command{c})
}

// Stats counts flags and subcommands across the whole tree; the CLI uses
// it for its one-line summary and its "nothing parsed" exit decision.
func (c *Command) Stats() (flags, subs int) {
	c.Walk(func(path []*Command) {
		n := path[len(path)-1]
		flags += len(n.Flags)
		if len(path) > 1 {
			subs++
		}
	})
	return
}

// SortedChoices returns a copy of the choice list in stable sorted order.
// Help screens list choices in prose order; sorting keeps generator
// output deterministic without mutating the parsed model.
func SortedChoices(choices []string) []string {
	out := append([]string(nil), choices...)
	sort.Strings(out)
	return out
}

// CleanDesc collapses runs of whitespace and caps the description at max
// runes for use in one-line completion labels. Long GNU descriptions are
// cut on a word boundary with an ellipsis.
func CleanDesc(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max <= 0 || len([]rune(s)) <= max {
		return s
	}
	runes := []rune(s)
	cut := max
	for cut > 0 && runes[cut-1] != ' ' {
		cut--
	}
	if cut == 0 {
		cut = max
	}
	return strings.TrimRight(string(runes[:cut]), " ") + "…"
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}
