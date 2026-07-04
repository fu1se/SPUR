package wgmesh

import (
	"fmt"
	"net/netip"
	"os/exec"

	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

// InterfaceName is the TUN device name created for the mesh network.
const InterfaceName = "spur0"

// Device wraps a wireguard-go Device bound to a real TUN interface.
// Creating one requires elevated privileges (root or CAP_NET_ADMIN on
// Linux): it creates a kernel network interface and assigns it an
// address — there is no way to test this without actually doing it, no
// automated test in this repo creates a real Device.
type Device struct {
	wg *device.Device
}

// NewDevice creates the TUN interface, assigns addr/prefixBits to it and
// brings it up, and wraps it in a wireguard-go Device using bind for peer
// transport. Callers still need to call IpcSet to configure keys/peers and
// Up to start the WireGuard device itself (separate from the interface
// being administratively up, which NewDevice already did).
func NewDevice(bind *Bind, addr netip.Addr, prefixBits int, logger *device.Logger) (*Device, error) {
	tunDev, err := tun.CreateTUN(InterfaceName, device.DefaultMTU)
	if err != nil {
		return nil, fmt.Errorf("wgmesh: create tun %s: %w", InterfaceName, err)
	}

	if err := assignAddress(InterfaceName, addr, prefixBits); err != nil {
		_ = tunDev.Close()
		return nil, err
	}

	return &Device{wg: device.NewDevice(tunDev, bind, logger)}, nil
}

func (d *Device) IpcSet(uapiConf string) error { return d.wg.IpcSet(uapiConf) }
func (d *Device) Up() error                    { return d.wg.Up() }
func (d *Device) Close()                       { d.wg.Close() }

// NewDeviceFromFD wraps an already-established TUN file descriptor into
// a wireguard-go Device using bind for peer transport — the Android
// counterpart to NewDevice. Unlike desktop, where NewDevice creates the
// interface itself via tun.CreateTUN (requires CAP_NET_ADMIN a regular
// app process doesn't have), Android's android.net.VpnService.Builder
// creates and configures the interface (address, routes) in the OS on
// the app's behalf and hands back a file descriptor over it via
// establish() — this constructor only wraps that already-configured fd,
// nothing left to assign here.
func NewDeviceFromFD(bind *Bind, fd int, logger *device.Logger) (*Device, error) {
	tunDev, _, err := tun.CreateUnmonitoredTUNFromFD(fd)
	if err != nil {
		return nil, fmt.Errorf("wgmesh: wrap tun fd %d: %w", fd, err)
	}
	return &Device{wg: device.NewDevice(tunDev, bind, logger)}, nil
}

// assignAddress configures the TUN interface's address and brings the
// interface up. Shells out to `ip` (iproute2) rather than pulling in a
// netlink library — CAP_NET_ADMIN is required either way, and `ip` is
// present on essentially every Linux system this runs on. Linux-only, like
// the rest of this package (tun.CreateTUN's behavior is already
// platform-specific).
func assignAddress(ifaceName string, addr netip.Addr, prefixBits int) error {
	prefix := fmt.Sprintf("%s/%d", addr, prefixBits)
	if out, err := exec.Command("ip", "addr", "add", prefix, "dev", ifaceName).CombinedOutput(); err != nil {
		return fmt.Errorf("wgmesh: ip addr add %s dev %s: %w: %s", prefix, ifaceName, err, out)
	}
	if out, err := exec.Command("ip", "link", "set", "up", "dev", ifaceName).CombinedOutput(); err != nil {
		return fmt.Errorf("wgmesh: ip link set up dev %s: %w: %s", ifaceName, err, out)
	}
	return nil
}
