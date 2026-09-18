package testx

import (
	"context"
	"testing"

	"github.com/vi-dev/nem/internal/report"
)

func TestReporterContextCarriesTheReturnedReporter(t *testing.T) {
	ctx, rep := ReporterContext(context.Background())
	if got := report.FromContext(ctx); got != report.Reporter(rep) {
		t.Fatalf("context carries %v, want the returned *Reporter", got)
	}
}
