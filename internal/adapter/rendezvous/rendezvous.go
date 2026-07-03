// Package rendezvous holds the client-side orchestration shared by every
// data-plane mode ("spur connect"/"expose"/"send"/"receive", and the
// per-peer tunnels "spur join" opens for mesh): load or create a
// persisted identity, dial and register against a control-plane server,
// resolve who the counterpart is, exchange NAT candidates, establish a
// session (punch or relay fallback), and wrap the result in end-to-end
// encryption.
//
// This used to live in cmd/spur/tunnel.go, in package main — which meant
// only the desktop CLI binary could call it. Anything else that wants the
// exact same flow (in particular, a future gomobile-bind facade for the
// Android client — see CLAUDE.md's Android roadmap) needs it importable,
// hence this package. It intentionally does NOT import
// internal/adapter/cli: OnCodeFunc/VersionMismatchFunc below are this
// package's own callback types, structurally identical to cli's, rather
// than an import of them — the same "each layer defines its own mirror
// type" pattern already used between cli and usecase (see
// cli.ProgressFunc's doc comment), so that a non-CLI caller (like a mobile
// facade) never has to pull in cobra-oriented code to use this package.
package rendezvous

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/adapter/e2e"
	"github.com/fu1se/spur/internal/adapter/nat"
	"github.com/fu1se/spur/internal/adapter/tunnel"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/infra"
	"github.com/fu1se/spur/internal/usecase"
	"github.com/fu1se/spur/internal/usecase/port"
)

// OnCodeFunc is called with a freshly minted pairing code, mirroring
// cli.OnCodeFunc — see this package's doc comment for why it's a separate
// type rather than an import.
type OnCodeFunc func(code string)

// VersionMismatchFunc is called when a client and the server it just
// registered with report different build versions, mirroring
// cli.VersionMismatchFunc.
type VersionMismatchFunc func(clientVersion, serverVersion string)

// Tunnel bundles a ready-to-use TunnelConn with the resources it depends
// on staying open (the control connection and the punched UDP socket) —
// see CLAUDE.md's "Время жизни relay-стрима" for why the control
// connection can't be closed early.
type Tunnel struct {
	Conn          port.TunnelConn
	ControlClient *controlclient.Client
	UDPConn       *net.UDPConn
}

func (t *Tunnel) Close() {
	_ = t.Conn.Close()
	_ = t.ControlClient.Close()
	_ = t.UDPConn.Close()
}

// ResolveIdentityPath falls back to infra.DefaultIdentityPath when the
// caller didn't supply one (e.g. the CLI's --identity flag left empty).
func ResolveIdentityPath(identityPath string) (string, error) {
	if identityPath != "" {
		return identityPath, nil
	}
	return infra.DefaultIdentityPath()
}

// ControlClientTLS builds the TLS config for dialing a control-plane
// server at serverAddr: trust-on-first-use pinning
// (infra.TOFUClientTLSConfig) against the default trust store, replacing
// blind InsecureSkipVerify trust. See infra/tofu.go's doc comment for
// what this does and doesn't protect against.
func ControlClientTLS(serverAddr string) (*tls.Config, error) {
	trustStorePath, err := infra.DefaultTrustStorePath()
	if err != nil {
		return nil, err
	}
	return infra.TOFUClientTLSConfig(trustStorePath, serverAddr, controlproto.ALPN), nil
}

// WarnIfVersionMismatch reports (via onMismatch, nil-safe) when this
// client and the server it just registered with are running different
// build versions — a best-effort compatibility hint, not a hard failure:
// this client has no way to know which specific features differ between
// the two versions, only that they aren't the same. "dev" (an
// unreleased/local build) on either side isn't meaningfully comparable,
// so it's skipped rather than flagged as a mismatch on every single local
// development run.
func WarnIfVersionMismatch(clientVersion, serverVersion string, onMismatch VersionMismatchFunc) {
	if onMismatch == nil || clientVersion == "" || serverVersion == "" {
		return
	}
	if clientVersion == "dev" || serverVersion == "dev" {
		return
	}
	if clientVersion != serverVersion {
		onMismatch(clientVersion, serverVersion)
	}
}

// DialAndRegister loads (or creates) the identity at identityPath, dials
// serverAddr over a TOFU-pinned control-plane TLS connection, and
// registers it — the common prefix shared by Establish below and every
// other control-plane-only operation (join-network, room create/join).
// The caller owns closing the returned client.
func DialAndRegister(ctx context.Context, serverAddr, identityPath, clientVersion string, onVersionMismatch VersionMismatchFunc) (*controlclient.Client, infra.Identity, error) {
	resolvedIdentityPath, err := ResolveIdentityPath(identityPath)
	if err != nil {
		return nil, infra.Identity{}, err
	}
	id, err := infra.LoadOrCreateIdentity(resolvedIdentityPath)
	if err != nil {
		return nil, infra.Identity{}, fmt.Errorf("app: load identity: %w", err)
	}

	tlsConf, err := ControlClientTLS(serverAddr)
	if err != nil {
		return nil, infra.Identity{}, err
	}
	client, err := controlclient.Dial(ctx, serverAddr, tlsConf, infra.DefaultQUICConfig())
	if err != nil {
		return nil, infra.Identity{}, err
	}

	regResult, err := client.Register(ctx, id.PublicKey, clientVersion)
	if err != nil {
		client.Close()
		return nil, infra.Identity{}, err
	}
	WarnIfVersionMismatch(clientVersion, regResult.ServerVersion, onVersionMismatch)

	return client, id, nil
}

// Establish runs the full client-side flow shared by "spur connect",
// "spur expose", "spur send", "spur receive" and each per-peer tunnel
// "spur join" opens: load (or create) a persisted identity, register,
// resolve the counterpart (see CounterpartResolver), gather and exchange
// NAT candidates, establish a session (punch or relay fallback), and
// build the resulting data-plane TunnelConn.
//
// onSelfID fires as soon as self is known — right after registration,
// well before the counterpart-dependent steps (candidate exchange can
// block for up to a minute; see controlserver's awaitCandidatesTimeout).
// That ordering matters for bootstrapping: even with pairing codes,
// there's still a "which peer is this" concept worth surfacing early for
// diagnostics/scripting — the persisted identity (see
// infra.LoadOrCreateIdentity) means it's unchanged run to run.
func Establish(ctx context.Context, serverAddr, stunAddr, identityPath, clientVersion string, resolve CounterpartResolver, onSelfID func(string), onVersionMismatch VersionMismatchFunc) (tun *Tunnel, self domain.PeerID, counterpart domain.PeerID, err error) {
	resolvedIdentityPath, err := ResolveIdentityPath(identityPath)
	if err != nil {
		return nil, "", "", err
	}
	id, err := infra.LoadOrCreateIdentity(resolvedIdentityPath)
	if err != nil {
		return nil, "", "", fmt.Errorf("app: load identity: %w", err)
	}
	self = domain.DerivePeerID(id.PublicKey)

	controlTLSConf, err := ControlClientTLS(serverAddr)
	if err != nil {
		return nil, "", "", err
	}
	client, err := controlclient.Dial(ctx, serverAddr, controlTLSConf, infra.DefaultQUICConfig())
	if err != nil {
		return nil, "", "", fmt.Errorf("app: dial control-plane: %w", err)
	}
	defer func() {
		if err != nil {
			client.Close()
		}
	}()

	regResult, err := client.Register(ctx, id.PublicKey, clientVersion)
	if err != nil {
		return nil, "", "", fmt.Errorf("app: register: %w", err)
	}
	WarnIfVersionMismatch(clientVersion, regResult.ServerVersion, onVersionMismatch)
	onSelfID(string(self))

	counterpart, err = resolve(ctx, client, id)
	if err != nil {
		return nil, "", "", fmt.Errorf("app: resolve counterpart: %w", err)
	}

	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, "", "", fmt.Errorf("app: bind data socket: %w", err)
	}
	defer func() {
		if err != nil {
			udpConn.Close()
		}
	}()

	stunUDPAddr, err := net.ResolveUDPAddr("udp", stunAddr)
	if err != nil {
		return nil, "", "", fmt.Errorf("app: resolve stun addr: %w", err)
	}

	hostCandidates, err := nat.HostCandidates(udpConn)
	if err != nil {
		return nil, "", "", fmt.Errorf("app: gather host candidates: %w", err)
	}
	reflexive, err := nat.DiscoverServerReflexive(ctx, udpConn, stunUDPAddr.AddrPort())
	if err != nil {
		return nil, "", "", fmt.Errorf("app: stun discovery: %w", err)
	}
	ownCandidates := append(hostCandidates, reflexive)

	sessionID := domain.SessionIDFor(self, counterpart)

	var ownSalt [32]byte
	if _, err = rand.Read(ownSalt[:]); err != nil {
		return nil, "", "", fmt.Errorf("app: generate session salt: %w", err)
	}

	exchange := usecase.ExchangeCandidates{Signaler: client}
	peerSet, err := exchange.Execute(ctx, sessionID, self, counterpart, domain.CandidateSet{
		Candidates: ownCandidates,
		PublicKey:  id.PublicKey,
		Salt:       ownSalt,
	})
	if err != nil {
		return nil, "", "", fmt.Errorf("app: exchange candidates: %w", err)
	}

	establish := usecase.EstablishSession{
		Puncher: &nat.UDPPuncher{Conn: udpConn, SessionID: sessionID},
		Relay:   client,
	}
	session, relayStream, err := establish.Execute(ctx, sessionID, peerSet.Candidates)
	if err != nil {
		return nil, "", "", fmt.Errorf("app: establish session: %w", err)
	}

	isDialer := domain.IsDialer(self, counterpart)

	var dataTLSConf *tls.Config
	if !isDialer {
		dataTLSConf, err = infra.SelfSignedServerTLSConfig(tunnel.DataALPN)
		if err != nil {
			return nil, "", "", fmt.Errorf("app: data-plane tls config: %w", err)
		}
	}

	transport := &tunnel.Transport{Conn: udpConn, TLSConf: dataTLSConf, QUICConf: infra.DefaultQUICConfig()}
	tunnelConn, err := transport.EstablishConn(ctx, session, relayStream, isDialer)
	if err != nil {
		return nil, "", "", fmt.Errorf("app: establish data-plane transport: %w", err)
	}

	// Wrap regardless of which path (P2P or relay) was used — see
	// adapter/e2e's package doc for why both need it: relay because the
	// server sees plaintext otherwise, P2P because the QUIC connection
	// isn't authenticated against the peer's real identity yet
	// (InsecureSkipVerify, Phase 7 follow-up).
	sessionSalt := e2e.CombineSalt(ownSalt, peerSet.Salt)
	encryptedConn, err := e2e.WrapConn(tunnelConn, id.PrivateKey, peerSet.PublicKey, sessionSalt, isDialer)
	if err != nil {
		return nil, "", "", fmt.Errorf("app: wrap end-to-end encryption: %w", err)
	}

	return &Tunnel{Conn: encryptedConn, ControlClient: client, UDPConn: udpConn}, self, counterpart, nil
}
