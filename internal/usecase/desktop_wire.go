package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/fu1se/spur/internal/usecase/port"
)

// DesktopOffer is what the sharing side tells the viewing side about the
// desktop server it just started, over a dedicated tunnel stream, before
// any port-forward traffic flows: which protocol the server behind the
// tunnel speaks and the session password guarding it. Sending the
// password in-band is the whole point — the tunnel is already
// end-to-end encrypted and authenticated (adapter/e2e), so the user only
// ever exchanges one secret (the pairing code / room), not two.
type DesktopOffer struct {
	Protocol string `json:"protocol"` // e.g. "vnc"
	Password string `json:"password"` // empty when the server needs none
	ViewOnly bool   `json:"view_only"`
}

// maxDesktopOfferSize bounds how much the viewing side reads from the
// offer stream: the counterpart is authenticated but not trusted with
// unbounded allocations (same reasoning as controlproto's maxFrameSize).
const maxDesktopOfferSize = 4 * 1024

// SendDesktopOffer opens one stream on tun, writes the JSON-encoded
// offer, and closes the stream (the close is the end-of-message marker —
// the offer is tiny and one-shot, so EOF-delimited JSON beats inventing
// another length-prefixed framing here). The sharing side calls this
// once per established tunnel, before serving port-forward streams.
func SendDesktopOffer(ctx context.Context, tun port.TunnelConn, offer DesktopOffer) error {
	stream, err := tun.OpenStream(ctx)
	if err != nil {
		return fmt.Errorf("usecase: open desktop offer stream: %w", err)
	}
	payload, err := json.Marshal(offer)
	if err != nil {
		stream.Close()
		return fmt.Errorf("usecase: encode desktop offer: %w", err)
	}
	if _, err := stream.Write(payload); err != nil {
		stream.Close()
		return fmt.Errorf("usecase: send desktop offer: %w", err)
	}
	return stream.Close()
}

// ReceiveDesktopOffer accepts the single offer stream the sharing side
// opens (see SendDesktopOffer) and decodes it. The viewing side calls
// this once per established tunnel, before it starts forwarding its
// local listener into the tunnel.
func ReceiveDesktopOffer(ctx context.Context, tun port.TunnelConn) (DesktopOffer, error) {
	stream, err := tun.AcceptStream(ctx)
	if err != nil {
		return DesktopOffer{}, fmt.Errorf("usecase: accept desktop offer stream: %w", err)
	}
	defer stream.Close()

	payload, err := io.ReadAll(io.LimitReader(stream, maxDesktopOfferSize))
	if err != nil {
		return DesktopOffer{}, fmt.Errorf("usecase: read desktop offer: %w", err)
	}
	var offer DesktopOffer
	if err := json.Unmarshal(payload, &offer); err != nil {
		return DesktopOffer{}, fmt.Errorf("usecase: decode desktop offer: %w", err)
	}
	return offer, nil
}
