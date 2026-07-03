# SPUR

English | [Русский](README.ru.md)

A CLI app and rendezvous server for connecting directly into a local
network across NAT — a P2P tunnel with UDP hole punching and automatic
fallback to relay through the server if punching fails.

Modes:

- **port-forward** (`connect`/`expose`) — forward a single port from one
  machine to another, like `ngrok`/`frp`.
- **mesh VPN** (`join`) — full network connectivity between every member
  of a network over a real TUN interface and WireGuard, like `tailscale`.
- **file transfer** (`send`/`receive`) — send a file or directory
  directly to a peer, with no intermediate service.

All data traffic is end-to-end encrypted with the participants' own keys
regardless of whether the connection went direct (P2P) or through the
relay server — the server is physically unable to read the content.

The client and server are two separate binaries (`spur` and
`spur-server`), not one with subcommands: the client has no reason to
pull in the SQLite driver and control-plane server it never uses.

## Installation

The simplest way (Linux/macOS, no Go required) — downloads a ready-built
binary for your platform from the latest
[release](https://github.com/fu1se/SPUR/releases) and **automatically
adds it to `PATH`**: detects your shell (bash/zsh/fish) and appends the
right line to the corresponding rc file, if the install directory isn't
already there.

```sh
curl -fsSL https://raw.githubusercontent.com/fu1se/SPUR/master/install.sh | sh
```

Installs into `~/.local/bin` by default; a different directory via
`SPUR_INSTALL_DIR=/wherever sh install.sh`. The script is idempotent —
safe to re-run, it won't add duplicate lines to your rc file. If the
directory was just added to `PATH`, the effect shows up after restarting
your shell (or `source ~/.bashrc`/`~/.zshrc`/`~/.config/fish/config.fish`
— the script tells you which) — a running script physically cannot
change the `PATH` of an already-open parent shell; that's a limitation
of shells themselves, not something to work around.

No automation on Windows — download `spur-windows-amd64.exe`/
`spur-server-windows-amd64.exe` from the same releases page and add their
folder to `PATH` manually via "Environment Variables".

Building and installing from source (needs Go 1.26+, see `go.mod`) —
the same install.sh under the hood, so `PATH` gets set up automatically
the same way:

```sh
git clone https://github.com/fu1se/SPUR.git
cd SPUR
make install        # builds and installs + sets up PATH
```

`make build` (without `install`) just builds `./bin/spur` and
`./bin/spur-server`, without copying anything anywhere — if that's what
you need instead. Cross-compiling for every platform: `make release`
(see `make help`).

`spur` is installed on every machine that will connect to others.
`spur-server` — only on one machine with a public IP (the rendezvous
server).

## Running the server

One server is needed for the whole system — it introduces clients to
each other and helps punch through NAT, but by default doesn't see or
proxy user traffic (except for fallback relay, when P2P punching fails,
and even then — only end-to-end encrypted bytes).

Running it on a machine with a public IP:

```sh
spur-server --listen :4443 --stun-listen :4444
```

Flags:

| Flag | Default | What it does |
|---|---|---|
| `--listen` | `:4443` | control-channel address (QUIC) |
| `--stun-listen` | `:4444` | STUN endpoint address (UDP) — clients need it to learn their own public `ip:port` |
| `--db` | `~/.config/spur/state.db` | state file (SQLite) — registered peers, mesh networks and their members |
| `--verbose` | off | verbose (debug-level) logs instead of info |

Both ports (`--listen` and `--stun-listen`) need to be reachable from the
outside — forward them on your router/security group if the server
itself is behind NAT or a firewall.

Server state is persistent: a restart (`systemd`, a crash, a binary
upgrade) doesn't lose registered peers and mesh networks — they live in
the SQLite file from `--db`. Logs go to stderr; for a systemd service
that ends up in `journalctl` automatically.

To check the server is alive and reachable, run `spur register` (see
below) from a client — it needs no prior setup besides the server
address.

## Client quick start

### 1. Find your peer-id

Every client is identified by its `peer-id`, which others pass in
`--to`. The ID is printed locally, with no network access:

```sh
spur whoami
```

The identity (an X25519 key) is generated on first run and saved to
`~/.config/spur/identity.key` — subsequent runs of `whoami` return the
same ID. Without this, the two sides would have no way to learn each
other's ID ahead of the first real connection.

To exchange IDs, participants need to share the output of `spur whoami`
with each other manually (chat, voice, whatever) — the server doesn't do
this for them.

### 2. Check connectivity to the server

An optional diagnostic step — register with the server and see what
address it observes for you (useful for debugging NAT):

```sh
spur register --server SERVER:4443
```

Prints `peer-id` and `observed-address` (your public `ip:port`, as seen
by the server).

## All client actions

### Single port forwarding: `connect` / `expose`

Like `ssh -L`/`ngrok`, but directly P2P (or via relay if that fails).
One side exposes a local service (`expose`), the other connects to it
(`connect`).

On the machine with the service (e.g. SSH on port 22):

```sh
spur expose --server SERVER:4443 --stun-server SERVER:4444 \
  --to <peer-id of whoever will connect> --port 22
```

On the connecting machine:

```sh
spur connect --server SERVER:4443 --stun-server SERVER:4444 \
  --to <peer-id of the machine with the service> --local-port 2222
```

After this, `localhost:2222` on the `connect` side is forwarded to port
`22` on the remote machine. Both commands block the terminal and keep
running until stopped (`Ctrl+C`).

> A single Ctrl+C doesn't stop things immediately — it just warns. Press
> it again within 3 seconds to confirm (protects a long transfer/session
> from an accidental keypress). Applies to every command, including
> `spur-server`.

### Mesh VPN: `join`

A full virtual network between every participant over a real TUN
interface and WireGuard — not just a single forwarded port. Requires
root/`CAP_NET_ADMIN`, since it creates a network interface.

The first participant creates the network and gets an invite token:

```sh
sudo spur join --server SERVER:4443 --stun-server SERVER:4444 \
  --network home
```

The output includes a line like `invite-token: ...` — pass it on to the
rest of the participants (manually, same as the peer-id). Everyone else
joins with that token:

```sh
sudo spur join --server SERVER:4443 --stun-server SERVER:4444 \
  --network home --invite <token>
```

A repeat `join` by an already-joined participant doesn't need the token.
Every participant is assigned an address from `100.64.0.0/10`; once up,
everyone can see each other directly at those addresses, like a regular
VPN — including anyone who joined later than the rest.

Diagnostics without bringing up a TUN (check network coordination
without touching the network/root):

```sh
spur join-network --server SERVER:4443 --network home --invite <token>
```

Prints the network's CIDR and the list of members with their mesh IPs.

### Sending files and directories: `send` / `receive`

Unlike `connect`/`expose`/`join`, this is a one-shot operation: the
command exits on its own once the transfer finishes, no need to keep it
running.

On the receiving side (start `receive` first — it waits for the transfer
to begin):

```sh
spur receive --server SERVER:4443 --stun-server SERVER:4444 \
  --to <sender's peer-id> --out ./received
```

On the sending side — the path can be either a single file or a
directory (sent recursively, preserving the relative structure of nested
folders):

```sh
spur send --server SERVER:4443 --stun-server SERVER:4444 \
  --to <recipient's peer-id> ./path/to/file-or-directory
```

Both commands exit with code `0` once everything has been transferred
and acknowledged by the receiving side. Both also print a live-updating
progress line to stderr while transferring — current file, percentage,
transfer speed, and overall bytes moved (the sender additionally knows
and shows an overall percentage; the receiver doesn't know the total
size of what's coming until it's done, so it only shows a running byte
count).

## Config file

To avoid repeating `--server`/`--stun-server`/`--identity` in every
command, you can set them once in `~/.config/spur/config.json`
(on macOS/Windows — in the corresponding per-user config directory):

```json
{
  "server": "SERVER:4443",
  "stun_server": "SERVER:4444"
}
```

Command-line flags always take priority over the config file; the file
itself is optional — its absence doesn't change any default behavior.

## All commands

| Command | What it does |
|---|---|
| `spur-server` | Rendezvous server (control-plane + STUN + relay fallback); flags live directly on the root command, not a subcommand |
| `spur whoami` | Your own peer-id, no network access |
| `spur register` | Diagnostic: register with the server, show the observed address |
| `spur connect` | Forward a local port to a peer's service (`expose`) |
| `spur expose` | Expose a local service to a peer (`connect`) |
| `spur join` | Join a mesh network (real TUN + WireGuard, needs root) |
| `spur join-network` | Diagnostic: the same network coordination, without TUN and without root |
| `spur send` | Send a file/directory to a peer running `spur receive` |
| `spur receive` | Receive a file/directory from a peer running `spur send` |
| `spur version` / `spur-server version` | Build version |

Every command has `--help` with the full list of flags.

## Security

- **Identity** — an X25519 keypair, persistent (`~/.config/spur/identity.key`).
- **Control channel** — QUIC/TLS with trust-on-first-use pinning of the
  server's certificate (`~/.config/spur/known_servers.json`): the first
  connection to a server is unprotected (like an SSH host key), but the
  server being swapped out *after* it was already trusted is detected.
- **Data plane** — end-to-end encryption (X25519 ECDH → HKDF →
  AES-256-GCM) over whatever transport is in use (P2P or relay),
  independent of WireGuard, which already encrypts on its own in mesh
  mode.
- **Mesh networks** are protected by an invite token: joining an
  already-existing network requires knowing the token issued when it was
  created.

Architecture details, known limitations, and the reasoning behind every
development phase's decisions are in `CLAUDE.md`.

## Development

```sh
make test     # go test ./... -race
make vet      # go vet + gofmt -l
make build    # build ./bin/spur and ./bin/spur-server for the current platform
make install  # build and install into PATH (see install.sh)
```

Changing the control protocol requires regenerating the protobuf code:

```sh
make proto    # needs protoc and protoc-gen-go in PATH
```

## License

[AGPLv3](LICENSE). In particular: if you host a modified `spur-server`
as a public service, you're required to provide that service's users
access to the source code of your modifications (this is what AGPL adds
on top of plain GPL — the requirement triggers on network access, not
just on distributing the binary).
