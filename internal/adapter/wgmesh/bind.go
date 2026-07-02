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
	"sync"

	"golang.zx2c4.com/wireguard/conn"

	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/usecase/port"
)

// maxPacketSize bounds a single framed WireGuard packet. WireGuard doesn't
// fragment; real packets are bounded by the tunnel MTU (~1420 bytes), this
// just guards against a corrupt length prefix reading garbage.
const maxPacketSize = 65535

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
	mu      sync.Mutex
	streams map[domain.PeerID]port.Stream

	incoming chan incomingPacket

	openMu sync.Mutex
	done   chan struct{}
}

func NewBind() *Bind {
	return &Bind{
		streams:  make(map[domain.PeerID]port.Stream),
		incoming: make(chan incomingPacket, 128),
	}
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

	b.mu.Lock()
	b.streams[peer] = stream
	b.mu.Unlock()

	go b.readLoop(peer, stream)
	return nil
}

func (b *Bind) readLoop(peer domain.PeerID, stream port.Stream) {
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
		// session starts draining it again.
		b.incoming <- incomingPacket{data: buf, ep: Endpoint{Peer: peer}}
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

	receive := func(packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
		select {
		case p := <-b.incoming:
			n := copy(packets[0], p.data)
			sizes[0] = n
			eps[0] = p.ep
			return 1, nil
		case <-done:
			return 0, net.ErrClosed
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
