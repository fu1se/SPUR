// Package sqlite provides persistent port.PeerRepository,
// port.NetworkRepository and port.RoomRepository implementations backed
// by SQLite (modernc.org/sqlite — pure Go, no cgo, matches the rest of
// this module's build requirements). This replaces adapter/repository/
// memory as the server's default storage: server state now survives a
// restart instead of resetting every time the process is killed. Rooms
// are persisted for the same reason peers/networks are and pairing codes
// deliberately aren't: a room is meant to outlive any single process
// restart, that's the whole point of it over a 10-minute pairing code.
//
// CandidateStore, RelayBroker, SessionRepository and PairingCodeStore
// stay in-memory on purpose: they hold short-lived, in-flight
// coordination state (an in-progress candidate exchange, a live relay
// splice, a pairing code with a hard TTL) that is meaningless after a
// restart anyway — there is nothing to persist.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// schema is applied on every Open via CREATE TABLE IF NOT EXISTS /
// CREATE INDEX IF NOT EXISTS, so opening an existing database is a no-op
// migration and opening a fresh path bootstraps it.
const schema = `
CREATE TABLE IF NOT EXISTS peers (
	id TEXT PRIMARY KEY,
	public_key BLOB NOT NULL,
	last_seen_at INTEGER NOT NULL,
	candidates TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS networks (
	name TEXT PRIMARY KEY,
	cidr TEXT NOT NULL,
	invite_token TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS network_members (
	network_name TEXT NOT NULL REFERENCES networks(name) ON DELETE CASCADE,
	peer_id TEXT NOT NULL,
	public_key BLOB NOT NULL,
	mesh_ip TEXT NOT NULL,
	PRIMARY KEY (network_name, peer_id)
);

CREATE TABLE IF NOT EXISTS rooms (
	name TEXT PRIMARY KEY,
	invite_token TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS room_members (
	room_name TEXT NOT NULL REFERENCES rooms(name) ON DELETE CASCADE,
	peer_id TEXT NOT NULL,
	PRIMARY KEY (room_name, peer_id)
);
`

// Open opens (creating if necessary) a SQLite database at path and applies
// schema. busy_timeout makes concurrent writers block-and-retry instead of
// immediately failing with SQLITE_BUSY — the server has many goroutines
// (one per client stream) potentially writing at once, and NetworkRepository.Update's
// atomicity (see port.NetworkRepository's doc comment) depends on writers
// actually serializing rather than erroring out under contention.
// foreign_keys is on so deleting a network cascades to its members.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}

	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: apply schema: %w", err)
	}

	return db, nil
}
