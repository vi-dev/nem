package report

import (
	"fmt"
	"time"
)

const (
	liveTickInterval = 100 * time.Millisecond

	autoElapsedAfter = 10 * time.Second

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

func (t *task) renderLine(now time.Time, width int, colored bool) string {
	return formatTaskLine(t.label, t.trailingText(now), width, colored)
}

func (t *task) trailingText(now time.Time) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.segment == "" {
		return ""
	}
	if p := progressText(t.done, t.total, t.cdone, t.ctotal); p != "" {
		return t.segment + " " + p
	}
	if elapsed := now.Sub(t.segmentStart); elapsed >= autoElapsedAfter {
		return t.segment + DurSuffix(elapsed)
	}
	return t.segment
}

func progressText(done, total int64, cdone, ctotal int) string {
	switch {
	case ctotal > 0:
		return fmt.Sprintf("%d/%d", cdone, ctotal)
	case total > 0:
		return fmt.Sprintf("%d%%", int(float64(done)/float64(total)*100))
	case total < 0:
		return FormatBytes(done)
	default:
		return ""
	}
}

func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.0f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func formatTaskLine(label, trailing string, width int, colored bool) string {
	plain := label
	if trailing != "" {
		plain += "  " + trailing
	}
	truncated := truncateToWidth(plain, width)
	if !colored || trailing == "" {
		return truncated
	}
	labelLen := len([]rune(label))
	truncRunes := []rune(truncated)
	prefixLen := labelLen + 2
	if prefixLen >= len(truncRunes) {
		return truncated
	}
	return string(truncRunes[:prefixLen]) + ansiDim + string(truncRunes[prefixLen:]) + ansiReset
}

func truncateToWidth(s string, width int) string {
	if width <= 0 {
		width = defaultWidth
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width])
}
