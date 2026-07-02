package infra

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSaveTrustStore_ConcurrentWritesNeverCorruptReaders is a regression
// test for a real bug found running `app connect` and `app expose`
// against each other live: both processes pin the same server at nearly
// the same time, and os.WriteFile truncates a file before writing its new
// contents — a reader landing in that window got "unexpected end of JSON
// input". saveTrustStore must write atomically (temp file + rename) so a
// concurrent reader always sees either the fully-old or fully-new
// content, no matter how many writers race. This test is in package
// infra (not infra_test) because it needs saveTrustStore/loadTrustStore
// directly, without going through TOFUClientTLSConfig's in-process
// tofuMu — the whole point is to reproduce a cross-process race, which a
// shared in-process mutex wouldn't exhibit.
func TestSaveTrustStore_ConcurrentWritesNeverCorruptReaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_servers.json")

	const writers = 20
	var wg sync.WaitGroup
	writeErrCh := make(chan error, writers)

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			writeErrCh <- saveTrustStore(path, map[string]string{"server": "fingerprint"})
		}()
	}

	stop := make(chan struct{})
	readErrCh := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				readErrCh <- nil
				return
			default:
			}
			if _, err := loadTrustStore(path); err != nil {
				readErrCh <- err
				return
			}
		}
	}()

	wg.Wait()
	close(stop)

	for i := 0; i < writers; i++ {
		require.NoError(t, <-writeErrCh)
	}
	require.NoError(t, <-readErrCh, "a concurrent reader observed a corrupt/partial trust store file")
}
