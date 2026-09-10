package report

import (
	"context"
	"errors"
)

type TaskLabels struct {
	Run, Segment, Done, Fail string
}

type ProgressFunc func(done, total int64, unit Unit)

func RunTask(rep Reporter, labels TaskLabels, fn func(progress ProgressFunc) error) error {
	task := rep.Task(labels.Run)
	task.Segment(labels.Segment)
	err := fn(task.Progress)
	if err != nil {
		task.Fail(labels.Fail)
		return err
	}
	task.Done(labels.Done)
	return nil
}

func IsCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
