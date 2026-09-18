package report

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func fakeClock(t0 time.Time) (now func() time.Time, advance func(time.Duration)) {
	cur := t0
	return func() time.Time { return cur }, func(d time.Duration) { cur = cur.Add(d) }
}

func TestTaskDoneAtOrOverOneSecondShowsSuffix(t *testing.T) {
	c, _, errb := newTest(Options{Color: ColorNever})
	now, advance := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	c.now = now

	task := c.Task("Installing go v1.26.5")
	advance(3 * time.Second)
	task.Done("Installed go v1.26.5")

	got := errb.String()
	if !strings.Contains(got, "OK Installed go v1.26.5 (3s)\n") {
		t.Errorf("done line missing duration suffix: %q", got)
	}
}

func TestTaskFailEmitsUnderQuiet(t *testing.T) {
	c, _, errb := newTest(Options{Quiet: true, Color: ColorNever})
	task := c.Task("Installing go v1.26.5")
	task.Fail("Install go v1.26.5 failed")

	if !strings.Contains(errb.String(), "ERROR Install go v1.26.5 failed\n") {
		t.Errorf("quiet suppressed a task failure: %q", errb.String())
	}
}

func TestTaskFailColoredShowsGlyph(t *testing.T) {
	c, _, errb := newTest(Options{Color: ColorAlways})
	task := c.Task("Installing go v1.26.5")
	task.Fail("Install go v1.26.5 failed")

	if !strings.Contains(errb.String(), "✗") {
		t.Errorf("expected unicode glyph in colored fail, got %q", errb.String())
	}
}

func TestTaskDoneSuppressedByQuiet(t *testing.T) {
	c, _, errb := newTest(Options{Quiet: true, Color: ColorNever})
	task := c.Task("Installing go v1.26.5")
	task.Done("Installed go v1.26.5")

	if errb.Len() != 0 {
		t.Errorf("quiet did not suppress task success: %q", errb.String())
	}
}

func TestTaskDoneAfterFailIsNoOp(t *testing.T) {
	c, _, errb := newTest(Options{Color: ColorNever})
	task := c.Task("Installing go v1.26.5")
	task.Fail("Install go v1.26.5 failed")
	errb.Reset()

	task.Done("Installed go v1.26.5")

	if errb.Len() != 0 {
		t.Errorf("Done after Fail must be a no-op (cancel-races-success), got %q", errb.String())
	}
}

func TestTaskDiscardEmitsNothing(t *testing.T) {
	c, _, errb := newTest(Options{Color: ColorNever})
	task := c.Task("Mirroring curl")
	task.Segment("probing")

	task.Discard()

	if errb.Len() != 0 {
		t.Errorf("Discard produced output: %q", errb.String())
	}
}

func TestDurSuffix(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, ""},
		{3 * time.Second, " (3s)"},
		{83 * time.Second, " (1m23s)"},
	}
	for _, c := range cases {
		if got := FormatDuration(c.d); got != c.want {
			t.Errorf("DurSuffix(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestTaskConcurrentUpdatesRaceClean(t *testing.T) {
	c, _, _ := newTest(Options{Color: ColorNever})
	task := c.Task("Installing go v1.26.5")

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			task.Segment("downloading")
			task.Progress(int64(i), 100, Bytes)
			task.Progress(int64(i), 10, Items)
		}(i)
	}
	wg.Wait()
	task.Done("Installed go v1.26.5")
}

func TestRunTaskSetsStatusBeforeFnSoCountRenders(t *testing.T) {
	c, _, errb := newLiveTest(Options{IsTTY: true, Color: ColorNever})
	labels := TaskLabels{Run: "Pulling catalog", Segment: "copying", Done: "Pulled catalog", Fail: "Pull failed"}

	err := RunTask(NewContext(context.Background(), c), labels, func(count ProgressFunc) error {
		count(3, 10, Items)
		c.repaint()
		got := errb.String()
		if !strings.Contains(got, "Pulling catalog") || !strings.Contains(got, "copying 3/10") {
			t.Fatalf("live line missing label/status/count: %q", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}
	if got := errb.String(); !strings.Contains(got, "Pulled catalog") {
		t.Errorf("missing Done completion line: %q", got)
	}
}

func TestRunTaskFailReturnsErrUnchanged(t *testing.T) {
	c, _, errb := newTest(Options{Color: ColorNever})
	labels := TaskLabels{Run: "Pushing catalog", Segment: "copying", Done: "Pushed catalog", Fail: "Push failed"}
	sentinel := errors.New("boom")

	err := RunTask(NewContext(context.Background(), c), labels, func(ProgressFunc) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("RunTask error = %v, want sentinel unchanged", err)
	}

	got := errb.String()
	if !strings.Contains(got, "Push failed") {
		t.Errorf("missing Fail line: %q", got)
	}
	if strings.Contains(got, "Pushed catalog") {
		t.Errorf("unexpected Done line on failure: %q", got)
	}
}
