// Package desktopshare starts (and stops) a desktop-sharing server on
// the local machine for "spur desktop share", and launches a viewer for
// "spur desktop view". It deliberately wraps existing, battle-tested VNC
// implementations instead of speaking RFB itself — screen capture and
// input injection are deep platform-specific rabbit holes (three
// different APIs just across X11/wlroots/GNOME-Wayland on Linux alone),
// and every desktop Linux setup already has a native way to serve them:
//
//   - x11vnc for plain X11 sessions,
//   - wayvnc for wlroots compositors (Sway, Hyprland, ...),
//   - gnome-remote-desktop (driven via grdctl) for GNOME, whose Wayland
//     compositor supports neither of the above by design — capture goes
//     through GNOME's own portal machinery, which only g-r-d speaks.
//
// The server always rides an already-authenticated, end-to-end encrypted
// spur tunnel (see cmd/spur/desktop.go); where a backend can bind to
// loopback only (x11vnc, wayvnc) it does, and where it can't
// (gnome-remote-desktop has no bind-address knob) a random per-session
// password is set instead and shipped to the viewer in-band through the
// tunnel (usecase.DesktopOffer), so the user still only exchanges one
// secret — the pairing code.
package desktopshare

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Server is a running desktop-sharing server owned by this process (or,
// for the gnome-remote-desktop backend, configured+started by it).
type Server struct {
	Backend  string // "x11vnc", "wayvnc" or "gnome-remote-desktop"
	Port     int    // TCP port the VNC server listens on (loopback for x11vnc/wayvnc)
	Password string // per-session VNC password; empty for loopback-only backends

	stop func()
}

// Stop tears the server down (kills the child process, or disables the
// gnome-remote-desktop VNC backend and restores its service state).
func (s *Server) Stop() {
	if s.stop != nil {
		s.stop()
	}
}

// environment abstracts the two probes backend detection needs, so its
// decision table is unit-testable without faking a whole desktop session.
type environment struct {
	getenv   func(string) string
	lookPath func(string) (string, error)
}

func systemEnvironment() environment {
	return environment{getenv: os.Getenv, lookPath: exec.LookPath}
}

const (
	backendX11VNC = "x11vnc"
	backendWayVNC = "wayvnc"
	backendGRD    = "gnome-remote-desktop"
)

// detectBackend picks which desktop server fits the current session:
//
//   - a plain X11 session with x11vnc installed uses x11vnc (also
//     preferred over grd on GNOME-on-Xorg: a self-contained child
//     process that needs no persistent configuration changes),
//   - a wlroots Wayland session uses wayvnc (never on GNOME — GNOME's
//     compositor doesn't implement the wlr-screencopy protocol wayvnc
//     requires, it fails at startup there no matter what),
//   - GNOME (Wayland or Xorg without x11vnc) uses gnome-remote-desktop
//     via grdctl.
func detectBackend(env environment) (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("desktopshare: only supported on Linux for now (found %s)", runtime.GOOS)
	}

	wayland := env.getenv("WAYLAND_DISPLAY") != ""
	x11 := env.getenv("DISPLAY") != ""
	gnome := strings.Contains(strings.ToLower(env.getenv("XDG_CURRENT_DESKTOP")), "gnome")
	have := func(bin string) bool {
		_, err := env.lookPath(bin)
		return err == nil
	}

	switch {
	case !wayland && x11 && have("x11vnc"):
		return backendX11VNC, nil
	case wayland && !gnome && have("wayvnc"):
		return backendWayVNC, nil
	case gnome && have("grdctl"):
		return backendGRD, nil
	case !wayland && !x11:
		return "", fmt.Errorf("desktopshare: no graphical session detected (neither DISPLAY nor WAYLAND_DISPLAY is set)")
	default:
		return "", fmt.Errorf("desktopshare: no usable desktop server found — install x11vnc (X11 session), wayvnc (Sway/Hyprland/other wlroots) or gnome-remote-desktop with grdctl (GNOME)")
	}
}

// StartServer detects the right backend for the current session, starts
// it on a free TCP port, and waits until that port actually accepts
// connections before returning.
func StartServer(ctx context.Context, viewOnly bool) (*Server, error) {
	backend, err := detectBackend(systemEnvironment())
	if err != nil {
		return nil, err
	}

	port, err := freeTCPPort()
	if err != nil {
		return nil, fmt.Errorf("desktopshare: pick free port: %w", err)
	}

	switch backend {
	case backendX11VNC:
		args := []string{"-display", os.Getenv("DISPLAY"), "-localhost", "-rfbport", strconv.Itoa(port), "-forever", "-shared", "-nopw", "-quiet"}
		if viewOnly {
			args = append(args, "-viewonly")
		}
		return startChildServer(ctx, backendX11VNC, port, "x11vnc", args)
	case backendWayVNC:
		args := []string{}
		if viewOnly {
			args = append(args, "--disable-input")
		}
		args = append(args, "127.0.0.1", strconv.Itoa(port))
		return startChildServer(ctx, backendWayVNC, port, "wayvnc", args)
	case backendGRD:
		return startGRDServer(ctx, port, viewOnly)
	default:
		return nil, fmt.Errorf("desktopshare: unknown backend %q", backend)
	}
}

// startChildServer runs a self-contained VNC server (x11vnc/wayvnc) as a
// child process bound to loopback. No password: nothing off-machine can
// reach the port, and the spur tunnel carrying it to the viewer is
// already end-to-end encrypted and peer-authenticated — a VNC password
// on top would only guard against other local users of the same machine,
// at the cost of shipping it through argv (visible in /proc/*/cmdline to
// exactly those users).
func startChildServer(ctx context.Context, backend string, port int, bin string, args []string) (*Server, error) {
	// Deliberately not CommandContext: the server's lifetime is managed
	// by Server.Stop, not by the (long-lived) command context, and a
	// double-kill is harmless anyway — but tying it to ctx would also be
	// fine; the explicit stop func just keeps both backends symmetrical.
	cmd := exec.Command(bin, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("desktopshare: start %s: %w", bin, err)
	}
	// Reap the child whenever it exits so it never lingers as a zombie.
	waitDone := make(chan struct{})
	go func() { _ = cmd.Wait(); close(waitDone) }()

	stop := func() {
		_ = cmd.Process.Kill()
		<-waitDone
	}

	if err := waitPortReady(ctx, port, waitDone); err != nil {
		stop()
		return nil, fmt.Errorf("desktopshare: %s did not start listening: %w", bin, err)
	}

	return &Server{Backend: backend, Port: port, stop: stop}, nil
}

// freeTCPPort asks the kernel for an ephemeral port and releases it —
// the classic bind(0)/close dance. Racy in principle (someone else could
// grab it between close and the server's own bind), harmless in
// practice for a port handed to a child process milliseconds later.
func freeTCPPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil //nolint:forcetypeassert // net.Listen("tcp") always returns *net.TCPAddr
}

// waitPortReady polls until the server's TCP port accepts a connection,
// the server process dies (died != nil), or ctx/timeout give up — a VNC
// server that never opens its port within 20s isn't coming up at all.
func waitPortReady(ctx context.Context, port int, died <-chan struct{}) error {
	deadline := time.Now().Add(20 * time.Second)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-died:
			return fmt.Errorf("server process exited before listening")
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", addr)
}

// randomVNCPassword generates an 8-character password — 8 characters
// because classic RFB authentication truncates passwords to 8 bytes, so
// anything longer would silently not be what the user thinks it is. The
// alphabet has exactly 64 characters so byte%64 is uniform without
// rejection sampling — same reasoning as usecase's pairing-code alphabet
// (32 there), just with a bigger set since this is pasted, not read
// aloud.
func randomVNCPassword() (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-."
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i, b := range buf {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf), nil
}
