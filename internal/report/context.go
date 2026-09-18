package report

import (
	"context"
	"errors"
)

type ctxKey struct{}

var noop Reporter = Discard()

// NewContext returns a copy of ctx carrying r.
func NewContext(ctx context.Context, r Reporter) context.Context {
	return context.WithValue(ctx, ctxKey{}, r)
}

// FromContext returns the Reporter carried by ctx, or a silent
// Reporter if none is attached. It never returns nil.
func FromContext(ctx context.Context) Reporter {
	if r, ok := ctx.Value(ctxKey{}).(Reporter); ok {
		return r
	}
	return noop
}

func IsCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
