package desktopshare

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

// viewerCandidates lists known VNC viewers in preference order, each
// with its own way of spelling "connect to host:port" (there is no
// universal syntax: TigerVNC wants host::port — double colon means a raw
// port instead of a display number — gvncviewer wants host:display where
// display = port-5900, and the GUI suites take a vnc:// URI).
var viewerCandidates = []struct {
	bin  string
	args func(host string, port int) []string
}{
	{"vncviewer", func(host string, port int) []string { return []string{fmt.Sprintf("%s::%d", host, port)} }},
	{"gvncviewer", func(host string, port int) []string { return []string{fmt.Sprintf("%s:%d", host, port-5900)} }},
	{"remmina", func(host string, port int) []string { return []string{"-c", fmt.Sprintf("vnc://%s:%d", host, port)} }},
	{"krdc", func(host string, port int) []string { return []string{fmt.Sprintf("vnc://%s:%d", host, port)} }},
}

// ViewerNames returns the viewer binaries LaunchViewer knows about, for
// "none of these were found, connect manually" messaging.
func ViewerNames() string {
	names := make([]string, len(viewerCandidates))
	for i, c := range viewerCandidates {
		names[i] = c.bin
	}
	return strings.Join(names, ", ")
}

// LaunchViewer starts the first installed VNC viewer it knows, pointed
// at host:port, and returns which one it launched — "" (with nil error)
// when none is installed, which the caller should surface as "connect
// your own client to <addr>" rather than a failure: the tunnel is up and
// perfectly usable either way. The viewer runs detached: it's the user's
// window now, and closing it must not tear down the tunnel (they may
// simply launch another client), so nothing waits on it beyond reaping.
//
// gvncviewer's display-number syntax can't express ports below 5900 —
// the caller picks the local port and keeps it in VNC's traditional
// range (see cmd/spur's listenDesktopViewerPort), so in practice the
// port-5900 arithmetic above stays valid.
func LaunchViewer(ctx context.Context, host string, port int) (string, error) {
	for _, candidate := range viewerCandidates {
		if _, err := exec.LookPath(candidate.bin); err != nil {
			continue
		}
		if candidate.bin == "gvncviewer" && port < 5900 {
			continue
		}
		cmd := exec.Command(candidate.bin, candidate.args(host, port)...)
		if err := cmd.Start(); err != nil {
			continue // try the next one — a broken install shouldn't end the search
		}
		go func() { _ = cmd.Wait() }() // reap; lifetime is otherwise the user's business
		return candidate.bin, nil
	}
	return "", nil
}

// ViewerAddr formats the address a manual VNC client should connect to.
func ViewerAddr(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}
