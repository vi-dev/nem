package mirror

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/testx"
)

type inFlightGauge struct {
	cur atomic.Int64
	max atomic.Int64
}

func (g *inFlightGauge) start() {
	n := g.cur.Add(1)
	for {
		m := g.max.Load()
		if n <= m || g.max.CompareAndSwap(m, n) {
			return
		}
	}
}

func (g *inFlightGauge) end() { g.cur.Add(-1) }

func wireArchivesWithSrcDelay(t *testing.T, delay time.Duration, gauge *inFlightGauge) {
	t.Helper()
	src := testx.NewArchiveFixtures()
	dst := testx.NewArchiveFixtures()
	t.Cleanup(SetSrcArchivesOpener(func(_, name string) (oras.ReadOnlyTarget, error) {
		gauge.start()
		time.Sleep(delay)
		gauge.end()
		return src.Open(name), nil
	}))
	t.Cleanup(SetDstArchivesOpener(func(_, name string) (oras.Target, error) {
		return dst.Open(name), nil
	}))
}

func TestRunPackagesFanOutInParallel(t *testing.T) {
	limit := min(runtime.NumCPU(), 8)
	if limit < 2 {
		t.Skip("needs at least two concurrent slots to distinguish parallel from serial")
	}

	const n = 32
	const delay = 100 * time.Millisecond

	pkgs := map[string]string{}
	for i := range n {
		name := fmt.Sprintf("pkg%02d", i)
		pkgs[name] = urlPkg(name, "1.0.0", "deadbeef")
	}
	srcStore, srcTag := newCatalog(t, pkgs)
	wireCatalog(t, srcStore, srcTag, memory.New(), "v2")

	var gauge inFlightGauge
	wireArchivesWithSrcDelay(t, delay, &gauge)

	opts := Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2"}
	start := time.Now()
	summary, err := Run(context.Background(), opts, &testx.Reporter{})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Packages != n {
		t.Fatalf("Packages = %d, want %d", summary.Packages, n)
	}

	serialFloor := n * delay
	idealWaves := (n + limit - 1) / limit

	parallelCeiling := time.Duration(idealWaves+3) * delay
	if elapsed >= serialFloor {
		t.Fatalf("elapsed %s is not faster than the fully-serialized floor %s: no parallelism", elapsed, serialFloor)
	}
	if elapsed > parallelCeiling {
		t.Fatalf("elapsed %s exceeds the parallel-%d ceiling %s", elapsed, limit, parallelCeiling)
	}

	maxInFlight := int(gauge.max.Load())
	if maxInFlight > limit {
		t.Fatalf("max in-flight = %d, exceeds SetLimit's bound %d", maxInFlight, limit)
	}
	if maxInFlight < limit {
		t.Fatalf("max in-flight = %d, want exactly the SetLimit bound %d (n=%d is well above it)", maxInFlight, limit, n)
	}

	t.Logf("n=%d delay=%s limit=%d elapsed=%s serialFloor=%s parallelCeiling=%s maxInFlight=%d",
		n, delay, limit, elapsed, serialFloor, parallelCeiling, maxInFlight)
}

func TestRunTasksRegisterBeforeProbingCompletes(t *testing.T) {
	limit := min(runtime.NumCPU(), 8)
	if limit < 2 {
		t.Skip("needs at least two concurrent slots to observe concurrent registration")
	}

	const n = 32
	const delay = 200 * time.Millisecond

	pkgs := map[string]string{}
	for i := range n {
		name := fmt.Sprintf("pkg%02d", i)
		pkgs[name] = urlPkg(name, "1.0.0", "deadbeef")
	}
	srcStore, srcTag := newCatalog(t, pkgs)
	wireCatalog(t, srcStore, srcTag, memory.New(), "v2")

	var gauge inFlightGauge
	wireArchivesWithSrcDelay(t, delay, &gauge)

	rep := &testx.Reporter{}
	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_, _ = Run(context.Background(), Options{SrcRef: "example.com/cat:v2", DstRef: "internal.example.com/cat:v2"}, rep)
	}()

	fanOutDeadline := time.After(5 * time.Second)
waitForFullFanOut:
	for {
		select {
		case <-fanOutDeadline:
			t.Fatalf("in-flight probes never reached the parallel bound %d (stuck at %d)", limit, gauge.cur.Load())
		case <-runDone:
			t.Fatal("Run finished before any probe delay elapsed; delay wiring is broken")
		default:
			if int(gauge.cur.Load()) == limit {
				break waitForFullFanOut
			}
			time.Sleep(time.Millisecond)
		}
	}

	tasks := rep.Tasks()
	if len(tasks) < limit {
		t.Fatalf("task count = %d while %d probes are in flight, want at least %d", len(tasks), limit, limit)
	}
	for _, task := range tasks {
		if task.Label == "Pulling catalog" || task.Label == "Pushing catalog" {
			continue
		}
		if done, failed, _ := task.Snapshot(); done || failed {
			t.Fatalf("task %q resolved via Done/Fail while probes were still in flight", task.Label)
		}
		if task.Discarded() {
			t.Fatalf("task %q discarded while probes were still in flight", task.Label)
		}
	}

	<-runDone
}
