#!/usr/bin/env bash
# End-to-end smoke test for tabsmith. No network, idempotent, runs from a
# clean tree. This script plus 'go test ./...' is the whole verification
# story — the repository intentionally ships no CI.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

fail() {
  echo "SMOKE FAIL: $*" >&2
  exit 1
}

BIN="$WORKDIR/tabsmith"
OUT="$WORKDIR/completions"

echo "[1/9] build"
(cd "$ROOT" && go build -o "$BIN" ./cmd/tabsmith) || fail "build failed"

echo "[2/9] version matches the manifest version"
VERSION_OUT="$("$BIN" version)"
[ "$VERSION_OUT" = "tabsmith 0.1.0" ] || fail "unexpected version output: $VERSION_OUT"

echo "[3/9] probe the demo tool and generate all three shells"
# Capture instead of piping into grep -q: -q exits at the first match and
# would SIGPIPE the generator mid-write under pipefail.
GEN_OUT="$("$BIN" gen --out "$OUT" "$ROOT/examples/shipctl" 2>&1)"
echo "$GEN_OUT" | grep -q "parsed shipctl: 13 flags, 4 subcommands" || fail "probe summary wrong"
for f in shipctl.bash _shipctl shipctl.fish; do
  [ -s "$OUT/$f" ] || fail "missing generated file $f"
done

echo "[4/9] generated bash script is valid and knows the nested tree"
bash -n "$OUT/shipctl.bash" || fail "bash script has syntax errors"
grep -q '"root.deploy/history") node=' "$OUT/shipctl.bash" || fail "nested walk arm missing"

echo "[5/9] live bash completion: flag values from parsed choices"
REPLY="$(bash --noprofile --norc -c '
  source "'"$OUT"'/shipctl.bash"
  COMP_WORDS=(shipctl deploy --strategy can); COMP_CWORD=3
  _tabsmith_shipctl
  printf "%s\n" "${COMPREPLY[@]}"
')"
[ "$REPLY" = "canary" ] || fail "expected canary, got: $REPLY"

echo "[6/9] live bash completion: nested subcommand flags"
REPLY="$(bash --noprofile --norc -c '
  source "'"$OUT"'/shipctl.bash"
  COMP_WORDS=(shipctl deploy history --js); COMP_CWORD=3
  _tabsmith_shipctl
  printf "%s\n" "${COMPREPLY[@]}"
')"
[ "$REPLY" = "--json" ] || fail "expected --json, got: $REPLY"

echo "[7/9] zsh and fish scripts carry the right dialect markers"
head -1 "$OUT/_shipctl" | grep -q '^#compdef shipctl$' || fail "zsh compdef tag missing"
grep -q -- "-l strategy -x -a 'bluegreen canary rolling'" "$OUT/shipctl.fish" \
  || fail "fish choice completion missing"
if command -v zsh >/dev/null 2>&1; then
  zsh -n "$OUT/_shipctl" || fail "zsh script has syntax errors"
fi

echo "[8/9] inspect emits the spec as JSON; --from - reads piped help"
INSPECT_OUT="$("$BIN" inspect "$ROOT/examples/shipctl")"
echo "$INSPECT_OUT" | grep -q '"name": "history"' \
  || fail "inspect JSON missing nested subcommand"
ZSH_OUT="$("$ROOT/examples/shipctl" --help | "$BIN" gen --from - --name shipctl --shell zsh 2>/dev/null)"
[ "$(echo "$ZSH_OUT" | head -1)" = "#compdef shipctl" ] || fail "stdin pipeline failed"

echo "[9/9] exit codes: 2 for usage errors, 1 for unusable help text"
set +e
"$BIN" frobnicate >/dev/null 2>&1; [ $? -eq 2 ] || fail "unknown command must exit 2"
printf 'just prose, no flags\n' | "$BIN" gen --from - --name x --shell bash >/dev/null 2>&1
[ $? -eq 1 ] || fail "unusable help must exit 1"
set -e

echo "SMOKE OK"
