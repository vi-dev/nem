package report

import (
	"fmt"
	"sync"
	"time"
)

type Reporter interface {
	Info(format string, a ...any)
	Warn(format string, a ...any)
	Debug(format string, a ...any)
	Task(label string) Task
}

type Task interface {
	Status(segment string)
	Progress(done, total int64)
	Count(done, total int)
	Done(outcome string)
	Fail(outcome string)
	Discard()
}

var _ Reporter = (*Console)(nil)

type task struct {
	console *Console
	label   string
	start   time.Time

	mu           sync.Mutex
	segment      string
	segmentStart time.Time
	done         int64
	total        int64
	cdone        int
	ctotal       int
	completed    bool
}

func (c *Console) Task(label string) Task {
	t := &task{console: c, label: label, start: c.now()}
	if c.liveActive() {
		c.registerLiveTask(t)
	}
	return t
}

func (t *task) Status(segment string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.segment = segment
	t.segmentStart = t.console.now()
}

func (t *task) Progress(done, total int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.done, t.total = done, total
}

func (t *task) Count(done, total int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cdone, t.ctotal = done, total
}

func (t *task) Done(outcome string) {
	if !t.markCompleted() {
		return
	}
	elapsed := t.console.now().Sub(t.start)
	t.console.completeTask(t, func() {
		t.console.successLocked(outcome + DurSuffix(elapsed))
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

func DurSuffix(d time.Duration) string {
	if d < time.Second {
		return ""
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf(" (%ds)", int(d.Seconds()))
	}
	return fmt.Sprintf(" (%dm%02ds)", int(d.Minutes()), int(d.Seconds())%60)
}
