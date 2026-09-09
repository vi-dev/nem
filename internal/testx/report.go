package testx

import (
	"fmt"
	"sync"

	"github.com/vi-dev/nem/internal/report"
)

type Reporter struct {
	mu    sync.Mutex
	tasks []*Task
	infos []string
	warns []string
}

var _ report.Reporter = (*Reporter)(nil)

func (r *Reporter) Info(format string, a ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.infos = append(r.infos, fmt.Sprintf(format, a...))
}

func (r *Reporter) Warn(format string, a ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.warns = append(r.warns, fmt.Sprintf(format, a...))
}

func (r *Reporter) Debug(string, ...any) {}

func (r *Reporter) Task(label string) report.Task {
	t := &Task{Label: label}
	r.mu.Lock()
	r.tasks = append(r.tasks, t)
	r.mu.Unlock()
	return t
}

func (r *Reporter) TaskFor(label string) *Task {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tasks {
		if t.Label == label {
			return t
		}
	}
	return nil
}

func (r *Reporter) Tasks() []*Task {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*Task(nil), r.tasks...)
}

func (r *Reporter) Warns() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.warns...)
}

func (r *Reporter) Infos() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.infos...)
}

type CountCall struct{ Done, Total int }

type ProgressCall struct{ Done, Total int64 }

type Task struct {
	Label string

	mu        sync.Mutex
	statuses  []string
	counts    []CountCall
	progress  []ProgressCall
	done      bool
	failed    bool
	discarded bool
	outcome   string
}

var _ report.Task = (*Task)(nil)

func (t *Task) Status(segment string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.statuses = append(t.statuses, segment)
}

func (t *Task) Progress(done, total int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.progress = append(t.progress, ProgressCall{done, total})
}

func (t *Task) Count(done, total int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts = append(t.counts, CountCall{done, total})
}

func (t *Task) Done(outcome string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.done = true
	t.outcome = outcome
}

func (t *Task) Fail(outcome string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
	t.outcome = outcome
}

func (t *Task) Discard() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.discarded = true
}

func (t *Task) Snapshot() (done, failed bool, outcome string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.done, t.failed, t.outcome
}

func (t *Task) Discarded() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.discarded
}

func (t *Task) Counts() []CountCall {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]CountCall(nil), t.counts...)
}

func (t *Task) Statuses() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.statuses...)
}

func (t *Task) ProgressCalls() []ProgressCall {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]ProgressCall(nil), t.progress...)
}
