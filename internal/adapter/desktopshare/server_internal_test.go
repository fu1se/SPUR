package desktopshare

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeEnv builds an environment from a var map and a set of installed
// binaries — detectBackend's whole decision table runs on just these two
// probes, no real desktop session required.
func fakeEnv(vars map[string]string, installed ...string) environment {
	return environment{
		getenv: func(key string) string { return vars[key] },
		lookPath: func(bin string) (string, error) {
			for _, have := range installed {
				if have == bin {
					return "/usr/bin/" + bin, nil
				}
			}
			return "", errors.New("not found")
		},
	}
}

func TestDetectBackend(t *testing.T) {
	x11 := map[string]string{"DISPLAY": ":0"}
	sway := map[string]string{"WAYLAND_DISPLAY": "wayland-1", "DISPLAY": ":0", "XDG_CURRENT_DESKTOP": "sway"}
	gnomeWayland := map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0", "XDG_CURRENT_DESKTOP": "GNOME"}
	gnomeXorg := map[string]string{"DISPLAY": ":0", "XDG_CURRENT_DESKTOP": "GNOME"}

	cases := []struct {
		name      string
		env       environment
		want      string
		wantError bool
	}{
		{"plain X11 with x11vnc", fakeEnv(x11, "x11vnc"), backendX11VNC, false},
		{"wlroots with wayvnc", fakeEnv(sway, "wayvnc"), backendWayVNC, false},
		{"GNOME Wayland with grdctl", fakeEnv(gnomeWayland, "grdctl"), backendGRD, false},
		// wayvnc requires wlr-screencopy, which GNOME's compositor
		// doesn't implement — grd must win even with wayvnc installed.
		{"GNOME Wayland never picks wayvnc", fakeEnv(gnomeWayland, "wayvnc", "grdctl"), backendGRD, false},
		// A self-contained child process beats mutating persistent
		// gnome-remote-desktop config when the session is plain Xorg.
		{"GNOME Xorg prefers x11vnc", fakeEnv(gnomeXorg, "x11vnc", "grdctl"), backendX11VNC, false},
		{"GNOME Xorg falls back to grd", fakeEnv(gnomeXorg, "grdctl"), backendGRD, false},
		{"headless", fakeEnv(map[string]string{}), "", true},
		{"X11 with nothing installed", fakeEnv(x11), "", true},
		{"GNOME Wayland with only wayvnc", fakeEnv(gnomeWayland, "wayvnc"), "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := detectBackend(tc.env)
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
