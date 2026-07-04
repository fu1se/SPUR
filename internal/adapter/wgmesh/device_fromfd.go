//go:build linux

package wgmesh

import (
	"fmt"

	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

// NewDeviceFromFD wraps an already-established TUN file descriptor into
// a wireguard-go Device using bind for peer transport — the Android
// counterpart to NewDevice. Unlike desktop, where NewDevice creates the
// interface itself via tun.CreateTUN (requires CAP_NET_ADMIN a regular
// app process doesn't have), Android's android.net.VpnService.Builder
// creates and configures the interface (address, routes) in the OS on
// the app's behalf and hands back a file descriptor over it via
// establish() — this constructor only wraps that already-configured fd,
// nothing left to assign here.
//
// Isolated in its own linux-only file: tun.CreateUnmonitoredTUNFromFD
// only has a linux implementation in wireguard-go's tun package (unlike
// tun.CreateTUN, which every platform defines its own version of) —
// referencing it from the same unconstrained file as NewDevice broke
// cross-compiling the desktop CLI for darwin/windows, since the whole
// file needs every symbol it references to exist for the target
// platform regardless of whether desktop code actually calls this one.
// GOOS=android matches a linux build constraint too (confirmed: `gomobile
// bind -target=android` links this file successfully), so this still
// covers the only real caller, android/spurmobile.
func NewDeviceFromFD(bind *Bind, fd int, logger *device.Logger) (*Device, error) {
	tunDev, _, err := tun.CreateUnmonitoredTUNFromFD(fd)
	if err != nil {
		return nil, fmt.Errorf("wgmesh: wrap tun fd %d: %w", fd, err)
	}
	return &Device{wg: device.NewDevice(tunDev, bind, logger)}, nil
}
