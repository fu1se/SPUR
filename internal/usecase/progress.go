package usecase

import "io"

// TransferProgress is called periodically while SendFiles/ReceiveFiles
// move bytes, so a caller (the CLI) can render a progress bar and speed
// without this package knowing anything about terminals. relPath is the
// file currently in flight; fileDone/fileTotal describe just that file,
// overallDone/overallTotal the whole transfer. overallTotal is 0 when
// unknown — ReceiveFiles never sees the sender's full file list up front
// (the wire protocol streams one header at a time, see
// file_transfer_wire.go), so it can only report a running overallDone,
// not a total to measure it against; SendFiles always knows the real
// total from Source.List(). nil is valid and means "don't report" — same
// nil-safe-callback pattern as controlserver.Server.Logger elsewhere in
// this codebase.
type TransferProgress func(relPath string, fileDone, fileTotal, overallDone, overallTotal int64)

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
