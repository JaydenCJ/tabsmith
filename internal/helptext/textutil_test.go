// Tests for the text-normalization layer. Help output arrives colored,
// tab-aligned, CRLF-terminated or piped through a pager with overstrike;
// every downstream rule assumes these tests hold.
package helptext

import (
	"reflect"
	"testing"
)

func TestStripANSIRemovesCSIAndOSCSequences(t *testing.T) {
	// Colors (CSI) and clap 4's OSC 8 hyperlinks both leak into piped
	// --help; both must vanish byte-cleanly.
	in := "\x1b[1;32mUsage:\x1b[0m tool [OPTIONS]"
	if got := stripANSI(in); got != "Usage: tool [OPTIONS]" {
		t.Fatalf("got %q", got)
	}
	in = "see \x1b]8;;https://example.test\x07docs\x1b]8;;\x07 for more"
	if got := stripANSI(in); got != "see docs for more" {
		t.Fatalf("got %q", got)
	}
}

func TestStripOverstrikeResolvesBoldAndUnderline(t *testing.T) {
	// "OPTIONS" rendered bold by a pager is O\bO P\bP …; underlined
	// words are _\bf _\bi …
	if got := stripOverstrike("O\bOP\bPT\bTI\bIO\bON\bNS\bS"); got != "OPTIONS" {
		t.Fatalf("got %q", got)
	}
	if got := stripOverstrike("_\bf_\bi_\bl_\be"); got != "file" {
		t.Fatalf("got %q", got)
	}
}

func TestExpandTabsIsColumnAccurate(t *testing.T) {
	// BusyBox aligns descriptions with tabs; the "two or more spaces"
	// splitter only works if expansion lands on real 8-column stops.
	if got := expandTabs("\t-q\tquiet"); got != "        -q      quiet" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeStripsCRAndTrailingSpace(t *testing.T) {
	got := normalize("Usage: x  \r\n  -v  verbose \r\n")
	want := []string{"Usage: x", "  -v  verbose", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestIndentAndCollapseHelpers(t *testing.T) {
	if got := indentOf("    -v  verbose"); got != 4 {
		t.Fatalf("indentOf got %d", got)
	}
	if got := indentOf("no indent"); got != 0 {
		t.Fatalf("indentOf got %d", got)
	}
	if got := collapse("  a   wrapped\n   description "); got != "a wrapped description" {
		t.Fatalf("collapse got %q", got)
	}
}
