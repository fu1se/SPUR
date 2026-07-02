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

	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/usecase/port"
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
// their tunnels come up; Send/receive to a peer not yet added fails (Send)
// or simply never happens (receive).
type Bind struct {
	mu      sync.Mutex
	streams map[domain.PeerID]port.Stream
	stops   map[domain.PeerID]chan struct{}

	incoming chan incomingPacket
	done     chan struct{}
	closeMu  sync.Mutex
}

func NewBind() *Bind {
	return &Bind{
		streams:  make(map[domain.PeerID]port.Stream),
		stops:    make(map[domain.PeerID]chan struct{}),
		incoming: make(chan incomingPacket, 128),
		done:     make(chan struct{}),
	}
}

// AddPeer registers stream as peer's transport and starts reading framed
// packets from it in the background. stream must already be established
// (see EstablishSession) and dedicated solely to WireGuard traffic.
func (b *Bind) AddPeer(peer domain.PeerID, stream port.Stream) {
	b.mu.Lock()
	b.streams[peer] = stream
	stop := make(chan struct{})
	b.stops[peer] = stop
	b.mu.Unlock()

	go b.readLoop(peer, stream, stop)
}

func (b *Bind) readLoop(peer domain.PeerID, stream port.Stream, stop chan struct{}) {
	for {
		size, err := readFrameLength(stream)
		if err != nil {
			return
		}

		buf := make([]byte, size)
		if _, err := io.ReadFull(stream, buf); err != nil {
			return
		}

		select {
		case b.incoming <- incomingPacket{data: buf, ep: Endpoint{Peer: peer}}:
		case <-stop:
			return
		case <-b.done:
			return
		}
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

func (b *Bind) Open(_ uint16) ([]conn.ReceiveFunc, uint16, error) {
	receive := func(packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
		select {
		case p, ok := <-b.incoming:
			if !ok {
				return 0, net.ErrClosed
			}
			n := copy(packets[0], p.data)
			sizes[0] = n
			eps[0] = p.ep
			return 1, nil
		case <-b.done:
			return 0, net.ErrClosed
		}
	}
	return []conn.ReceiveFunc{receive}, 0, nil
}

func (b *Bind) Close() error {
	b.closeMu.Lock()
	defer b.closeMu.Unlock()

	select {
	case <-b.done:
		return nil // already closed
	default:
	}
	close(b.done)

	b.mu.Lock()
	defer b.mu.Unlock()
	for _, stop := range b.stops {
		close(stop)
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
		var lenBuf [2]byte
		binary.BigEndian.PutUint16(lenBuf[:], uint16(len(buf)))
		if _, err := stream.Write(lenBuf[:]); err != nil {
			return err
		}
		if _, err := stream.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bind) ParseEndpoint(s string) (conn.Endpoint, error) {
	return Endpoint{Peer: domain.PeerID(s)}, nil
}

func (b *Bind) BatchSize() int { return 1 }
