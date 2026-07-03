package cli

import (
	"fmt"
	"io"
	"time"
)

// newProgressPrinter returns a ProgressFunc that renders a single,
// redrawn status line to w: the current file's progress, overall
// progress (when known), and a rolling transfer speed. verb is
// "отправка"/"приём" etc. Throttled to a few redraws a second (except
// the very last update for a given file, always shown) so a fast local
// transfer doesn't spend more time drawing than copying bytes.
func newProgressPrinter(w io.Writer, verb string) ProgressFunc {
	const minInterval = 100 * time.Millisecond
	const speedWindow = 500 * time.Millisecond

	var (
		lastPrint  time.Time
		speedFrom  time.Time
		speedBytes int64
		speed      float64
	)

	return func(relPath string, fileDone, fileTotal, overallDone, overallTotal int64) {
		now := time.Now()
		final := fileDone == fileTotal
		if !final && now.Sub(lastPrint) < minInterval {
			return
		}
		lastPrint = now

		if speedFrom.IsZero() {
			speedFrom, speedBytes = now, overallDone
		} else if elapsed := now.Sub(speedFrom); elapsed >= speedWindow || final {
			if secs := elapsed.Seconds(); secs > 0 {
				speed = float64(overallDone-speedBytes) / secs
			}
			speedFrom, speedBytes = now, overallDone
		}

		name := relPath
		if len(name) > 40 {
			name = "…" + name[len(name)-39:]
		}

		var filePct float64
		if fileTotal > 0 {
			filePct = 100 * float64(fileDone) / float64(fileTotal)
		}

		overall := fmt.Sprintf(msg().ProgressOverallNoTotal, humanBytes(overallDone))
		if overallTotal > 0 {
			overall = fmt.Sprintf(msg().ProgressOverallWithTotal, humanBytes(overallDone), humanBytes(overallTotal), 100*float64(overallDone)/float64(overallTotal))
		}

		eta := ""
		if remaining := overallTotal - overallDone; overallTotal > 0 && remaining > 0 && speed > 0 {
			eta = fmt.Sprintf(msg().ProgressETASuffix, formatETA(float64(remaining)/speed))
		}

		fmt.Fprintf(w, msg().ProgressLine,
			verb, name, humanBytes(fileDone), humanBytes(fileTotal), filePct, humanBytes(int64(speed)), overall, eta)
	}
}

// formatETA renders a remaining-time estimate in seconds as a short
// human string ("~45с"/"~45s", "~3м 20с"/"~3m 20s", "~2ч 05м"/"~2h 05m")
// — coarse on purpose: an ETA derived from a short rolling speed sample
// (see speedWindow) is already a rough guess, and displaying more
// precision than that would just be noise that flickers between redraws.
func formatETA(seconds float64) string {
	d := time.Duration(seconds) * time.Second
	switch {
	case d < time.Minute:
		return fmt.Sprintf(msg().ETASeconds, int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf(msg().ETAMinutes, int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf(msg().ETAHours, int(d.Hours()), int(d.Minutes())%60)
	}
}

// progressDone moves the cursor to a fresh line after a progress printer
// has been (or might have been) used, so whatever the caller prints next
// — a success message, an error — doesn't land in the middle of the last
// redrawn progress line.
func progressDone(w io.Writer) {
	fmt.Fprintln(w)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
