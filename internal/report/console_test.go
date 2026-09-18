package report

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTest(opts Options) (*Console, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return New(&out, &errb, opts), &out, &errb
}

func TestLinesAndStreams(t *testing.T) {
	c, out, errb := newTest(Options{Color: ColorNever})
	c.Success("Installed go v1.26.5")
	c.Warn("Catalog dev is stale")
	c.Info("Resolving")
	c.Debug("hidden without verbose")

	if out.Len() != 0 {
		t.Fatalf("narration leaked to stdout: %q", out.String())
	}
	got := errb.String()
	for _, want := range []string{"OK Installed go v1.26.5\n", "WARN Catalog dev is stale\n", "Resolving\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("stderr missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "hidden") {
		t.Errorf("debug shown without verbose")
	}
}

func TestQuietSuppressesNarrationNotDiagnostics(t *testing.T) {
	c, _, errb := newTest(Options{Quiet: true, Color: ColorNever})
	c.Info("nope")
	c.Success("nope")
	c.Warn("still here")
	got := errb.String()
	if strings.Contains(got, "nope") {
		t.Errorf("quiet did not suppress narration: %q", got)
	}
	if !strings.Contains(got, "WARN still here") {
		t.Errorf("quiet suppressed diagnostics: %q", got)
	}
}

func TestErrorCapitalizesDisplayOnly(t *testing.T) {
	c, _, errb := newTest(Options{Color: ColorNever})
	c.Error(errors.New("parse nem.toml: bad value"), "Run `nem status`")
	got := errb.String()
	if !strings.Contains(got, "ERROR Parse nem.toml: bad value\n") {
		t.Errorf("error line wrong: %q", got)
	}
	if !strings.Contains(got, "  hint: Run `nem status`\n") {
		t.Errorf("hint line wrong: %q", got)
	}
}

func TestUnicodeGlyphsWhenColored(t *testing.T) {
	c, _, errb := newTest(Options{Color: ColorAlways})
	c.Success("Done")
	if !strings.Contains(errb.String(), "✓") {
		t.Errorf("expected unicode glyph, got %q", errb.String())
	}
}

func TestHintSuppressedByQuiet(t *testing.T) {
	c, _, errb := newTest(Options{Color: ColorNever})
	c.Hint("Run `nem catalog update dev` to sync it")
	if !strings.Contains(errb.String(), "  hint: Run `nem catalog update dev` to sync it\n") {
		t.Errorf("hint line wrong: %q", errb.String())
	}

	cq, _, errbq := newTest(Options{Quiet: true, Color: ColorNever})
	cq.Hint("nope")
	if strings.Contains(errbq.String(), "nope") {
		t.Errorf("quiet did not suppress hint: %q", errbq.String())
	}
}

func TestPromptIgnoresQuiet(t *testing.T) {
	c, out, errb := newTest(Options{Quiet: true, Color: ColorNever})
	c.Prompt("Remove these? [y/N]: ")
	if errb.String() != "Remove these? [y/N]: \n" {
		t.Errorf("quiet suppressed a question the command blocks on: %q", errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("prompt leaked to stdout: %q", out.String())
	}
}

func TestColorAutoFollowsTTY(t *testing.T) {
	c, _, errb := newTest(Options{Color: ColorAuto, IsTTY: true})
	c.Success("On")
	if !strings.Contains(errb.String(), "✓") {
		t.Errorf("ColorAuto with TTY should color: %q", errb.String())
	}
	c2, _, errb2 := newTest(Options{Color: ColorAuto, IsTTY: false})
	c2.Success("Off")
	if !strings.Contains(errb2.String(), "OK Off") {
		t.Errorf("ColorAuto without TTY should be plain: %q", errb2.String())
	}
}

func TestNarrationIsGoroutineSafe(t *testing.T) {
	c, _, _ := newTest(Options{Color: ColorNever, Verbose: true})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			c.Success("s")
			c.Warn("w")
			c.Debug("d")
			c.Hint("h")
		})
	}
	wg.Wait()
}

func TestTable(t *testing.T) {
	c, out, _ := newTest(Options{Color: ColorNever})
	c.Table([]string{"package", "version"}, [][]string{
		{"go", "v1.26.5"},
		{"kubectl", "v1.34.1"},
	})
	want := "PACKAGE  VERSION\ngo       v1.26.5\nkubectl  v1.34.1\n"
	if out.String() != want {
		t.Errorf("table:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestDataGoesToStdoutVerbatim(t *testing.T) {
	c, out, errb := newTest(Options{Color: ColorNever})
	c.Data("%s v%s\n", "go", "1.26.5")
	if got, want := out.String(), "go v1.26.5\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if errb.Len() != 0 {
		t.Errorf("data leaked to stderr: %q", errb.String())
	}
}

func TestDataIgnoresQuiet(t *testing.T) {
	c, out, _ := newTest(Options{Quiet: true, Color: ColorNever})
	c.Data("payload\n")
	if out.String() != "payload\n" {
		t.Errorf("quiet suppressed data output: %q", out.String())
	}
}

func TestDataNeverColored(t *testing.T) {
	c, out, _ := newTest(Options{Color: ColorAlways})
	c.Data("plain\n")
	if strings.Contains(out.String(), "\x1b[") {
		t.Errorf("data output contains ANSI codes: %q", out.String())
	}
}

func TestJSONGoesToStdoutIndented(t *testing.T) {
	c, out, errb := newTest(Options{Color: ColorNever})
	if err := c.JSON(map[string]string{"name": "go"}); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if got, want := out.String(), "{\n  \"name\": \"go\"\n}\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if errb.Len() != 0 {
		t.Errorf("json leaked to stderr: %q", errb.String())
	}
}

func TestJSONIgnoresQuiet(t *testing.T) {
	c, out, _ := newTest(Options{Quiet: true, Color: ColorNever})
	if err := c.JSON([]int{1}); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if out.Len() == 0 {
		t.Errorf("quiet suppressed json output")
	}
}

func TestGuardedWriteClearsAndRepaintsAroundTheLiveBlock(t *testing.T) {
	c, _, errb := newLiveTest(Options{IsTTY: true, Color: ColorNever})
	tk := c.Task("Building erlang v27.2")
	tk.Segment("./make.bash")
	c.repaint()
	errb.Reset()

	fmt.Fprint(c.ErrOut(), "configure: checking for cc\n")

	got := errb.String()
	const wantClear = "\x1b[1A\x1b[J"
	_, after, ok := strings.Cut(got, wantClear)
	if !ok {
		t.Fatalf("expected clear sequence %q before the streamed payload, got %q", wantClear, got)
	}
	payloadIdx := strings.Index(after, "configure: checking for cc")
	if payloadIdx == -1 {
		t.Fatalf("streamed payload missing after the clear sequence: %q", after)
	}
	repaintIdx := strings.Index(after, "Building erlang v27.2")
	if repaintIdx == -1 {
		t.Fatalf("live block not repainted after the streamed payload: %q", after)
	}
	if payloadIdx > repaintIdx {
		t.Errorf("payload should land above the repainted block: %q", after)
	}
	if !strings.Contains(after[payloadIdx:repaintIdx], "\x1b[2K") {
		t.Errorf("repaint after the payload missing clear-line sequence: %q", after)
	}

	tk.Done("Built erlang v27.2")
}

func TestGuardedWritersStayIntactWhileALiveTaskIsActive(t *testing.T) {
	c, out, errb := newTest(Options{IsTTY: true, Color: ColorNever})
	c.tick = time.Millisecond

	tk := c.Task("Building erlang v27.2")
	tk.Segment("./make.bash")

	const streams, lines = 4, 25
	var wg sync.WaitGroup
	for s := range streams {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range lines {
				fmt.Fprintf(c.ErrOut(), "err-%d-%d\n", s, i)
				fmt.Fprintf(c.Out(), "out-%d-%d\n", s, i)
			}
		}()
	}
	wg.Wait()
	tk.Done("Built erlang v27.2")

	gotErr := errb.String()
	for s := range streams {
		for i := range lines {

			want := fmt.Sprintf("\x1b[1A\x1b[Jerr-%d-%d\n\x1b[2K", s, i)
			if n := strings.Count(gotErr, want); n != 1 {
				t.Fatalf("payload err-%d-%d not framed by clear+repaint exactly once (got %d)", s, i, n)
			}
		}
	}

	gotOut := out.String()
	if strings.Contains(gotOut, "\x1b") {
		t.Errorf("live-block escapes leaked into stdout: %q", gotOut)
	}
	for s := range streams {
		for i := range lines {
			want := fmt.Sprintf("out-%d-%d\n", s, i)
			if n := strings.Count(gotOut, want); n != 1 {
				t.Fatalf("stdout payload %q written %d times, want 1", want, n)
			}
		}
	}
}

func TestStreamAccessorsAreTheConstructorsWriters(t *testing.T) {
	c, out, errb := newTest(Options{Color: ColorNever})

	if _, err := io.WriteString(c.Out(), "artifact.tar.gz\n"); err != nil {
		t.Fatalf("Out write: %v", err)
	}
	if _, err := io.WriteString(c.ErrOut(), "cc: warning\n"); err != nil {
		t.Fatalf("ErrOut write: %v", err)
	}

	if got := out.String(); got != "artifact.tar.gz\n" {
		t.Errorf("Out wrote %q to the stdout buffer, want the payload verbatim", got)
	}
	if got := errb.String(); got != "cc: warning\n" {
		t.Errorf("ErrOut wrote %q to the stderr buffer, want the payload verbatim", got)
	}
}

func TestDiscardConsoleStreamsDiscard(t *testing.T) {
	c := Discard()
	n, err := io.WriteString(c.Out(), "dropped")
	if err != nil || n != len("dropped") {
		t.Fatalf("Out write = %d, %v; want a full, error-free discard", n, err)
	}
	if n, err = io.WriteString(c.ErrOut(), "dropped"); err != nil || n != len("dropped") {
		t.Fatalf("ErrOut write = %d, %v; want a full, error-free discard", n, err)
	}
}

func newLiveTest(opts Options) (*Console, *bytes.Buffer, *bytes.Buffer) {
	c, out, errb := newTest(opts)
	c.tick = time.Hour
	return c, out, errb
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 'A' || s[j] > 'Z') && (s[j] < 'a' || s[j] > 'z') {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestLiveSingleTaskShowsLabelAndSegment(t *testing.T) {
	c, _, errb := newLiveTest(Options{IsTTY: true, Color: ColorNever})
	tk := c.Task("Installing go v1.26.5")
	tk.Segment("downloading")
	c.repaint()

	got := errb.String()
	if !strings.Contains(got, "Installing go v1.26.5") || !strings.Contains(got, "downloading") {
		t.Errorf("live line missing label/segment: %q", got)
	}
	if !strings.Contains(got, "\x1b[2K") {
		t.Errorf("live line missing clear-line sequence: %q", got)
	}
	tk.Done("Installed go v1.26.5")
}

func TestLiveProgressAloneRendersNothingUntilStatusIsSet(t *testing.T) {
	c, _, errb := newLiveTest(Options{IsTTY: true, Color: ColorNever})
	tk := c.Task("Syncing catalog")
	tk.Progress(3, 10, Items)
	c.repaint()

	if got := errb.String(); strings.Contains(got, "3/10") {
		t.Fatalf("Progress alone must not render: %q", got)
	}

	tk.Segment("copying")
	c.repaint()
	if got := errb.String(); !strings.Contains(got, "copying 3/10") {
		t.Fatalf("Status set after Progress must render the count alongside it: %q", got)
	}
	tk.Done("Synced catalog")
}

func TestLiveDiscardShrinksBlockWithNoCompletionLine(t *testing.T) {
	c, _, errb := newLiveTest(Options{IsTTY: true, Color: ColorNever})
	first := c.Task("Mirroring curl")
	first.Segment("probing")
	second := c.Task("Mirroring go")
	second.Segment("copying 1.26.5")
	c.repaint()
	errb.Reset()

	first.Discard()

	got := errb.String()
	if strings.Contains(got, "Mirroring curl") {
		t.Errorf("discarded task's line still present: %q", got)
	}
	if strings.Contains(got, "✓") || strings.Contains(got, "✗") {
		t.Errorf("Discard rendered a completion glyph: %q", got)
	}
	if !strings.Contains(got, "Mirroring go") {
		t.Errorf("surviving task not repainted after the discard: %q", got)
	}

	second.Done("Mirrored go (1 tag copied)")
}

func TestLiveTwoTasksInStartOrder(t *testing.T) {
	c, _, errb := newLiveTest(Options{IsTTY: true, Color: ColorNever})
	first := c.Task("Installing go v1.26.5")
	first.Segment("downloading")
	second := c.Task("Installing kubectl v1.34.1")
	second.Segment("extracting")
	c.repaint()

	got := errb.String()
	goIdx := strings.Index(got, "Installing go v1.26.5")
	kubectlIdx := strings.Index(got, "Installing kubectl v1.34.1")
	if goIdx == -1 || kubectlIdx == -1 {
		t.Fatalf("expected both task lines, got %q", got)
	}
	if goIdx > kubectlIdx {
		t.Errorf("tasks not painted in start order: %q", got)
	}

	first.Done("Installed go v1.26.5")
	second.Done("Installed kubectl v1.34.1")
}

func TestLiveDonePrintsCompletionAboveAndRemovesLine(t *testing.T) {
	c, _, errb := newLiveTest(Options{IsTTY: true, Color: ColorAlways})
	first := c.Task("Installing go v1.26.5")
	first.Segment("downloading")
	second := c.Task("Installing kubectl v1.34.1")
	second.Segment("extracting")
	c.repaint()
	errb.Reset()

	first.Done("Installed go v1.26.5")

	got := errb.String()

	const wantClear = "\x1b[2A\x1b[J"
	_, after0, ok := strings.Cut(got, wantClear)
	if !ok {
		t.Fatalf("expected cursor-up-to-top + erase sequence %q, got %q", wantClear, got)
	}
	if strings.Contains(got, "\x1b[2K\n\x1b[2K\n") {
		t.Errorf("cursor left mid-block by a per-line clear loop instead of a single erase: %q", got)
	}

	after := after0
	doneIdx := strings.Index(after, "✓")
	if doneIdx == -1 {
		t.Fatalf("completion line missing right after the clear sequence: %q", after)
	}
	remainIdx := strings.Index(after, "Installing kubectl v1.34.1")
	if remainIdx == -1 {
		t.Fatalf("remaining task line missing after Done: %q", after)
	}
	if doneIdx > remainIdx {
		t.Errorf("completion did not print above the repainted block: %q", after)
	}
	if strings.Contains(after[:remainIdx], "Installing go v1.26.5") {
		t.Errorf("finished task's line was not removed from the block: %q", after)
	}

	second.Done("Installed kubectl v1.34.1")
}

func TestLiveTruncatesAtWidth(t *testing.T) {
	c, _, errb := newLiveTest(Options{IsTTY: true, Color: ColorNever})
	c.width = func() int { return 20 }
	tk := c.Task("Installing go v1.26.5")
	tk.Segment("downloading a very long segment name")
	c.repaint()

	got := errb.String()
	for line := range strings.SplitSeq(got, "\n") {
		plain := stripANSI(line)
		if n := len([]rune(plain)); n > 20 {
			t.Errorf("line exceeds width 20 (%d runes): %q", n, plain)
		}
	}
	tk.Done("Installed go v1.26.5")
}

func TestLiveAutoElapsedAfterTenSeconds(t *testing.T) {
	c, _, errb := newLiveTest(Options{IsTTY: true, Color: ColorNever})
	now, advance := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	c.now = now

	tk := c.Task("Building erlang v27.2")
	tk.Segment("./make.bash")
	advance(11 * time.Second)
	c.repaint()

	got := errb.String()
	if !strings.Contains(got, "(11s)") {
		t.Errorf("expected auto-elapsed suffix after 11s of silence: %q", got)
	}
	tk.Done("Built erlang v27.2")
}

func TestInfoDuringLiveBlockClearsPrintsAndRepaints(t *testing.T) {
	c, _, errb := newLiveTest(Options{IsTTY: true, Color: ColorNever})
	tk := c.Task("Installing go v1.26.5")
	tk.Segment("downloading")
	c.repaint()
	errb.Reset()

	c.Info("Resolving catalog dev")

	got := errb.String()
	const wantClear = "\x1b[1A\x1b[J"
	_, after0, ok := strings.Cut(got, wantClear)
	if !ok {
		t.Fatalf("expected clear sequence %q before the info line, got %q", wantClear, got)
	}
	after := after0
	infoIdx := strings.Index(after, "Resolving catalog dev")
	if infoIdx == -1 {
		t.Fatalf("info line missing after the clear sequence: %q", after)
	}
	repaintIdx := strings.Index(after, "Installing go v1.26.5")
	if repaintIdx == -1 {
		t.Fatalf("live block not repainted after the info line: %q", after)
	}
	if infoIdx > repaintIdx {
		t.Errorf("info line should print above the repainted block: %q", after)
	}
	if !strings.Contains(after[infoIdx:repaintIdx], "\x1b[2K") {
		t.Errorf("repaint after info missing clear-line sequence: %q", after)
	}

	tk.Done("Installed go v1.26.5")
}

func TestLiveBlockAbsentWhenNotTTY(t *testing.T) {
	c, _, errb := newLiveTest(Options{IsTTY: false, Color: ColorNever})
	tk := c.Task("Installing go v1.26.5")
	tk.Segment("downloading")
	c.repaint()

	if errb.Len() != 0 {
		t.Errorf("non-TTY console painted a live block: %q", errb.String())
	}
	tk.Done("Installed go v1.26.5")
}

func TestLiveBlockAbsentWhenQuiet(t *testing.T) {
	c, _, errb := newLiveTest(Options{IsTTY: true, Quiet: true, Color: ColorNever})
	tk := c.Task("Installing go v1.26.5")
	tk.Segment("downloading")
	c.repaint()

	if strings.Contains(errb.String(), "downloading") {
		t.Errorf("quiet console painted a live block: %q", errb.String())
	}
	tk.Done("Installed go v1.26.5")
}
