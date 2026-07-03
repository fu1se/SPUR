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

		overall := fmt.Sprintf("всего: %s", humanBytes(overallDone))
		if overallTotal > 0 {
			overall = fmt.Sprintf("всего: %s/%s (%.0f%%)", humanBytes(overallDone), humanBytes(overallTotal), 100*float64(overallDone)/float64(overallTotal))
		}

		fmt.Fprintf(w, "\r\033[K%s %s: %s/%s (%.0f%%) — %s/с — %s",
			verb, name, humanBytes(fileDone), humanBytes(fileTotal), filePct, humanBytes(int64(speed)), overall)
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
