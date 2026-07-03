package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// newResumePrompt returns a ResumeOfferFunc that asks the user on the
// terminal whether to resume a detected partial transfer. Defaults to
// "yes" on a bare Enter — the common case is the user re-running the same
// command specifically to finish an interrupted transfer — and to "no" if
// the input can't be read at all (e.g. stdin isn't a terminal), rather
// than blocking forever waiting for an answer that will never come.
func newResumePrompt(cmd *cobra.Command) ResumeOfferFunc {
	return func(filesWithData int, alreadyHave, total int64) bool {
		out := cmd.ErrOrStderr()
		fmt.Fprintf(out, "Обнаружена незавершённая передача: %d файл(ов), уже получено %s из %s.\n",
			filesWithData, humanBytes(alreadyHave), humanBytes(total))
		fmt.Fprint(out, "Продолжить с того места, где остановились? [Y/n] ")

		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && err != io.EOF {
			return false
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		return answer == "" || answer == "y" || answer == "yes" || answer == "д" || answer == "да"
	}
}
