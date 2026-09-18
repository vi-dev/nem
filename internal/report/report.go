// Package report renders nem's terminal output on two planes: narration
// (progress, tasks, warnings, errors) on stderr, and machine-consumable
// data (tables, JSON) on stdout. Commands hold a *Console; internal
// packages take the narration Reporter from the context (FromContext),
// attached once per command invocation in cmd/nem.
package report

import "io"

type Reporter interface {
	Info(format string, a ...any)
	Success(format string, a ...any)
	Warn(format string, a ...any)
	Error(err error, hint string)
	Debug(format string, a ...any)
	Hint(msg string)
	Task(label string) Task

	Out() io.Writer
	ErrOut() io.Writer
}

type Unit int

const (
	Items Unit = iota
	Bytes
)

type Task interface {
	Segment(segment string)
	Progress(done, total int64, unit Unit)
	Done(outcome string)
	Fail(outcome string)
	Discard()
}

var _ Reporter = (*Console)(nil)
