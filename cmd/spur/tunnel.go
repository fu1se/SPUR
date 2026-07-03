package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/adapter/e2e"
	"github.com/fu1se/spur/internal/adapter/localnet"
	"github.com/fu1se/spur/internal/adapter/nat"
	"github.com/fu1se/spur/internal/adapter/tunnel"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/infra"
	"github.com/fu1se/spur/internal/usecase"
	"github.com/fu1se/spur/internal/usecase/port"
)

// establishedTunnel bundles a ready-to-use TunnelConn with the resources it
// depends on staying open (the control connection and the punched UDP
// socket) — see CLAUDE.md's "Время жизни relay-стрима" for why the control
// connection can't be closed early.
type establishedTunnel struct {
	conn          port.TunnelConn
	controlClient *controlclient.Client
	udpConn       *net.UDPConn
}

func (t *establishedTunnel) Close() {
	_ = t.conn.Close()
	_ = t.controlClient.Close()
	_ = t.udpConn.Close()
}

// resolveIdentityPath falls back to infra.DefaultIdentityPath when the user
// didn't pass --identity.
func resolveIdentityPath(identityPath string) (string, error) {
	if identityPath != "" {
		return identityPath, nil
	}
	return infra.DefaultIdentityPath()
}

// controlClientTLS builds the TLS config for dialing a control-plane
// server at serverAddr: trust-on-first-use pinning (infra.TOFUClientTLSConfig)
// against the default trust store, replacing blind InsecureSkipVerify
// trust. See infra/tofu.go's doc comment for what this does and doesn't
// protect against.
func controlClientTLS(serverAddr string) (*tls.Config, error) {
	trustStorePath, err := infra.DefaultTrustStorePath()
	if err != nil {
		return nil, err
	}
	return infra.TOFUClientTLSConfig(trustStorePath, serverAddr, controlproto.ALPN), nil
}

// counterpartResolver learns the counterpart's peer ID once the
// control-plane connection is registered — either it's already known
// (see resolveCounterpartArg, the "guest" side: a raw peer ID or a short
// pairing code the guest was handed) or it has to be learned by
// registering a fresh pairing code and waiting for someone to use it (see
// hostViaPairingCode, the "host" side — see CLAUDE.md's "Код-based
// pairing" for why both sides funnel through the exact same rendezvous
// logic downstream regardless of which one they are).
type counterpartResolver func(ctx context.Context, client *controlclient.Client, id infra.Identity) (domain.PeerID, error)

// resolveCounterpartArg treats raw as a full peer ID if it's already
// shaped like one (see looksLikePeerID), or resolves it as a short
// pairing code against the server otherwise — the "guest" side of both
// the classic --to <peer-id> flow and the newer --to <code> flow.
func resolveCounterpartArg(raw string) counterpartResolver {
	return func(ctx context.Context, client *controlclient.Client, id infra.Identity) (domain.PeerID, error) {
		if looksLikePeerID(raw) {
			return domain.PeerID(raw), nil
		}
		peer, err := client.ResolvePairingCode(ctx, raw, id.PublicKey)
		if err != nil {
			return "", fmt.Errorf("resolve pairing code %q: %w", raw, err)
		}
		return peer, nil
	}
}

// looksLikePeerID reports whether s has the exact shape
// domain.DerivePeerID produces (32 lowercase hex characters — the first
// 16 bytes of a SHA-256 digest) as opposed to a short pairing code
// (usecase.pairingCodeLength characters drawn from a smaller, uppercase
// alphabet) — the two formats never overlap, so this is enough to tell
// them apart without a round trip to the server.
func looksLikePeerID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// fixedCounterpart is a counterpartResolver for callers that already
// have a real domain.PeerID in hand and need no resolution at all — e.g.
// mesh mode, where the counterpart comes from network membership, not
// user input, so there's nothing to parse or look up.
func fixedCounterpart(peer domain.PeerID) counterpartResolver {
	return func(context.Context, *controlclient.Client, infra.Identity) (domain.PeerID, error) {
		return peer, nil
	}
}

// hostViaPairingCode is the "host" side of the single-command connect
// flow: register a fresh short code, hand it to onCode so the caller can
// print it (e.g. "Код для подключения: ABC123"), then block until some
// guest resolves it — see usecase.PairingCodeTTL for how long that can
// take before giving up.
func hostViaPairingCode(onCode cli.OnCodeFunc) counterpartResolver {
	return func(ctx context.Context, client *controlclient.Client, id infra.Identity) (domain.PeerID, error) {
		code, err := client.RegisterPairingCode(ctx, id.PublicKey)
		if err != nil {
			return "", fmt.Errorf("register pairing code: %w", err)
		}
		if onCode != nil {
			onCode(code)
		}
		guest, err := client.AwaitPairingCodeUse(ctx, code)
		if err != nil {
			return "", fmt.Errorf("await pairing code use: %w", err)
		}
		return guest, nil
	}
}

// roomCounterpart is a counterpartResolver for --room: the caller already
// set up a persistent, two-member room (see roomCreate/roomJoin), so the
// counterpart is whichever of the two members isn't the caller — resolved
// server-side, no code or peer ID to type in each time.
func roomCounterpart(roomName string) counterpartResolver {
	return func(ctx context.Context, client *controlclient.Client, id infra.Identity) (domain.PeerID, error) {
		peer, err := client.ResolveRoom(ctx, roomName, id.PublicKey)
		if err != nil {
			return "", fmt.Errorf("resolve room %q: %w", roomName, err)
		}
		return peer, nil
	}
}

// rendezvous runs the full client-side flow shared by "spur connect",
// "spur expose", "spur send" and "spur receive": load (or create) a
// persisted identity, register, resolve the counterpart (see
// counterpartResolver), gather and exchange NAT candidates, establish a
// session (punch or relay fallback), and build the resulting data-plane
// TunnelConn.
//
// onSelfID fires as soon as self is known — right after registration,
// well before the counterpart-dependent steps (candidate exchange can
// block for up to a minute; see controlserver's awaitCandidatesTimeout).
// That ordering matters for bootstrapping: even with pairing codes,
// there's still a "which peer is this" concept worth surfacing early for
// diagnostics/scripting — the persisted identity (see
// infra.LoadOrCreateIdentity) means it's unchanged run to run.
func rendezvous(ctx context.Context, serverAddr, stunAddr, identityPath string, resolve counterpartResolver, onSelfID func(string), onVersionMismatch cli.VersionMismatchFunc) (tun *establishedTunnel, self domain.PeerID, counterpart domain.PeerID, err error) {
	resolvedIdentityPath, err := resolveIdentityPath(identityPath)
	if err != nil {
		return nil, "", "", err
	}
	id, err := infra.LoadOrCreateIdentity(resolvedIdentityPath)
	if err != nil {
		return nil, "", "", fmt.Errorf("app: load identity: %w", err)
	}
	self = domain.DerivePeerID(id.PublicKey)

	controlTLSConf, err := controlClientTLS(serverAddr)
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

	regResult, err := client.Register(ctx, id.PublicKey, cli.Version())
	if err != nil {
		return nil, "", "", fmt.Errorf("app: register: %w", err)
	}
	warnIfVersionMismatch(cli.Version(), regResult.ServerVersion, onVersionMismatch)
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

	return &establishedTunnel{conn: encryptedConn, controlClient: client, udpConn: udpConn}, self, counterpart, nil
}

// counterpartResolverFor picks between three counterpartResolver flavors:
// a non-empty room takes priority (see roomCounterpart — the CLI layer
// already rejects --to and --room being set together, so this ordering
// is just a tie-breaker for the impossible case); otherwise an empty
// --to means "host" (register a pairing code, wait for it to be used),
// non-empty means "guest" (the value is either a full peer ID or a
// pairing code — resolveCounterpartArg tells them apart).
func counterpartResolverFor(to, room string, onCode cli.OnCodeFunc) counterpartResolver {
	if room != "" {
		return roomCounterpart(room)
	}
	if to == "" {
		return hostViaPairingCode(onCode)
	}
	return resolveCounterpartArg(to)
}

// warnIfVersionMismatch reports (via onMismatch, nil-safe) when this
// client and the server it just registered with are running different
// build versions — a best-effort compatibility hint, not a hard failure:
// this client has no way to know which specific features differ between
// the two versions, only that they aren't the same. "dev" (an
// unreleased/local build, see cli.version's doc comment) on either side
// isn't meaningfully comparable, so it's skipped rather than flagged as
// a mismatch on every single local development run.
func warnIfVersionMismatch(clientVersion, serverVersion string, onMismatch cli.VersionMismatchFunc) {
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

// connect is "spur connect": forward every local connection on localPort
// through a tunnel to counterpart, who must be running "spur expose".
// counterpartID may be empty (host mode: register and print a pairing
// code, wait for "spur expose <code>" to use it), a full peer ID, or a
// pairing code the counterpart printed; roomName, if non-empty, resolves
// the counterpart via a persistent room instead (see roomCounterpart) —
// the CLI layer already ensures the two aren't both set.
func connect(ctx context.Context, serverAddr, stunAddr, counterpartID, roomName, identityPath string, localPort int, onSelfID func(string), onCode cli.OnCodeFunc, onVersionMismatch cli.VersionMismatchFunc) error {
	tun, _, _, err := rendezvous(ctx, serverAddr, stunAddr, identityPath, counterpartResolverFor(counterpartID, roomName, onCode), onSelfID, onVersionMismatch)
	if err != nil {
		return err
	}
	defer tun.Close()

	listener, err := localnet.ListenTCP(fmt.Sprintf(":%d", localPort))
	if err != nil {
		return fmt.Errorf("app: listen locally: %w", err)
	}
	defer listener.Close()

	return usecase.ForwardPort{Listener: listener, Tunnel: tun.conn}.Run(ctx)
}

// expose is "spur expose": accept tunnel streams from counterpart and
// forward each to targetPort on the local machine. counterpartID,
// roomName: see connect.
func expose(ctx context.Context, serverAddr, stunAddr, counterpartID, roomName, identityPath string, targetPort int, onSelfID func(string), onCode cli.OnCodeFunc, onVersionMismatch cli.VersionMismatchFunc) error {
	tun, _, _, err := rendezvous(ctx, serverAddr, stunAddr, identityPath, counterpartResolverFor(counterpartID, roomName, onCode), onSelfID, onVersionMismatch)
	if err != nil {
		return err
	}
	defer tun.Close()

	dialer := localnet.TCPDialer{Addr: fmt.Sprintf("127.0.0.1:%d", targetPort)}

	return usecase.ServeExposedPort{Dialer: dialer, Tunnel: tun.conn}.Run(ctx)
}
