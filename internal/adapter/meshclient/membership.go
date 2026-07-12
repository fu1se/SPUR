package meshclient

import (
	"context"
	"time"

	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/adapter/rendezvous"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/infra"
)

// fetchTimeout bounds one Fetch (dial + register + JoinNetwork round
// trip): the callers poll on a short ticker with a long-lived context,
// and without a per-poll bound a poll against a silently dead network
// path would otherwise hang for the connection's MaxIdleTimeout (5
// minutes — see infra.DefaultQUICConfig) before the next poll could
// even start. Generous next to a healthy round trip (milliseconds), tiny
// next to that. Requires controlclient's RPCs to actually honor ctx —
// see bindStreamToContext there.
const fetchTimeout = 15 * time.Second

// Membership polls a mesh network's member list over a control-plane
// connection it owns — and re-dials that connection when it dies, so a
// network drop degrades to "membership briefly stale" instead of "this
// node never learns about new peers again". Before this existed, both
// mesh loops (desktop's "spur join" and the Android facade) dialed one
// control client up front and kept polling it forever: every JoinNetwork
// call after a drop failed on the same dead QUIC connection, each tick
// said "transient — retry next tick", and the retry was doomed for the
// same reason, silently, until process restart. Per-peer data-plane
// tunnels already self-heal (see Peers.reapDeadConnection) — this closes
// the same gap one layer up, on the control plane.
//
// Not safe for concurrent use; both call sites poll from a single loop.
type Membership struct {
	ServerAddr, IdentityPath, TrustStorePath, ClientVersion string
	NetworkName, InviteToken                                string

	// OnVersionMismatch fires on the first successful dial only —
	// re-warning on every reconnect of a long-lived mesh session would
	// be noise (same policy as rendezvous.RunPersistent).
	OnVersionMismatch rendezvous.VersionMismatchFunc

	client     *controlclient.Client
	id         infra.Identity
	dialedOnce bool
}

// Fetch returns the network's current membership, dialing (or re-dialing)
// the control-plane connection if there isn't a live one. Any JoinNetwork
// failure drops the connection so the next Fetch starts from a fresh
// dial: distinguishing "QUIC connection died" from other failures isn't
// worth the fragility — an unnecessary re-dial costs one round trip on
// the next tick, while a missed one used to cost the whole session.
func (m *Membership) Fetch(ctx context.Context) (domain.Network, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	if m.client == nil {
		onMismatch := m.OnVersionMismatch
		if m.dialedOnce {
			onMismatch = nil
		}
		client, id, err := rendezvous.DialAndRegister(ctx, m.ServerAddr, m.IdentityPath, m.TrustStorePath, m.ClientVersion, onMismatch)
		if err != nil {
			return domain.Network{}, err
		}
		m.client, m.id, m.dialedOnce = client, id, true
	}

	network, err := m.client.JoinNetwork(ctx, m.NetworkName, m.InviteToken, m.id.PublicKey)
	if err != nil {
		m.client.Close()
		m.client = nil
		return domain.Network{}, err
	}
	return network, nil
}

// Close drops the control-plane connection, if any.
func (m *Membership) Close() {
	if m.client != nil {
		m.client.Close()
		m.client = nil
	}
}
