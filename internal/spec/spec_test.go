// Tests for the command model: merge semantics, tree traversal and the
// helpers the generators lean on for deterministic output.
package spec

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAddFlagMergeSemantics(t *testing.T) {
	// Same long spelling merges (filling in missing fields), distinct
	// spellings append, and the returned pointer addresses the entry
	// that actually holds the data.
	c := &Command{Name: "x"}
	c.AddFlag(Flag{Long: "--output", Arg: "FILE", Hint: HintFile})
	got := c.AddFlag(Flag{Short: "-o", Long: "--output", Desc: "write to FILE"})
	if len(c.Flags) != 1 || got != &c.Flags[0] {
		t.Fatalf("got %d flags", len(c.Flags))
	}
	if got.Short != "-o" || got.Arg != "FILE" || got.Desc != "write to FILE" {
		t.Fatalf("merge lost data: %+v", got)
	}
	c.AddFlag(Flag{Short: "-b"})
	if len(c.Flags) != 2 {
		t.Fatalf("distinct flag must append: %+v", c.Flags)
	}
}

func TestFormsOrderShortLongAliases(t *testing.T) {
	f := Flag{Short: "-f", Long: "--force", Aliases: []string{"--overwrite"}}
	want := []string{"-f", "--force", "--overwrite"}
	if got := f.Forms(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestAddSubcommandMergesByName(t *testing.T) {
	c := &Command{Name: "x"}
	c.AddSubcommand(&Command{Name: "run", Summary: "run it"})
	c.AddSubcommand(&Command{Name: "run", Flags: []Flag{{Short: "-v"}}})
	if len(c.Subcommands) != 1 {
		t.Fatalf("got %d subcommands", len(c.Subcommands))
	}
	got := c.Subcommands[0]
	if got.Summary != "run it" || len(got.Flags) != 1 {
		t.Fatalf("merge lost data: %+v", got)
	}
}

func TestWalkVisitsDepthFirstWithPaths(t *testing.T) {
	root := &Command{Name: "a"}
	b := root.AddSubcommand(&Command{Name: "b"})
	b.AddSubcommand(&Command{Name: "c"})
	root.AddSubcommand(&Command{Name: "d"})

	var visited []string
	root.Walk(func(path []*Command) {
		names := ""
		for _, p := range path {
			names += p.Name
		}
		visited = append(visited, names)
	})
	want := []string{"a", "ab", "abc", "ad"}
	if !reflect.DeepEqual(visited, want) {
		t.Fatalf("got %v", visited)
	}
}

func TestStatsCountsWholeTree(t *testing.T) {
	root := &Command{Name: "a", Flags: []Flag{{Short: "-v"}}}
	sub := root.AddSubcommand(&Command{Name: "b", Flags: []Flag{{Short: "-x"}, {Short: "-y"}}})
	sub.AddSubcommand(&Command{Name: "c"})
	flags, subs := root.Stats()
	if flags != 3 || subs != 2 {
		t.Fatalf("got flags=%d subs=%d", flags, subs)
	}
}

func TestGeneratorHelpersAreDeterministicAndSafe(t *testing.T) {
	// SortedChoices must not mutate the parsed model; CleanDesc must
	// collapse whitespace and cap on a word boundary.
	in := []string{"c", "a", "b"}
	if got := SortedChoices(in); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("got %v", got)
	}
	if !reflect.DeepEqual(in, []string{"c", "a", "b"}) {
		t.Fatalf("input mutated: %v", in)
	}
	if got := CleanDesc("a  very\n long   description that keeps going", 20); got != "a very long…" {
		t.Fatalf("got %q", got)
	}
	if CleanDesc("short", 20) != "short" {
		t.Fatal("short strings must pass through")
	}
}

func TestJSONRoundTripPreservesTree(t *testing.T) {
	root := &Command{
		Name:  "x",
		Usage: "x [OPTIONS]",
		Flags: []Flag{{Short: "-o", Long: "--output", Arg: "FILE", Hint: HintFile}},
		Subcommands: []*Command{
			{Name: "run", Flags: []Flag{{Long: "--fast", Choices: []string{"a", "b"}}}},
		},
	}
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	var back Command
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(&back, root) {
		t.Fatalf("round trip changed the tree:\n%+v\n%+v", root, &back)
	}
}
