package report

import (
	"fmt"
	"time"
)

func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return ""
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf(" (%ds)", int(d.Seconds()))
	}
	return fmt.Sprintf(" (%dm%02ds)", int(d.Minutes()), int(d.Seconds())%60)
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
