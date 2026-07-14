// Package cli implements the tabsmith command-line interface. All
// behavior lives here, driven through io.Reader/Writer, so the whole CLI
// is testable in-process; cmd/tabsmith/main.go only wires the real
// process streams.
//
// Exit codes: 0 success, 1 nothing useful could be parsed out of the
// help text, 2 usage/probe/IO error.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JaydenCJ/tabsmith/internal/gen"
	"github.com/JaydenCJ/tabsmith/internal/helptext"
	"github.com/JaydenCJ/tabsmith/internal/probe"
	"github.com/JaydenCJ/tabsmith/internal/spec"
	"github.com/JaydenCJ/tabsmith/internal/version"
)

const usageText = `tabsmith — forge shell completions from a CLI's --help output.

Usage:
  tabsmith gen [options] <tool>       generate completion scripts
  tabsmith inspect [options] <tool>   print the parsed command spec as JSON
  tabsmith version                    print the tabsmith version

Options (gen):
  --shell <bash|zsh|fish|all>  target dialect (default: all; needs --out)
  --out <dir>                  write script files into dir instead of stdout
  --from <file|->              parse this help text instead of running <tool>
  --name <name>                tool name to complete (required with --from -)
  --depth <n>                  subcommand levels to probe (default: 2)
  --timeout <seconds>          per-invocation probe timeout (default: 5)

Options (inspect): --from, --name, --depth, --timeout as above.

Examples:
  tabsmith gen --shell fish kubectl > ~/.config/fish/completions/kubectl.fish
  tabsmith gen --out completions mytool
  mytool --help | tabsmith gen --from - --name mytool --shell zsh
`

// Run executes the CLI and returns the process exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return 2
	}
	switch args[0] {
	case "gen":
		return runGen(args[1:], stdin, stdout, stderr)
	case "inspect":
		return runInspect(args[1:], stdin, stdout, stderr)
	case "version", "--version", "-V":
		fmt.Fprintf(stdout, "tabsmith %s\n", version.Version)
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usageText)
		return 0
	default:
		fmt.Fprintf(stderr, "tabsmith: unknown command %q (try: tabsmith help)\n", args[0])
		return 2
	}
}

// probeFlags are the options shared by gen and inspect.
type probeFlags struct {
	from    string
	name    string
	depth   int
	timeout float64
}

func addProbeFlags(fs *flag.FlagSet, pf *probeFlags) {
	fs.StringVar(&pf.from, "from", "", "")
	fs.StringVar(&pf.name, "name", "", "")
	fs.IntVar(&pf.depth, "depth", 2, "")
	fs.Float64Var(&pf.timeout, "timeout", 5, "")
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usageText) }
	return fs
}

func runGen(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var pf probeFlags
	var shellName, outDir string
	fs := newFlagSet("gen", stderr)
	addProbeFlags(fs, &pf)
	fs.StringVar(&shellName, "shell", "all", "")
	fs.StringVar(&outDir, "out", "", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cmd, code := buildSpec(fs.Args(), pf, stdin, stderr)
	if cmd == nil {
		return code
	}

	shells := gen.Shells
	if shellName != "all" {
		sh, err := gen.ParseShell(shellName)
		if err != nil {
			fmt.Fprintf(stderr, "tabsmith: %v\n", err)
			return 2
		}
		shells = []gen.Shell{sh}
	} else if outDir == "" {
		fmt.Fprintln(stderr, "tabsmith: --shell all writes multiple scripts; add --out <dir>, or pick one shell for stdout")
		return 2
	}

	flags, subs := cmd.Stats()
	fmt.Fprintf(stderr, "tabsmith: parsed %s: %d flags, %d subcommands\n", cmd.Name, flags, subs)

	for _, sh := range shells {
		script, err := gen.Generate(sh, cmd)
		if err != nil {
			fmt.Fprintf(stderr, "tabsmith: %v\n", err)
			return 2
		}
		if outDir == "" {
			fmt.Fprint(stdout, script)
			continue
		}
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			fmt.Fprintf(stderr, "tabsmith: %v\n", err)
			return 2
		}
		path := filepath.Join(outDir, gen.FileName(sh, cmd.Name))
		if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
			fmt.Fprintf(stderr, "tabsmith: %v\n", err)
			return 2
		}
		fmt.Fprintf(stderr, "tabsmith: wrote %s\n", path)
	}
	return 0
}

func runInspect(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var pf probeFlags
	fs := newFlagSet("inspect", stderr)
	addProbeFlags(fs, &pf)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cmd, code := buildSpec(fs.Args(), pf, stdin, stderr)
	if cmd == nil {
		return code
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cmd); err != nil {
		fmt.Fprintf(stderr, "tabsmith: %v\n", err)
		return 2
	}
	return 0
}

// buildSpec produces the command tree from either a help-text file
// (--from) or a live probe of the named tool. A nil return means the
// caller should exit with the accompanying code.
func buildSpec(rest []string, pf probeFlags, stdin io.Reader, stderr io.Writer) (*spec.Command, int) {
	var cmd *spec.Command
	switch {
	case pf.from != "":
		if len(rest) > 0 {
			fmt.Fprintln(stderr, "tabsmith: --from replaces the <tool> argument; drop one of them")
			return nil, 2
		}
		text, name, err := readFrom(pf.from, pf.name, stdin)
		if err != nil {
			fmt.Fprintf(stderr, "tabsmith: %v\n", err)
			return nil, 2
		}
		cmd = helptext.Parse(name, text)
	case len(rest) == 1:
		var err error
		cmd, err = probe.Probe(rest[0], probe.Options{
			Depth: pf.depth,
			Runner: probe.ExecRunner{
				Timeout: time.Duration(pf.timeout * float64(time.Second)),
			},
		})
		if err != nil {
			fmt.Fprintf(stderr, "tabsmith: %v\n", err)
			return nil, 2
		}
		if pf.name != "" {
			cmd.Name = pf.name
		}
	default:
		fmt.Fprintln(stderr, "tabsmith: expected exactly one <tool> argument (or --from)")
		return nil, 2
	}

	if flags, subs := cmd.Stats(); flags == 0 && subs == 0 && len(cmd.Positionals) == 0 {
		fmt.Fprintf(stderr, "tabsmith: found no flags, subcommands or arguments in %s's help text\n", cmd.Name)
		return nil, 1
	}
	return cmd, 0
}

// readFrom loads help text from a file or stdin ("-") and settles the
// tool name: --name wins, else the file's basename.
func readFrom(from, name string, stdin io.Reader) (text, tool string, err error) {
	var data []byte
	if from == "-" {
		if name == "" {
			return "", "", fmt.Errorf("--from - needs --name <tool>")
		}
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(from)
	}
	if err != nil {
		return "", "", err
	}
	if name == "" {
		base := filepath.Base(from)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if strings.TrimSpace(string(data)) == "" {
		src := from
		if from == "-" {
			src = "stdin"
		}
		return "", "", fmt.Errorf("%s is empty", src)
	}
	return string(data), name, nil
}
