// End-to-end CLI tests, run fully in-process through cli.Run with fake
// streams. One case drives a real probe against a shell-script tool
// written into a temp dir, covering the exec path without any network.
package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaydenCJ/tabsmith/internal/spec"
)

const fixtureHelp = `Usage: demo [OPTIONS] COMMAND

Options:
  -v, --verbose        noisy
  -o, --output FILE    write here

Commands:
  run    run the demo
`

// run invokes the CLI and returns exit code, stdout and stderr.
func run(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

// helpFile writes the fixture help text into a temp file named demo.txt.
func helpFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "demo.txt")
	if err := os.WriteFile(path, []byte(fixtureHelp), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVersionAndHelpCommands(t *testing.T) {
	code, out, _ := run(t, "", "version")
	if code != 0 || out != "tabsmith 0.1.0\n" {
		t.Fatalf("code=%d out=%q", code, out)
	}
	code, out, _ = run(t, "", "help")
	if code != 0 || !strings.Contains(out, "tabsmith gen [options] <tool>") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestTopLevelUsageErrors(t *testing.T) {
	code, _, errOut := run(t, "")
	if code != 2 || !strings.Contains(errOut, "Usage:") {
		t.Fatalf("no args: code=%d err=%q", code, errOut)
	}
	code, _, errOut = run(t, "", "generate")
	if code != 2 || !strings.Contains(errOut, `unknown command "generate"`) {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
}

func TestGenFromFileToStdout(t *testing.T) {
	code, out, errOut := run(t, "", "gen", "--shell", "bash", "--from", helpFile(t))
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	// The tool name is derived from the file basename (demo.txt → demo).
	if !strings.Contains(out, "complete -F _tabsmith_demo demo") {
		t.Fatalf("stdout is not a bash script:\n%s", out)
	}
	if !strings.Contains(errOut, "parsed demo: 2 flags, 1 subcommands") {
		t.Fatalf("missing summary: %q", errOut)
	}
}

func TestGenFromStdin(t *testing.T) {
	// "-" needs --name (there is no basename to derive from), then works.
	code, _, errOut := run(t, fixtureHelp, "gen", "--shell", "zsh", "--from", "-")
	if code != 2 || !strings.Contains(errOut, "--name") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	code, out, _ := run(t, fixtureHelp, "gen", "--shell", "zsh", "--from", "-", "--name", "demo")
	if code != 0 || !strings.HasPrefix(out, "#compdef demo\n") {
		t.Fatalf("code=%d out=%q", code, out[:40])
	}
}

func TestGenAllShellsToStdoutIsRejected(t *testing.T) {
	code, _, errOut := run(t, "", "gen", "--from", helpFile(t))
	if code != 2 || !strings.Contains(errOut, "--out") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
}

func TestGenAllShellsWritesThreeFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "completions")
	code, _, errOut := run(t, "", "gen", "--out", dir, "--from", helpFile(t))
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	for _, name := range []string{"demo.bash", "_demo", "demo.fish"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func TestGenUsageErrors(t *testing.T) {
	// Unknown shell, --from together with a tool argument, and a blank
	// help file are all usage errors (exit 2) with pointed messages.
	code, _, errOut := run(t, "", "gen", "--shell", "powershell", "--from", helpFile(t))
	if code != 2 || !strings.Contains(errOut, "unknown shell") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	code, _, errOut = run(t, "", "gen", "--shell", "bash", "--from", helpFile(t), "sometool")
	if code != 2 || !strings.Contains(errOut, "--from replaces") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	path := filepath.Join(t.TempDir(), "empty.txt")
	os.WriteFile(path, []byte("  \n"), 0o644)
	code, _, errOut = run(t, "", "gen", "--shell", "bash", "--from", path)
	if code != 2 || !strings.Contains(errOut, "empty") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
}

func TestGenNothingParsedExitsOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prose.txt")
	os.WriteFile(path, []byte("This tool has documentation but lists nothing.\n"), 0o644)
	code, _, errOut := run(t, "", "gen", "--shell", "bash", "--from", path)
	if code != 1 || !strings.Contains(errOut, "found no flags") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
}

func TestInspectEmitsValidSpecJSON(t *testing.T) {
	code, out, _ := run(t, "", "inspect", "--from", helpFile(t))
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	var cmd spec.Command
	if err := json.Unmarshal([]byte(out), &cmd); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if cmd.Name != "demo" || len(cmd.Flags) != 2 || len(cmd.Subcommands) != 1 {
		t.Fatalf("got %+v", cmd)
	}
}

func TestInspectMissingToolArgIsUsageError(t *testing.T) {
	code, _, errOut := run(t, "", "inspect")
	if code != 2 || !strings.Contains(errOut, "exactly one <tool>") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
}

// writeToolScript drops a tiny nested CLI (sh script) into a temp dir so
// the real probe path — os/exec, fallbacks, recursion — is exercised
// offline and deterministically.
func writeToolScript(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	script := `#!/bin/sh
case "$1" in
--help)
    printf 'Usage: mini [OPTIONS] COMMAND\n\nOptions:\n  -v, --verbose   noisy\n\nCommands:\n  go   start it\n'
    ;;
go)
    printf 'Usage: mini go [OPTIONS]\n\nOptions:\n  --fast   hurry\n'
    ;;
*)
    echo "mini: unknown" >&2; exit 2 ;;
esac
`
	path := filepath.Join(t.TempDir(), "mini")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGenProbesARealExecutable(t *testing.T) {
	tool := writeToolScript(t)
	code, out, errOut := run(t, "", "gen", "--shell", "bash", tool)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if !strings.Contains(out, "complete -F _tabsmith_mini mini") ||
		!strings.Contains(out, `"root.go")`) ||
		!strings.Contains(out, "--fast") {
		t.Fatalf("probed script incomplete:\n%s", out)
	}
}

func TestInspectProbedToolReportsNestedFlags(t *testing.T) {
	tool := writeToolScript(t)
	code, out, _ := run(t, "", "inspect", tool)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	var cmd spec.Command
	if err := json.Unmarshal([]byte(out), &cmd); err != nil {
		t.Fatal(err)
	}
	sub := cmd.FindSub("go")
	if sub == nil || len(sub.Flags) != 1 || sub.Flags[0].Long != "--fast" {
		t.Fatalf("got %+v", cmd)
	}
}

func TestGenNameOverrideAppliesToProbedTool(t *testing.T) {
	tool := writeToolScript(t)
	code, out, _ := run(t, "", "gen", "--shell", "fish", "--name", "renamed", tool)
	if code != 0 {
		t.Fatal("gen failed")
	}
	if !strings.Contains(out, "complete -c renamed") {
		t.Fatalf("--name not applied:\n%s", out)
	}
}
