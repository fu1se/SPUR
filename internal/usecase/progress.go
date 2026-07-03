package usecase

import "io"

// TransferProgress is called periodically while SendFiles/ReceiveFiles
// move bytes, so a caller (the CLI) can render a progress bar and speed
// without this package knowing anything about terminals. relPath is the
// file currently in flight; fileDone/fileTotal describe just that file,
// overallDone/overallTotal the whole transfer. Both sides know the real
// overallTotal before any content is copied: SendFiles from Source.List(),
// ReceiveFiles from the manifest phase (see file_transfer_wire.go) it
// reads before content starts arriving. nil is valid and means "don't
// report" — same nil-safe-callback pattern as controlserver.Server.Logger
// elsewhere in this codebase.
type TransferProgress func(relPath string, fileDone, fileTotal, overallDone, overallTotal int64)

// ResumeOffer is called by ReceiveFiles when the sender's manifest shows
// at least one file the destination already has some or all bytes of
// (see port.FileSink.ExistingSize) — filesWithData is how many of those
// there are, alreadyHave/total are byte counts across the whole
// transfer. Returning true resumes every such file from where it left
// off; false (or OnResumeOffer being nil) starts every file fresh,
// matching pre-resume behavior exactly. Asked once for the whole
// transfer, not per file — a single interrupted transfer is the
// expected shape of "why does the destination already have some of
// this", not an unrelated leftover file for every entry individually.
type ResumeOffer func(filesWithData int, alreadyHave, total int64) bool

// progressChunkSize bounds how often onChunk fires: io.CopyN's default
// internal buffer would call the underlying Read/Write in similarly
// sized chunks anyway, so this doesn't trade away throughput for
// reporting granularity.
const progressChunkSize = 32 * 1024

// copyWithProgress copies exactly n bytes from src to dst, calling
// onChunk after every write with the cumulative bytes copied so far by
// this call. onChunk is nil-safe (skipped if nil) so callers that don't
// care about progress don't pay for the extra call.
func copyWithProgress(dst io.Writer, src io.Reader, n int64, onChunk func(copied int64)) error {
	buf := make([]byte, progressChunkSize)
	var done int64
	for done < n {
		toRead := int64(len(buf))
		if remaining := n - done; remaining < toRead {
			toRead = remaining
		}
		read, err := io.ReadFull(src, buf[:toRead])
		if read > 0 {
			if _, werr := dst.Write(buf[:read]); werr != nil {
				return werr
			}
			done += int64(read)
			if onChunk != nil {
				onChunk(done)
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}
