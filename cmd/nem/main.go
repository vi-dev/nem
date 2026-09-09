package main

import (
	"context"
	"errors"
	"fmt"
	"os"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	installInterruptWatcher(cancel)

	root := newRoot()
	err := root.ExecuteContext(ctx)
	cancel()
	if err == nil {
		return
	}

	if exitErr, ok := errors.AsType[*ExitError](err); ok {
		os.Exit(exitErr.Code)
	}
	if errors.Is(err, context.Canceled) {
		os.Exit(130)
	}
	if ranHook && console != nil {
		console.Error(err, hintFor(err))
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "Error:", err) // nolint
	os.Exit(2)
}
