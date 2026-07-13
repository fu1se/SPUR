package desktopshare

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// grdUnit is gnome-remote-desktop's per-user systemd service — the
// process that actually serves VNC once grdctl has configured it.
// grdctl only writes configuration; without (re)starting the unit,
// nothing listens.
const grdUnit = "gnome-remote-desktop.service"

// startGRDServer drives gnome-remote-desktop via grdctl: configure the
// VNC backend with a random per-session password on our chosen port,
// (re)start the service, and wait for the port. Unlike x11vnc/wayvnc
// this mutates persistent per-user configuration, so Stop undoes it:
// VNC disabled, password cleared, and the unit stopped again unless it
// was already running before we touched it (in which case the user had
// their own remote-desktop setup — restart it and leave it be).
//
// g-r-d has no bind-address knob, so unlike the loopback-only backends
// this port is reachable from the LAN while sharing — that's exactly
// what the mandatory random password (shipped to the viewer in-band
// through the encrypted tunnel, see usecase.DesktopOffer) is for.
func startGRDServer(ctx context.Context, port int, viewOnly bool) (*Server, error) {
	// Refuse to stomp on a VNC configuration the user set up themselves:
	// our Stop would otherwise wipe their password and disable a backend
	// they intentionally enabled.
	if enabled, err := grdVNCEnabled(ctx); err == nil && enabled {
		return nil, fmt.Errorf("desktopshare: gnome-remote-desktop VNC is already enabled — spur won't overwrite that configuration; disable it first (grdctl vnc disable) or share your desktop through it directly")
	}

	password, err := randomVNCPassword()
	if err != nil {
		return nil, fmt.Errorf("desktopshare: generate password: %w", err)
	}

	wasActive := grdUnitActive(ctx)

	viewOnlyCmd := "disable-view-only"
	if viewOnly {
		viewOnlyCmd = "enable-view-only"
	}
	setup := [][]string{
		{"grdctl", "vnc", "set-auth-method", "password"},
		{"grdctl", "vnc", "set-password", password},
		{"grdctl", "vnc", "set-port", strconv.Itoa(port)},
		{"grdctl", "vnc", "disable-port-negotiation"},
		{"grdctl", "vnc", viewOnlyCmd},
		{"grdctl", "vnc", "enable"},
		{"systemctl", "--user", "restart", grdUnit},
	}
	for _, argv := range setup {
		if out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput(); err != nil {
			teardownGRD(wasActive)
			return nil, fmt.Errorf("desktopshare: %s: %w (%s)", strings.Join(argv, " "), err, strings.TrimSpace(string(out)))
		}
	}

	stop := func() { teardownGRD(wasActive) }

	if err := waitPortReady(ctx, port, nil); err != nil {
		stop()
		return nil, fmt.Errorf("desktopshare: gnome-remote-desktop did not start listening: %w", err)
	}

	return &Server{Backend: backendGRD, Port: port, Password: password, stop: stop}, nil
}

// teardownGRD reverses startGRDServer's configuration. Best-effort by
// design (the session is ending either way): each step runs even if a
// previous one failed, so a hiccup in one grdctl call doesn't leave the
// password behind too.
func teardownGRD(wasActive bool) {
	run := func(argv ...string) { _, _ = exec.Command(argv[0], argv[1:]...).CombinedOutput() }
	run("grdctl", "vnc", "disable")
	run("grdctl", "vnc", "clear-password")
	run("grdctl", "vnc", "enable-view-only") // g-r-d's own safe default
	if wasActive {
		run("systemctl", "--user", "restart", grdUnit)
	} else {
		run("systemctl", "--user", "stop", grdUnit)
	}
}

func grdUnitActive(ctx context.Context) bool {
	out, err := exec.CommandContext(ctx, "systemctl", "--user", "is-active", grdUnit).Output()
	return err == nil && strings.TrimSpace(string(out)) == "active"
}

// grdVNCEnabled best-effort-parses `grdctl status` for the VNC backend's
// state. grdctl's output isn't a stable machine interface; if the shape
// ever changes, this returns an error and the caller proceeds as if
// disabled — the failure mode is then a clobbered config for users who
// both enabled g-r-d VNC manually AND run spur desktop share, which the
// happy-path check exists to avoid, not a broken share.
func grdVNCEnabled(ctx context.Context) (bool, error) {
	out, err := exec.CommandContext(ctx, "grdctl", "status").CombinedOutput()
	if err != nil {
		return false, err
	}
	inVNC := false
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(line, "\t") && strings.HasPrefix(trimmed, "VNC:") {
			inVNC = true
			continue
		}
		if inVNC {
			if !strings.HasPrefix(line, "\t") { // next top-level section
				break
			}
			if strings.HasPrefix(trimmed, "Status:") {
				return strings.Contains(trimmed, "enabled"), nil
			}
		}
	}
	return false, fmt.Errorf("desktopshare: no VNC status in grdctl output")
}
