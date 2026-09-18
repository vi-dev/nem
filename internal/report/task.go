package report

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type task struct {
	console *Console
	label   string
	start   time.Time

	mu           sync.Mutex
	segment      string
	segmentStart time.Time
	done         int64
	total        int64
	unit         Unit
	completed    bool
}

func (c *Console) Task(label string) Task {
	t := &task{console: c, label: label, start: c.now()}
	if c.liveActive() {
		c.registerLiveTask(t)
	}
	return t
}

func (t *task) Segment(segment string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.segment = segment
	t.segmentStart = t.console.now()
}

func (t *task) Progress(done, total int64, unit Unit) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.done, t.total, t.unit = done, total, unit
}

func (t *task) Done(outcome string) {
	if !t.markCompleted() {
		return
	}
	elapsed := t.console.now().Sub(t.start)
	t.console.completeTask(t, func() {
		t.console.successLocked(outcome + FormatDuration(elapsed))
	})
}

func (t *task) Fail(outcome string) {
	if !t.markCompleted() {
		return
	}
	t.console.completeTask(t, func() {
		c := t.console
		if c.colored {
			fmt.Fprintf(c.err, "%s✗ %s%s\n", ansiRed, outcome, ansiReset)
		} else {
			fmt.Fprintf(c.err, "ERROR %s\n", outcome)
		}
	})
}

func (t *task) Discard() {
	if !t.markCompleted() {
		return
	}
	t.console.completeTask(t, func() {})
}

func (t *task) markCompleted() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.completed {
		return false
	}
	t.completed = true
	return true
}

const autoElapsedAfter = 10 * time.Second

func (t *task) renderLine(now time.Time, width int, colored bool) string {
	return formatTaskLine(t.label, t.trailingText(now), width, colored)
}

func (t *task) trailingText(now time.Time) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.segment == "" {
		return ""
	}
	if p := progressText(t.done, t.total, t.unit); p != "" {
		return t.segment + " " + p
	}
	if elapsed := now.Sub(t.segmentStart); elapsed >= autoElapsedAfter {
		return t.segment + FormatDuration(elapsed)
	}
	return t.segment
}

func progressText(done, total int64, unit Unit) string {
	switch {
	case unit == Items && total > 0:
		return fmt.Sprintf("%d/%d", done, total)
	case unit == Items && total < 0:
		return fmt.Sprintf("%d", done)
	case unit == Bytes && total > 0:
		return fmt.Sprintf("%d%%", int(float64(done)/float64(total)*100))
	case unit == Bytes && total < 0:
		return FormatBytes(done)
	default:
		return ""
	}
}

func formatTaskLine(label, trailing string, width int, colored bool) string {
	plain := label
	if trailing != "" {
		plain += "  " + trailing
	}
	truncated := truncateToWidth(plain, width)
	if !colored || trailing == "" {
		return truncated
	}
	labelLen := len([]rune(label))
	truncRunes := []rune(truncated)
	prefixLen := labelLen + 2
	if prefixLen >= len(truncRunes) {
		return truncated
	}
	return string(truncRunes[:prefixLen]) + ansiDim + string(truncRunes[prefixLen:]) + ansiReset
}

func truncateToWidth(s string, width int) string {
	if width <= 0 {
		width = defaultWidth
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width])
}

type TaskLabels struct {
	Run, Segment, Done, Fail string
}

type ProgressFunc func(done, total int64, unit Unit)

func RunTask(ctx context.Context, labels TaskLabels, fn func(progress ProgressFunc) error) error {
	task := FromContext(ctx).Task(labels.Run)
	task.Segment(labels.Segment)
	if err := fn(task.Progress); err != nil {
		task.Fail(labels.Fail)
		return err
	}
	task.Done(labels.Done)
	return nil
}
