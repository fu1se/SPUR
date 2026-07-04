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

The client itself comes in two forms sharing the same identity, trust
store and config file: `spur`, the CLI covered below, and `spur-gui`, a
desktop GUI wrapping the same operations (port forwarding, file
transfer, rooms, mesh VPN) in a window instead of subcommands — see
[GUI client](#gui-client).

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

`make build` (without `install`) just builds `./bin/spur`, `./bin/spur-gui`
and `./bin/spur-server`, without copying anything anywhere — if that's
what you need instead. Cross-compiling for every platform: `make release`
(see `make help`) — covers `spur`/`spur-server` only, not `spur-gui` (see
[GUI client](#gui-client) for why).

`spur` (and, optionally, `spur-gui`) is installed on every machine that
will connect to others. `spur-server` — only on one machine with a public
IP (the rendezvous server).

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

### The quick way: pairing codes

Every point-to-point command (`connect`, `expose`, `send`, `receive`)
works without either side needing to know anything about the other
ahead of time. Leave `--to` empty on one side — it registers a short
code and waits:

```sh
spur receive --server SERVER:4443 --stun-server SERVER:4444 --out ./received
# Код для подключения: 5PFGG5
# Сообщите его собеседнику — он должен указать этот код в --to. Ждём подключения (до 10 минут)...
```

Tell the other side that code — they pass it as `--to`, same as a
peer-id:

```sh
spur send --server SERVER:4443 --stun-server SERVER:4444 --to 5PFGG5 ./file-or-directory
```

That's the whole flow: no separate `whoami` step, no exchanging long
hex IDs. The code is a short-lived (10 minute), server-mediated pointer
to whichever side registered it — not a shortened/weakened version of
the real identity, so guessing one doesn't get you anything close to
impersonating a peer (see [Security](#security)).

### The other way: full peer-id

Useful for scripting, or reconnecting to someone you already know the
ID of without a fresh round of code exchange. Every client has a
permanent `peer-id`, which can be passed in `--to` exactly like a
pairing code:

```sh
spur whoami
```

The identity (an X25519 key) is generated on first run and saved to
`~/.config/spur/identity.key` — subsequent runs of `whoami` return the
same ID. Participants share it with each other manually (chat, voice,
whatever) — the server doesn't do this for them. `--to` accepts either
form; `spur` tells them apart automatically by shape (a peer-id is
always 32 lowercase hex characters, a pairing code is shorter and
uppercase).

### The long-term way: rooms

Pairing codes expire in 10 minutes and full peer-ids have to be
exchanged out of band every time. If you regularly connect to the same
specific person, set up a room once instead:

```sh
spur room create --server SERVER:4443 --room friends
```

This prints an invite token. Send it to the other person once — they
join with it:

```sh
spur room join --server SERVER:4443 --room friends --invite <token>
```

From then on, either side can use `--room friends` in place of `--to`
on `connect`/`expose`/`send`/`receive` — no code or peer-id to type or
exchange again, indefinitely (a room, unlike a pairing code, doesn't
expire and survives a server restart):

```sh
spur send --server SERVER:4443 --stun-server SERVER:4444 --room friends ./path/to/file
```

A room is capped at exactly two members and its name is a shared secret
between the two of you in practice (a third person would need both the
exact room name and the invite token to join) — see
[Security](#security).

### Check connectivity to the server

An optional diagnostic step — register with the server and see what
address it observes for you (useful for debugging NAT). It also compares
your client's build version against the server's and warns if they
differ:

```sh
spur register --server SERVER:4443
```

Prints `peer-id` and `observed-address` (your public `ip:port`, as seen
by the server).

### Version mismatch warnings

Every command that talks to the server (`register`, `connect`, `expose`,
`send`, `receive`, `join`, `join-network`, `room create`, `room join`)
compares its own build version against the server's and prints a warning
if they differ — a heads-up that some functionality might not work as
expected, not a hard failure. Keep both sides on the same release when
possible (see [Installation](#installation) for how to update).

## GUI client

`spur-gui` is a desktop GUI covering the same operations as the CLI
(identity, port forwarding, file transfer, rooms, mesh VPN) as a window
with tabs instead of subcommands — for anyone who'd rather click buttons
than remember flags. It's a genuine peer of the CLI, not a replacement:
both binaries read and write the exact same identity file, TOFU trust
store and `config.json` (see [Config file](#config-file)), so switching
between `spur connect ...` on the command line and the GUI's "Port
forward" tab on the same machine just works — no separate setup, same
peer-id either way.

Build and run it like any other binary (see
[Installation](#installation) — `make build`/`make install` produce it
alongside `spur`/`spur-server`):

```sh
spur-gui
```

Not part of the cross-compiled `make release` set, unlike `spur`/
`spur-server`: it's built on Fyne, which needs cgo bindings to the real
target platform's GL/windowing libraries (X11/Wayland, Cocoa, Win32) —
that can't be produced by cross-compiling from one machine the way the
pure-Go CLI/server binaries can. Build it natively on whichever OS you
want to run it on.

On Linux, `make install` also adds spur-gui to your desktop's
applications menu (a `~/.local/share/applications/spur-gui.desktop`
entry with the product's own icon) — look for "spur" alongside your
other installed apps instead of launching it from a terminal every time.

Uses the product's own logo for its window/taskbar icon and a dark
navy/teal color theme matching the Android app — the same visual
identity across every client, not a separately invented desktop look.

The window has five tabs:

- **Identity** — your peer-id (with a copy button), the shared server/
  STUN address settings every other tab uses, and a connectivity test
  (same as `spur register`).
- **Port forward** — `connect`/`expose`, picking a counterpart by peer-id,
  pairing code, or room.
- **Files** — `send`/`receive`, with a file/folder picker, a progress
  bar, and a resume prompt for partially-received transfers.
- **Rooms** — `room create`/`room join`.
- **Mesh VPN** — `join`, same root/`CAP_NET_ADMIN` requirement as the CLI
  (creating a real TUN interface).

## All client actions

### Single port forwarding: `connect` / `expose`

Like `ssh -L`/`ngrok`, but directly P2P (or via relay if that fails).
One side exposes a local service (`expose`), the other connects to it
(`connect`). `--to` on either side accepts a peer-id or a pairing code,
same as everywhere else — omit it to generate a code instead.

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
to begin). Leave `--to` empty to get a pairing code instead of needing
the sender's peer-id up front:

```sh
spur receive --server SERVER:4443 --stun-server SERVER:4444 --out ./received
```

On the sending side — the path can be either a single file or a
directory (sent recursively, preserving the relative structure of nested
folders):

```sh
spur send --server SERVER:4443 --stun-server SERVER:4444 \
  --to <the code printed above, or the recipient's peer-id> ./path/to/file-or-directory
```

Both commands exit with code `0` once everything has been transferred
and acknowledged by the receiving side. Both also print a live-updating
progress line to stderr while transferring — current file, percentage,
transfer speed, overall bytes moved, and an estimated time remaining
(`~2м 05с` — derived from the current transfer speed, so it settles down
after the first second or two rather than being accurate immediately).
The receiving side reads the sender's full file list before any content
arrives, so both sides show a real overall percentage from the start.

**Resuming an interrupted transfer**: if `receive` is interrupted
(crash, `Ctrl+C`, lost connection) partway through, just run the same
`receive`/`send` pair again with the same `--out`. `receive` checks what's
already on disk and, if it finds a partial file from a previous attempt,
asks:

```
Обнаружена незавершённая передача: 1 файл(ов), уже получено 3.0 MiB из 8.0 MiB.
Продолжить с того места, где остановились? [Y/n]
```

Answering yes (or just pressing Enter) tells the sender to skip the bytes
already received and send only the remainder; answering no starts that
file over from scratch. Either way the result is byte-identical to a
fresh transfer — declining doesn't trust the existing partial content,
it's fully overwritten. This is a resume of content, not a resume of the
session: the P2P/relay handshake still runs again from the start.

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

## Language

`spur` speaks Russian and English. By default it picks whichever matches
your system locale (`LANG`/`LC_ALL`/`LC_MESSAGES`) — Russian if it's set
to a Russian locale, English otherwise. To override it explicitly:

```sh
spur lang en    # always use English from now on
spur lang ru    # always use Russian from now on
spur lang auto  # go back to auto-detecting from the system locale
spur lang       # show the current effective language and how it was chosen
```

The choice is saved to `~/.config/spur/config.json` and applies to every
future invocation. `spur-server` doesn't have a persistent config file
(see [Config file](#config-file)), so it always auto-detects from its
own process's locale.

## All commands

| Command | What it does |
|---|---|
| `spur-server` | Rendezvous server (control-plane + STUN + relay fallback); flags live directly on the root command, not a subcommand |
| `spur-gui` | Desktop GUI covering the same client operations as a window with tabs (see [GUI client](#gui-client)) |
| `spur whoami` | Your own peer-id, no network access |
| `spur register` | Diagnostic: register with the server, show the observed address |
| `spur connect` | Forward a local port to a peer's service (`expose`) |
| `spur expose` | Expose a local service to a peer (`connect`) |
| `spur join` | Join a mesh network (real TUN + WireGuard, needs root) |
| `spur join-network` | Diagnostic: the same network coordination, without TUN and without root |
| `spur send` | Send a file/directory to a peer running `spur receive` |
| `spur receive` | Receive a file/directory from a peer running `spur send` |
| `spur room create` | Create a long-term, two-member room and get an invite token |
| `spur room join` | Join a room created by someone else, using its invite token |
| `spur lang` | Show or change the UI language (see [Language](#language)) |
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
- **Pairing codes** are a short-lived (10 minute), server-mediated
  pointer to a peer-id — not a shortened or weakened derivation of the
  real identity. The server hands out and resolves the mapping; nobody
  can compute someone else's code from their peer-id or vice versa, and
  guessing a code (~1 billion possibilities, expires quickly) doesn't
  get an attacker anywhere near impersonating the peer it points to —
  the actual session still goes through the same end-to-end key exchange
  as a connection addressed by full peer-id.
- **Rooms** are capped at exactly two members forever — a third `room
  join` attempt is rejected outright, even with the right invite token.
  Joining an existing room requires the invite token issued at creation
  (compared constant-time server-side, same as mesh network tokens);
  after both members have joined, resolving `--room <name>` to a peer-id
  requires the caller to already be one of the two members — an outsider
  who merely learns the room name gets nothing without the token used to
  actually join it.

Architecture details, known limitations, and the reasoning behind every
development phase's decisions are in `CLAUDE.md`.

## Development

```sh
make test     # go test ./... -race
make vet      # go vet + gofmt -l
make build    # build ./bin/spur, ./bin/spur-gui and ./bin/spur-server for the current platform
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
