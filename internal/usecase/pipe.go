package usecase

import "io"

// pipe copies bytes in both directions between a and b until one side's
// copy ends, then closes both so the other direction unblocks too. Used by
// both port-forward use cases below to splice a local connection to a
// tunnel stream.
func pipe(a, b io.ReadWriteCloser) {
	defer a.Close()
	defer b.Close()

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(a, b); done <- struct{}{} }()
	go func() { _, _ = io.Copy(b, a); done <- struct{}{} }()
	<-done
}
