// Package probe runs a real binary's help commands and hands the text to
// the parser, recursively: `tool --help` first, then `tool sub --help`
// for every subcommand the parser discovered, down to a depth limit.
//
// Probing is defensive by design. It only ever appends help-shaped
// arguments (--help, -h, help) — never a bare subcommand invocation — and
// it fingerprints each help screen so tools that ignore unknown arguments
// and re-print the root help do not produce phantom nested trees.
package probe

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/JaydenCJ/tabsmith/internal/helptext"
	"github.com/JaydenCJ/tabsmith/internal/spec"
)

// Runner abstracts process execution so tests can substitute a fake.
type Runner interface {
	// Run executes argv and returns captured stdout and stderr. A non-zero
	// exit is not an error here: plenty of tools exit 1 or 2 from --help.
	// err is reserved for "could not run at all" (not found, timeout).
	Run(argv []string) (stdout, stderr string, err error)
}

// ExecRunner is the real Runner: it executes the binary with a hard
// timeout so a tool that treats "--help" as "start serving" cannot hang
// the probe.
type ExecRunner struct {
	Timeout time.Duration
}

// Run implements Runner via os/exec with a context deadline.
func (r ExecRunner) Run(argv []string) (string, string, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var out, errOut strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", "", fmt.Errorf("timed out after %s: %s", timeout, strings.Join(argv, " "))
	}
	if _, isExit := err.(*exec.ExitError); err != nil && !isExit {
		return "", "", err // not found, not executable, …
	}
	return out.String(), errOut.String(), nil
}

// Options tunes a probe run.
type Options struct {
	// Depth is how many subcommand levels below the root to walk;
	// 0 parses only the root help. The CLI defaults it to 2.
	Depth int
	// MaxProbes caps the total number of process invocations, so a tool
	// with hundreds of listed commands cannot turn one probe into a storm.
	// The default is 64.
	MaxProbes int
	// Runner defaults to ExecRunner{}.
	Runner Runner
}

// helpSpellings are tried in order until one produces help-shaped output.
var helpSpellings = [][]string{{"--help"}, {"-h"}, {"help"}}

// Probe builds the full command spec for tool. The returned tree is named
// after the binary's basename regardless of how the tool was addressed.
func Probe(tool string, opts Options) (*spec.Command, error) {
	if opts.MaxProbes == 0 {
		opts.MaxProbes = 64
	}
	if opts.Runner == nil {
		opts.Runner = ExecRunner{}
	}

	p := &prober{opts: opts, seen: map[string]bool{}}
	name := strings.TrimSuffix(filepath.Base(tool), filepath.Ext(filepath.Base(tool)))
	if name == "" || name == "." {
		name = tool
	}

	text, err := p.helpText([]string{tool})
	if err != nil {
		return nil, err
	}
	if text == "" {
		return nil, fmt.Errorf("%s produced no help output (tried --help, -h, help)", tool)
	}
	root := helptext.Parse(name, text)
	p.seen[fingerprint(text)] = true
	p.walk(root, []string{tool}, opts.Depth)
	return root, nil
}

type prober struct {
	opts   Options
	probes int
	seen   map[string]bool // help-screen fingerprints already parsed
}

// walk fills in each listed subcommand by probing `base sub --help`.
func (p *prober) walk(node *spec.Command, base []string, depth int) {
	if depth <= 0 {
		return
	}
	for _, sub := range node.Subcommands {
		if strings.HasPrefix(sub.Name, "-") {
			continue // never let a parsed oddity become an argument
		}
		argv := append(append([]string{}, base...), sub.Name)
		text, err := p.helpText(argv)
		if err != nil || text == "" {
			continue // leave the stub: name + summary still complete fine
		}
		fp := fingerprint(text)
		if p.seen[fp] {
			// Identical to a screen we already parsed: the tool ignored
			// the subcommand and re-printed some other help. Treat as leaf.
			continue
		}
		p.seen[fp] = true
		parsed := helptext.Parse(sub.Name, text)
		if parsed.Summary != "" && sub.Summary == "" {
			sub.Summary = parsed.Summary
		}
		sub.Usage = parsed.Usage
		sub.Flags = parsed.Flags
		sub.Positionals = parsed.Positionals
		sub.Subcommands = parsed.Subcommands
		p.walk(sub, argv, depth-1)
	}
}

// helpText tries each help spelling appended to argv and returns the
// first help-shaped output, preferring stdout over stderr (getopt-era
// tools print usage on stderr).
func (p *prober) helpText(argv []string) (string, error) {
	var firstErr error
	for _, sp := range helpSpellings {
		if p.probes >= p.opts.MaxProbes {
			return "", firstErr
		}
		p.probes++
		full := append(append([]string{}, argv...), sp...)
		stdout, stderr, err := p.opts.Runner.Run(full)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if helpShaped(stdout) {
			return stdout, nil
		}
		if helpShaped(stderr) {
			return stderr, nil
		}
	}
	return "", firstErr
}

// helpShaped is the acceptance test for candidate output: it must mention
// usage, a known section header, or contain at least one option-looking
// line. Error banners like "unknown command" fail all three.
func helpShaped(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "usage:") || strings.Contains(lower, "usage ") {
		return true
	}
	for _, h := range []string{"options:", "options\n", "commands:", "flags:", "arguments:"} {
		if strings.Contains(lower, h) {
			return true
		}
	}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "-") && len(t) > 1 && t[1] != '-' &&
			strings.Contains(t, "  ") {
			return true
		}
		if strings.HasPrefix(t, "--") && len(t) > 2 {
			return true
		}
	}
	return false
}

// fingerprint hashes a help screen for the ignored-argument check.
func fingerprint(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return fmt.Sprintf("%x", sum[:8])
}
