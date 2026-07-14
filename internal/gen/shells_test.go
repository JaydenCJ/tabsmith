// Per-shell generator tests. Beyond string assertions, the bash script is
// executed for real: a bash subprocess sources it, fakes COMP_WORDS and
// asserts on COMPREPLY — the same check a human would do by pressing Tab.
package gen

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/JaydenCJ/tabsmith/internal/spec"
)

// ---- bash ----

func TestBashScriptStructure(t *testing.T) {
	out, _ := Generate(Bash, sample())
	if !strings.Contains(out, "complete -F _tabsmith_ship ship") {
		t.Fatalf("missing complete registration:\n%s", out)
	}
	for _, arm := range []string{
		`"root/deploy") node='root.deploy'`,
		`"root.deploy/history") node='root.deploy.history'`,
	} {
		if !strings.Contains(out, arm) {
			t.Fatalf("missing walk arm %q", arm)
		}
	}
}

func TestBashValueCompletionWiring(t *testing.T) {
	out, _ := Generate(Bash, sample())
	if !strings.Contains(out, "compgen -W 'canary rolling'") {
		t.Fatalf("choices not sorted:\n%s", out)
	}
	if !strings.Contains(out, `"root:-c" | "root:--config")`) ||
		!strings.Contains(out, `compgen -f -- "$cur"`) {
		t.Fatalf("file flag not wired to compgen -f:\n%s", out)
	}
}

func TestBashOptionalArgFlagDoesNotEatNextWord(t *testing.T) {
	out, _ := Generate(Bash, sample())
	if strings.Contains(out, `"root:--color"`) {
		t.Fatal("--color[=WHEN] must not appear in the prev-value cases")
	}
}

// runBashCompletion sources the generated script in a real bash, fakes
// the completion state and prints COMPREPLY.
func runBashCompletion(t *testing.T, script string, words ...string) []string {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	var b strings.Builder
	b.WriteString(script)
	b.WriteString("\nCOMP_WORDS=(")
	for _, w := range words {
		b.WriteString(shQuote(w) + " ")
	}
	b.WriteString(")\nCOMP_CWORD=$((${#COMP_WORDS[@]} - 1))\n")
	b.WriteString("_tabsmith_ship\nprintf '%s\\n' \"${COMPREPLY[@]}\"\n")
	out, err := exec.Command("bash", "--noprofile", "--norc", "-c", b.String()).Output()
	if err != nil {
		t.Fatalf("bash run failed: %v", err)
	}
	return strings.Fields(string(out))
}

func TestBashLiveRootSubcommandCompletion(t *testing.T) {
	script, _ := Generate(Bash, sample())
	got := runBashCompletion(t, script, "ship", "de")
	if len(got) != 1 || got[0] != "deploy" {
		t.Fatalf("got %v", got)
	}
}

func TestBashLiveFlagValueCompletion(t *testing.T) {
	script, _ := Generate(Bash, sample())
	got := runBashCompletion(t, script, "ship", "deploy", "--strategy", "can")
	if len(got) != 1 || got[0] != "canary" {
		t.Fatalf("got %v", got)
	}
}

func TestBashLiveNestedFlagCompletion(t *testing.T) {
	script, _ := Generate(Bash, sample())
	got := runBashCompletion(t, script, "ship", "deploy", "history", "--js")
	if len(got) != 1 || got[0] != "--json" {
		t.Fatalf("got %v", got)
	}
}

func TestBashLiveFlagsSkippedDuringNodeWalk(t *testing.T) {
	// A flag between the tool and the subcommand must not derail the walk.
	script, _ := Generate(Bash, sample())
	got := runBashCompletion(t, script, "ship", "--verbose", "deploy", "--strategy", "roll")
	if len(got) != 1 || got[0] != "rolling" {
		t.Fatalf("got %v", got)
	}
}

// ---- zsh ----

func TestZshScriptStructure(t *testing.T) {
	out, _ := Generate(Zsh, sample())
	if !strings.HasPrefix(out, "#compdef ship\n") {
		t.Fatalf("first line must be the compdef tag:\n%s", out[:60])
	}
	for _, fn := range []string{
		"_tabsmith_ship()", "_tabsmith_ship_deploy()",
		"_tabsmith_ship_deploy_history()", "_tabsmith_ship_logs()",
	} {
		if !strings.Contains(out, fn) {
			t.Fatalf("missing function %s", fn)
		}
	}
	if !strings.HasSuffix(out, "_tabsmith_ship \"$@\"\n") {
		t.Fatal("script must invoke the root function")
	}
}

func TestZshOptspecForms(t *testing.T) {
	// Multi-spelling flags use an exclusion group with brace expansion;
	// optional arguments use zsh's :: marker; choices come out sorted.
	out, _ := Generate(Zsh, sample())
	if !strings.Contains(out, "'(-c --config)'{-c+,--config=}'[config file to load]:file:_files'") {
		t.Fatalf("missing grouped optspec:\n%s", out)
	}
	if !strings.Contains(out, "'--color=::when:(always auto never)'") {
		t.Fatalf("optional arg must use :: and sorted choices:\n%s", out)
	}
}

func TestZshSubcommandMenuAndPositionals(t *testing.T) {
	out, _ := Generate(Zsh, sample())
	if !strings.Contains(out, "'deploy:roll a release out'") ||
		!strings.Contains(out, "_describe -t commands 'ship command' commands") {
		t.Fatalf("missing _describe block:\n%s", out)
	}
}

func TestZshEscapesBracketsInDescriptions(t *testing.T) {
	root := &spec.Command{Name: "x", Flags: []spec.Flag{
		{Long: "--mode", Desc: "pick [possible values: a, b]"},
	}}
	out, _ := Generate(Zsh, root)
	if !strings.Contains(out, `\[possible values: a, b\]`) {
		t.Fatalf("unescaped brackets break _arguments:\n%s", out)
	}
}

func TestZshPositionalFileUsesFilesAction(t *testing.T) {
	out, _ := Generate(Zsh, sample())
	if !strings.Contains(out, "'1:file:_files'") {
		t.Fatalf("logs FILE positional must complete files:\n%s", out)
	}
}

func TestZshSyntaxChecksClean(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not available")
	}
	out, _ := Generate(Zsh, sample())
	cmd := exec.Command(zsh, "-n", "/dev/stdin")
	cmd.Stdin = strings.NewReader(out)
	if err := cmd.Run(); err != nil {
		t.Fatalf("zsh -n rejected the script: %v\n%s", err, out)
	}
}

// ---- fish ----

func TestFishEmitsResolverOnlyWhenNested(t *testing.T) {
	out, _ := Generate(Fish, sample())
	if !strings.Contains(out, "function _tabsmith_ship_cmd") {
		t.Fatal("nested tree needs the path resolver")
	}
	flat := &spec.Command{Name: "flat", Flags: []spec.Flag{{Short: "-v"}}}
	out, _ = Generate(Fish, flat)
	if strings.Contains(out, "function") {
		t.Fatalf("flat tool must not emit helper functions:\n%s", out)
	}
}

func TestFishFlagWiring(t *testing.T) {
	// Spellings map to -s/-l, file flags to -r -F, choice flags to a
	// sorted exclusive argument list.
	out, _ := Generate(Fish, sample())
	if !strings.Contains(out, "-s c -l config -r -F") {
		t.Fatalf("file flag wiring wrong:\n%s", out)
	}
	if !strings.Contains(out, "-l strategy -x -a 'canary rolling'") {
		t.Fatalf("choice flag wiring wrong:\n%s", out)
	}
}

func TestFishOldStyleFlagUsesDashO(t *testing.T) {
	root := &spec.Command{Name: "gotool", Flags: []spec.Flag{
		{Long: "-json", Desc: "as JSON"},
	}}
	out, _ := Generate(Fish, root)
	if !strings.Contains(out, "complete -c gotool -o json") {
		t.Fatalf("Go-style flag must use -o:\n%s", out)
	}
}

func TestFishConditionsCoverEveryNodeWithContent(t *testing.T) {
	// "logs" has only a file positional: no line is emitted for it, which
	// deliberately leaves fish's default file completion in charge there.
	out, _ := Generate(Fish, sample())
	for _, cond := range []string{
		`_tabsmith_ship_at ""`, `_tabsmith_ship_at "deploy"`,
		`_tabsmith_ship_at "deploy history"`,
	} {
		if !strings.Contains(out, cond) {
			t.Fatalf("missing condition %q:\n%s", cond, out)
		}
	}
	if strings.Contains(out, `_tabsmith_ship_at "logs"`) {
		t.Fatal("logs must not get explicit completions (file default applies)")
	}
}

func TestFishSubcommandsCarryDescriptions(t *testing.T) {
	out, _ := Generate(Fish, sample())
	if !strings.Contains(out, "-f -a 'deploy' -d 'roll a release out'") {
		t.Fatalf("missing subcommand entry:\n%s", out)
	}
}
