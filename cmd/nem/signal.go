package main

import (
	"os"
	"os/signal"
)

func watchInterrupts(sig <-chan os.Signal, cancel func(), exit func(int)) {
	if _, ok := <-sig; !ok {
		return
	}
	cancel()
	if _, ok := <-sig; !ok {
		return
	}
	exit(130)
}

func installInterruptWatcher(cancel func()) {
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt)
	go watchInterrupts(sig, cancel, os.Exit)
}
