// Package wgmesh implements wireguard-go's conn.Bind over already-punched
// (or relayed) port.Stream connections to each mesh peer, instead of
// wireguard-go opening its own UDP sockets and doing its own peer
// discovery. NAT traversal already happened by the time a Stream exists
// (see usecase.EstablishSession / adapter/tunnel.Transport) — WireGuard
// itself never needs to know punching or relay exist.
package wgmesh

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/netip"
	"reflect"
	"sync"

	"golang.zx2c4.com/wireguard/conn"

	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/usecase/port"
)

// maxPacketSize bounds a single framed WireGuard packet. WireGuard doesn't
// fragment; real packets are bounded by the tunnel MTU (device.DefaultMTU,
// see NewDevice — not configurable in this codebase) plus its own Noise
// overhead, comfortably under this. It's set well below the frame length
// prefix's own uint16 range (65535) specifically so the "corrupt length
// prefix" check in readFrameLength can actually trigger — a bound equal to
// the field's own max value can never reject anything.
const maxPacketSize = 2048

// Endpoint identifies a mesh peer as a WireGuard conn.Endpoint. There's no
// IP:port here on purpose: addressing was already resolved by
// EstablishSession, all that's left is "which peer's Stream do I write
// to."
type Endpoint struct {
	Peer domain.PeerID
}

func (e Endpoint) ClearSrc()           {}
func (e Endpoint) SrcToString() string { return "" }
func (e Endpoint) DstToString() string { return string(e.Peer) }
func (e Endpoint) DstToBytes() []byte  { return []byte(e.Peer) }
func (e Endpoint) DstIP() netip.Addr   { return netip.Addr{} }
func (e Endpoint) SrcIP() netip.Addr   { return netip.Addr{} }

type incomingPacket struct {
	data []byte
	ep   Endpoint
}

// Bind implements conn.Bind. Peers are added dynamically via AddPeer as
// their tunnels come up.
//
// wireguard-go calls Close then Open on its Bind every time listen_port is
// set via IpcSet (see device/uapi.go's handling of "listen_port", which
// unconditionally calls BindUpdate) — including during the very first
// configuration, before any peer has even been added: BuildDeviceConfig
// sets listen_port=0 once, and Device.Up() triggers another such cycle.
// Open and Close here are therefore scoped to the *receive interface*
// (ReceiveFuncs) only, gated by a done channel recreated on every Open —
// they must not tear down peer streams. The first version closed the one
// permanent gating channel in Close and never reopened it, so every peer
// silently went unreachable the moment the interface came up — this
// comment is here so nobody "simplifies" that back in.
type Bind struct {
	mu           sync.Mutex
	streams      map[domain.PeerID]port.Stream
	peerChans    map[domain.PeerID]chan incomingPacket
	peersChanged chan struct{}

	openMu sync.Mutex
	done   chan struct{}
}

// peerChanBuffer is each peer's dedicated inbound buffer. Per-peer, not
// shared: a single shared channel used to mean one bursty (or malicious)
// peer filling it would backpressure every other peer's readLoop too,
// since they'd all block trying to write into the same full channel —
// this way, one peer's channel filling up only slows delivery for that
// peer.
const peerChanBuffer = 128

func NewBind() *Bind {
	return &Bind{
		streams:      make(map[domain.PeerID]port.Stream),
		peerChans:    make(map[domain.PeerID]chan incomingPacket),
		peersChanged: make(chan struct{}),
	}
}

// notifyPeersChangedLocked wakes up any receive() call currently blocked in
// reflect.Select on a stale channel snapshot, by closing the current
// peersChanged channel (a case in every such Select) and replacing it with
// a fresh one. Must be called with mu held.
//
// This closes a real race, not just a hypothetical one: Open() is called
// (via wireguard-go's own listen_port-triggered BindUpdate — see the
// struct doc comment) *before* any peer has ever been added — that's the
// normal startup order, Device.Up() happens before
// meshclient.Peers.ConnectToNewMembers's first AddPeer. If receive()'s
// reflect.Select call is already blocked on that empty snapshot (0 peer
// channels, just `done`) by the time AddPeer creates the first peer's
// channel, that specific Select can never learn about it — reflect.Select
// only watches the exact channels it was given, and rebuilding the case
// list only happens on the *next* call to receive(), which never comes
// because this one never returns. Confirmed live: two Android peers
// dialing each other, WireGuard handshake-initiation frames demonstrably
// arriving at the listener's Bind.readLoop (proven via temporary
// instrumentation), yet wireguard-go itself never logged receiving them —
// receive() was still blocked on its original, peer-less snapshot.
func (b *Bind) notifyPeersChangedLocked() {
	close(b.peersChanged)
	b.peersChanged = make(chan struct{})
}

// AddPeer registers stream as peer's transport and starts reading framed
// packets from it in the background, for the life of stream — independent
// of any Bind Open/Close cycle. stream must already be established (see
// EstablishSession) and dedicated solely to WireGuard traffic.
//
// isDialer must match whatever picked the transport role for stream
// (domain.IsDialer). It's not just bookkeeping: QUIC/yamux streams aren't
// visible to the peer until something is actually written to them, but
// WireGuard doesn't write anything on its own until it has a real packet
// to send — neither side proactively initiates just because a peer was
// configured. Left alone, the dialer's stream would sit open-but-silent
// forever and the listener's AcceptStream would never return, each
// waiting on the other. So the dialer primes the stream with one empty
// frame immediately; the listener's read loop below discards empty
// frames as a no-op rather than forwarding them into WireGuard.
func (b *Bind) AddPeer(peer domain.PeerID, stream port.Stream, isDialer bool) error {
	if isDialer {
		if err := writeFrame(stream, nil); err != nil {
			return fmt.Errorf("wgmesh: prime stream to %s: %w", peer, err)
		}
	}

	ch := make(chan incomingPacket, peerChanBuffer)

	b.mu.Lock()
	old, hadOld := b.streams[peer]
	b.streams[peer] = stream
	b.peerChans[peer] = ch
	b.notifyPeersChangedLocked()
	b.mu.Unlock()
	if hadOld {
		// A second AddPeer for a peer ID already registered would
		// otherwise silently overwrite the map entry while the first
		// stream's readLoop kept running underneath (a leaked goroutine
		// stuck reading a stream nothing references anymore). Closing it
		// makes that readLoop's next read fail and return.
		_ = old.Close()
	}

	go b.readLoop(peer, stream, ch)
	return nil
}

// RemovePeer stops reading from and forgets peer's stream, and closes it.
// Used when a peer's tunnel has died so a later reconnect attempt (a fresh
// AddPeer call) starts clean rather than tripping the same-peer guard
// above.
func (b *Bind) RemovePeer(peer domain.PeerID) {
	b.mu.Lock()
	stream, ok := b.streams[peer]
	delete(b.streams, peer)
	b.notifyPeersChangedLocked()
	b.mu.Unlock()
	if ok {
		_ = stream.Close()
	}
}

// removeIfCurrent drops peer's stream and channel registrations, but only
// if they're still the ones passed in — guards against clobbering a newer
// registration a concurrent AddPeer may have already installed for the
// same peer ID by the time this (an old readLoop's deferred cleanup,
// typically) runs.
func (b *Bind) removeIfCurrent(peer domain.PeerID, stream port.Stream, ch chan incomingPacket) {
	b.mu.Lock()
	if b.streams[peer] == stream {
		delete(b.streams, peer)
	}
	if b.peerChans[peer] == ch {
		delete(b.peerChans, peer)
	}
	b.notifyPeersChangedLocked()
	b.mu.Unlock()
}

// HasPeer reports whether peer currently has a live, registered stream.
func (b *Bind) HasPeer(peer domain.PeerID) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.streams[peer]
	return ok
}

func (b *Bind) readLoop(peer domain.PeerID, stream port.Stream, ch chan incomingPacket) {
	// Self-heal on exit: once this stream dies (peer went offline,
	// transient network failure, whatever), forget it so HasPeer reports
	// it as gone and a caller like cmd/spur's meshPeers can retry a fresh
	// AddPeer instead of that peer being wedged as "connected" forever.
	// Guarded by identity, not just presence, so this doesn't clobber a
	// newer stream a concurrent AddPeer may have already installed for
	// the same peer. ch is closed here too (readLoop is its only writer)
	// so Open's receive loop notices this peer is gone instead of
	// selecting on a channel nobody will ever write to again.
	defer func() {
		b.removeIfCurrent(peer, stream, ch)
		close(ch)
	}()

	for {
		size, err := readFrameLength(stream)
		if err != nil {
			return
		}

		if size == 0 {
			continue // priming frame (see AddPeer) — not a real packet
		}

		buf := make([]byte, size)
		if _, err := io.ReadFull(stream, buf); err != nil {
			return
		}

		// Deliberately unconditional: if nothing is currently reading
		// (Bind momentarily between an Open/Close cycle), this just
		// backpressures until the buffer has room or a new receive
		// session starts draining it again. Isolated to this peer's own
		// channel, so it only ever delays this peer's own delivery.
		ch <- incomingPacket{data: buf, ep: Endpoint{Peer: peer}}
	}
}

func readFrameLength(r io.Reader) (uint16, error) {
	var lenBuf [2]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return 0, err
	}
	size := binary.BigEndian.Uint16(lenBuf[:])
	if int(size) > maxPacketSize {
		return 0, fmt.Errorf("wgmesh: frame too large: %d", size)
	}
	return size, nil
}

func writeFrame(w io.Writer, payload []byte) error {
	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(payload)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := w.Write(payload)
	return err
}

func (b *Bind) Open(_ uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.openMu.Lock()
	defer b.openMu.Unlock()

	done := make(chan struct{})
	b.done = done

	// Fans in across every peer's own channel (see peerChanBuffer's doc
	// comment for why each peer gets one instead of sharing a single
	// channel) using reflect.Select, since the set of channels changes at
	// runtime as peers are added/removed and a plain select statement
	// needs a fixed case list at compile time. Rebuilt on every call: the
	// peer set for a mesh network is small (tens of members, not
	// thousands), so the reflection overhead is an acceptable trade for
	// not having to manage a fleet of per-peer forwarder goroutines.
	//
	// peersChanged is its own case, not just a detail of the rebuild
	// loop: without it, a call already blocked in reflect.Select on a
	// case list built before a peer existed would never learn that
	// AddPeer later registered one — reflect.Select only watches the
	// exact channels it was handed. Closing (and replacing)
	// peersChanged whenever the peer set changes forces any in-flight
	// Select to wake up and rebuild against current state. See
	// notifyPeersChangedLocked's doc comment for the real bug this
	// fixes.
	receive := func(packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
		for {
			b.mu.Lock()
			cases := make([]reflect.SelectCase, 0, len(b.peerChans)+2)
			for _, ch := range b.peerChans {
				cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ch)})
			}
			changed := b.peersChanged
			b.mu.Unlock()

			changedIdx := len(cases)
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(changed)})
			doneIdx := len(cases)
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(done)})

			chosen, recv, ok := reflect.Select(cases)
			if chosen == doneIdx {
				return 0, net.ErrClosed
			}
			if chosen == changedIdx {
				continue // peer set changed — rebuild cases against current state
			}
			if !ok {
				// That peer's channel was closed (readLoop exited,
				// possibly superseded by a newer AddPeer) between us
				// building cases and reflect.Select firing on it.
				// Rebuild against current state and try again.
				continue
			}

			p := recv.Interface().(incomingPacket) //nolint:forcetypeassert // only incomingPacket values are ever sent on peerChans
			n := copy(packets[0], p.data)
			sizes[0] = n
			eps[0] = p.ep
			return 1, nil
		}
	}
	return []conn.ReceiveFunc{receive}, 0, nil
}

func (b *Bind) Close() error {
	b.openMu.Lock()
	defer b.openMu.Unlock()

	if b.done == nil {
		return nil
	}
	select {
	case <-b.done:
	default:
		close(b.done)
	}
	return nil
}

func (b *Bind) SetMark(uint32) error { return nil }

func (b *Bind) Send(bufs [][]byte, ep conn.Endpoint) error {
	wgEp, ok := ep.(Endpoint)
	if !ok {
		return fmt.Errorf("wgmesh: unexpected endpoint type %T", ep)
	}

	b.mu.Lock()
	stream, ok := b.streams[wgEp.Peer]
	b.mu.Unlock()
	if !ok {
		return fmt.Errorf("wgmesh: unknown peer %s", wgEp.Peer)
	}

	for _, buf := range bufs {
		if len(buf) > maxPacketSize {
			return fmt.Errorf("wgmesh: packet too large: %d", len(buf))
		}
		if err := writeFrame(stream, buf); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bind) ParseEndpoint(s string) (conn.Endpoint, error) {
	return Endpoint{Peer: domain.PeerID(s)}, nil
}

func (b *Bind) BatchSize() int { return 1 }
