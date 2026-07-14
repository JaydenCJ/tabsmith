// Tests for the probing engine, driven through a fake Runner so every
// case is offline and deterministic. The fake maps exact argv strings to
// canned stdout/stderr, mirroring how real binaries answer --help.
package probe

import (
	"errors"
	"strings"
	"testing"
)

// fakeRunner replays canned responses; unknown argv returns an
// "unknown command" banner on stderr like a real CLI would.
type fakeRunner struct {
	responses map[string]response
	calls     []string
}

type response struct {
	stdout string
	stderr string
	err    error
}

func (r *fakeRunner) Run(argv []string) (string, string, error) {
	key := strings.Join(argv, " ")
	r.calls = append(r.calls, key)
	if resp, ok := r.responses[key]; ok {
		return resp.stdout, resp.stderr, resp.err
	}
	return "", "error: unknown command\n", nil
}

const rootHelp = `Usage: ship [OPTIONS] COMMAND

Options:
  -v, --verbose   noisy

Commands:
  deploy   roll out a release
  status   show status
`

const deployHelp = `Usage: ship deploy [OPTIONS]

Options:
  --strategy KIND   one of: rolling, canary

Commands:
  history   list past deployments
`

const historyHelp = `Usage: ship deploy history

Options:
  --json   machine output
`

func shipRunner() *fakeRunner {
	return &fakeRunner{responses: map[string]response{
		"ship --help":                {stdout: rootHelp},
		"ship deploy --help":         {stdout: deployHelp},
		"ship deploy history --help": {stdout: historyHelp},
		"ship status --help":         {stdout: rootHelp}, // ignores the arg
	}}
}

func TestProbeParsesRootHelp(t *testing.T) {
	cmd, err := Probe("ship", Options{Runner: shipRunner()})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name != "ship" || len(cmd.Flags) != 1 || len(cmd.Subcommands) != 2 {
		t.Fatalf("got %+v", cmd)
	}
}

func TestProbeWalksIntoSubcommands(t *testing.T) {
	cmd, err := Probe("ship", Options{Runner: shipRunner(), Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	deploy := cmd.FindSub("deploy")
	if deploy == nil || len(deploy.Flags) != 1 || deploy.Flags[0].Long != "--strategy" {
		t.Fatalf("got %+v", deploy)
	}
	history := deploy.FindSub("history")
	if history == nil || len(history.Flags) != 1 {
		t.Fatalf("nested probe missing: %+v", deploy)
	}
}

func TestProbeKeepsListingSummaryOverSubHelp(t *testing.T) {
	cmd, _ := Probe("ship", Options{Runner: shipRunner(), Depth: 2})
	if got := cmd.FindSub("deploy").Summary; got != "roll out a release" {
		t.Fatalf("got %q", got)
	}
}

func TestProbeDetectsIgnoredArgumentByFingerprint(t *testing.T) {
	// "ship status --help" re-prints the root help; treating it as real
	// would nest the whole tree inside itself, forever.
	cmd, _ := Probe("ship", Options{Runner: shipRunner(), Depth: 2})
	status := cmd.FindSub("status")
	if len(status.Flags) != 0 || len(status.Subcommands) != 0 {
		t.Fatalf("ignored-arg screen must leave a leaf stub: %+v", status)
	}
}

func TestProbeDepthZeroParsesOnlyRootHelp(t *testing.T) {
	// --depth 0 promises "root help only": exactly one invocation, and
	// the listed subcommands stay stubs (name + summary, nothing probed).
	r := shipRunner()
	cmd, err := Probe("ship", Options{Runner: r, Depth: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 1 || r.calls[0] != "ship --help" {
		t.Fatalf("depth 0 must probe once, got %v", r.calls)
	}
	if deploy := cmd.FindSub("deploy"); deploy == nil || len(deploy.Flags) != 0 {
		t.Fatalf("depth 0 must leave stubs: %+v", deploy)
	}
}

func TestProbeDepthLimitStopsRecursion(t *testing.T) {
	cmd, _ := Probe("ship", Options{Runner: shipRunner(), Depth: 1})
	deploy := cmd.FindSub("deploy")
	if len(deploy.Flags) != 1 {
		t.Fatalf("depth 1 must still fill direct children: %+v", deploy)
	}
	if history := deploy.FindSub("history"); len(history.Flags) != 0 {
		t.Fatalf("depth 1 must not probe grandchildren: %+v", history)
	}
}

func TestProbeFallbackHelpSpellings(t *testing.T) {
	// Tools that only answer -h, or only a bare "help" subcommand, must
	// still probe: the spellings are tried in order.
	for _, answering := range []string{"tool -h", "tool help"} {
		r := &fakeRunner{responses: map[string]response{
			answering: {stdout: "Usage: tool\n\nOptions:\n  -x   do x\n"},
		}}
		cmd, err := Probe("tool", Options{Runner: r})
		if err != nil || len(cmd.Flags) != 1 {
			t.Fatalf("%s: err=%v cmd=%+v", answering, err, cmd)
		}
	}
}

func TestProbeAcceptsUsageOnStderr(t *testing.T) {
	// getopt-era tools print usage on stderr and exit non-zero.
	r := &fakeRunner{responses: map[string]response{
		"old --help": {stderr: "usage: old [-abc] [file ...]\n  -a   all\n"},
	}}
	cmd, err := Probe("old", Options{Runner: r})
	if err != nil || len(cmd.Flags) != 1 {
		t.Fatalf("err=%v cmd=%+v", err, cmd)
	}
}

func TestProbeFailureModes(t *testing.T) {
	// Error banners are not help; an unrunnable binary is a hard error.
	r := &fakeRunner{responses: map[string]response{
		"tool --help": {stderr: "tool: fatal: cannot open database\n"},
	}}
	if _, err := Probe("tool", Options{Runner: r}); err == nil {
		t.Fatal("non-help output everywhere must fail the probe")
	}
	notFound := response{err: errors.New("executable not found")}
	r = &fakeRunner{responses: map[string]response{
		"gone --help": notFound, "gone -h": notFound, "gone help": notFound,
	}}
	if _, err := Probe("gone", Options{Runner: r}); err == nil {
		t.Fatal("want an error for an unrunnable tool")
	}
}

func TestProbeMaxProbesCapsInvocations(t *testing.T) {
	r := shipRunner()
	if _, err := Probe("ship", Options{Runner: r, Depth: 2, MaxProbes: 1}); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("MaxProbes=1 must allow exactly one run, got %v", r.calls)
	}
}

func TestProbeUsesBasenameForToolPath(t *testing.T) {
	r := &fakeRunner{responses: map[string]response{
		"./bin/ship.exe --help": {stdout: rootHelp},
	}}
	cmd, err := Probe("./bin/ship.exe", Options{Runner: r, Depth: 1, MaxProbes: 3})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name != "ship" {
		t.Fatalf("got name %q", cmd.Name)
	}
}

func TestHelpShapedAcceptsAndRejects(t *testing.T) {
	yes := []string{
		"Usage: x [OPTIONS]",
		"Options:\n  -v  verbose",
		"some tool\n\n  --long-flag",
	}
	no := []string{"", "   \n", "error: unknown command", "42"}
	for _, s := range yes {
		if !helpShaped(s) {
			t.Fatalf("must accept %q", s)
		}
	}
	for _, s := range no {
		if helpShaped(s) {
			t.Fatalf("must reject %q", s)
		}
	}
}

func TestProbeNeverExecutesDashPrefixedSubcommands(t *testing.T) {
	// Defensive: if the parser ever mislabels a flag as a subcommand, the
	// prober must not turn it into an argument.
	r := &fakeRunner{responses: map[string]response{
		"x --help": {stdout: "Usage: x\n\nCommands:\n  ok   fine\n"},
	}}
	Probe("x", Options{Runner: r, Depth: 2})
	for _, call := range r.calls {
		if strings.Contains(call, "x - ") {
			t.Fatalf("dash-prefixed probe issued: %v", r.calls)
		}
	}
}
