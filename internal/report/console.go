package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/term"
)

type Mode int

const (
	ColorAuto Mode = iota
	ColorAlways
	ColorNever
)

type Options struct {
	Quiet   bool
	Verbose bool
	Color   Mode
	IsTTY   bool
}

type Console struct {
	out, err io.Writer
	opts     Options
	colored  bool

	now func() time.Time

	width func() int

	tick time.Duration

	liveMu    sync.Mutex
	liveTasks []*task
	liveLines int
	liveStop  chan struct{}
}

func New(stdout, stderr io.Writer, opts Options) *Console {
	colored := opts.Color == ColorAlways || (opts.Color == ColorAuto && opts.IsTTY)
	return &Console{
		out:     stdout,
		err:     stderr,
		opts:    opts,
		colored: colored,
		now:     time.Now,
		width:   terminalWidth,
		tick:    liveTickInterval,
	}
}

func terminalWidth() int {
	w, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || w <= 0 {
		return defaultWidth
	}
	return w
}

func Discard() *Console { return New(io.Discard, io.Discard, Options{Color: ColorNever}) }

const (
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiDim    = "\x1b[2m"
	ansiReset  = "\x1b[0m"
)

func (c *Console) Info(format string, a ...any) {
	if c.opts.Quiet {
		return
	}
	c.narrate(func() { fmt.Fprintf(c.err, format+"\n", a...) })
}

func (c *Console) Debug(format string, a ...any) {
	if !c.opts.Verbose {
		return
	}
	c.dim(fmt.Sprintf(format, a...))
}

func (c *Console) Success(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	c.narrate(func() { c.successLocked(msg) })
}

func (c *Console) successLocked(msg string) {
	if c.opts.Quiet {
		return
	}
	if c.colored {
		fmt.Fprintf(c.err, "%s✓%s %s\n", ansiGreen, ansiReset, msg)
	} else {
		fmt.Fprintf(c.err, "OK %s\n", msg)
	}
}

func (c *Console) Warn(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	c.narrate(func() {
		if c.colored {
			fmt.Fprintf(c.err, "%s!%s %s\n", ansiYellow, ansiReset, msg)
		} else {
			fmt.Fprintf(c.err, "WARN %s\n", msg)
		}
	})
}

func (c *Console) Error(err error, hint string) {
	msg := capitalizeLead(err.Error())
	c.narrate(func() {
		if c.colored {
			fmt.Fprintf(c.err, "%s✗ %s%s\n", ansiRed, msg, ansiReset)
		} else {
			fmt.Fprintf(c.err, "ERROR %s\n", msg)
		}
		if hint != "" {
			c.hintLocked(hint)
		}
	})
}

func (c *Console) Hint(msg string) {
	if c.opts.Quiet {
		return
	}
	c.narrate(func() { c.hintLocked(msg) })
}

func (c *Console) hintLocked(msg string) {
	if c.colored {
		fmt.Fprintf(c.err, "  %s→ %s%s\n", ansiDim, msg, ansiReset)
	} else {
		fmt.Fprintf(c.err, "  hint: %s\n", msg)
	}
}

func (c *Console) Prompt(format string, a ...any) {
	c.narrate(func() { fmt.Fprintf(c.err, format+"\n", a...) })
}

func (c *Console) Out() io.Writer { return &guardedWriter{console: c, w: c.out} }

func (c *Console) ErrOut() io.Writer { return &guardedWriter{console: c, w: c.err} }

type guardedWriter struct {
	console *Console
	w       io.Writer
}

func (g *guardedWriter) Write(p []byte) (int, error) {
	var (
		n   int
		err error
	)
	g.console.narrate(func() { n, err = g.w.Write(p) })
	return n, err
}

func (c *Console) Data(format string, a ...any) {
	fmt.Fprintf(c.out, format, a...)
}

func (c *Console) JSON(v any) error {
	enc := json.NewEncoder(c.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (c *Console) Blank() {
	fmt.Fprintln(c.out)
}

func (c *Console) Table(headers []string, rows [][]string) {
	maxCols := len(headers)
	for _, r := range rows {
		if len(r) > maxCols {
			maxCols = len(r)
		}
	}

	widths := make([]int, maxCols)
	for i, h := range headers {
		if i < maxCols {
			widths[i] = len(h)
		}
	}
	for _, r := range rows {
		for i, cell := range r {
			if i < maxCols && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	line := func(cells []string, upper bool) {
		var b strings.Builder
		for i := 0; i < maxCols; i++ {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			if upper {
				cell = strings.ToUpper(cell)
			}
			b.WriteString(cell)
			if i < maxCols-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-len(cell)+2))
			}
		}
		fmt.Fprintln(c.out, strings.TrimRight(b.String(), " "))
	}
	line(headers, true)
	for _, r := range rows {
		line(r, false)
	}
}

func (c *Console) dim(msg string) {
	c.narrate(func() {
		if c.colored {
			fmt.Fprintf(c.err, "%s%s%s\n", ansiDim, msg, ansiReset)
		} else {
			fmt.Fprintln(c.err, msg)
		}
	})
}

func capitalizeLead(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

const (
	liveTickInterval = 100 * time.Millisecond

	defaultWidth = 80
)

func (c *Console) liveActive() bool {
	return c.opts.IsTTY && !c.opts.Quiet
}

func (c *Console) registerLiveTask(t *task) {
	c.liveMu.Lock()
	defer c.liveMu.Unlock()
	c.liveTasks = append(c.liveTasks, t)
	if len(c.liveTasks) == 1 {
		c.startLiveTickerLocked()
	}
	c.repaintLocked()
}

func (c *Console) completeTask(t *task, printCompletion func()) {
	c.liveMu.Lock()
	defer c.liveMu.Unlock()

	idx := -1
	for i, lt := range c.liveTasks {
		if lt == t {
			idx = i
			break
		}
	}
	if idx == -1 {
		printCompletion()
		return
	}

	c.clearBlockLocked()
	printCompletion()
	c.liveTasks = append(c.liveTasks[:idx], c.liveTasks[idx+1:]...)
	if len(c.liveTasks) == 0 {
		c.stopLiveTickerLocked()
	}
	c.repaintLocked()
}

func (c *Console) narrate(print func()) {
	c.liveMu.Lock()
	defer c.liveMu.Unlock()
	if len(c.liveTasks) == 0 {
		print()
		return
	}
	c.clearBlockLocked()
	print()
	c.repaintLocked()
}

func (c *Console) repaint() {
	c.liveMu.Lock()
	defer c.liveMu.Unlock()
	c.repaintLocked()
}

func (c *Console) repaintLocked() {
	if c.liveLines > 0 {
		fmt.Fprintf(c.err, "\x1b[%dA", c.liveLines)
	}
	now := c.now()
	width := c.width()
	for _, t := range c.liveTasks {
		fmt.Fprintf(c.err, "\x1b[2K%s\n", t.renderLine(now, width, c.colored))
	}
	c.liveLines = len(c.liveTasks)
}

func (c *Console) clearBlockLocked() {
	if c.liveLines == 0 {
		return
	}
	fmt.Fprintf(c.err, "\x1b[%dA\x1b[J", c.liveLines)
	c.liveLines = 0
}

func (c *Console) startLiveTickerLocked() {
	stop := make(chan struct{})
	c.liveStop = stop
	go func() {
		ticker := time.NewTicker(c.tick)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.repaint()
			case <-stop:
				return
			}
		}
	}()
}

func (c *Console) stopLiveTickerLocked() {
	if c.liveStop != nil {
		close(c.liveStop)
		c.liveStop = nil
	}
}
