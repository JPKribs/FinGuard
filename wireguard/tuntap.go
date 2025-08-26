package wireguard

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"

	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
)

type TUNDevice struct {
	iface *water.Interface
	name  string
	mtu   int
}

// MARK: Create TUN device
func CreateTUN(name string, mtu int) (*TUNDevice, error) {
	cfg := water.Config{DeviceType: water.TUN}

	iface, err := water.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUN: %w", err)
	}

	return &TUNDevice{
		iface: iface,
		name:  iface.Name(),
		mtu:   mtu,
	}, nil
}

// MARK: Add IP address to interface
func (t *TUNDevice) AddAddress(cidr string) error {
	if runtime.GOOS == "darwin" {
		return t.addAddressDarwin(cidr)
	}
	return t.addAddressLinux(cidr)
}

// MARK: Add address on macOS
func (t *TUNDevice) addAddressDarwin(cidr string) error {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("parsing CIDR %s: %w", cidr, err)
	}

	ones, bits := ipnet.Mask.Size()
	if ones == bits {
		cmd := exec.Command("ifconfig", t.name, "inet", ip.String(), ip.String())
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("adding address %s: %w", cidr, err)
		}
	} else {
		network := ipnet.IP.String()
		cmd := exec.Command("ifconfig", t.name, "inet", ip.String(), network)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("adding address %s: %w", cidr, err)
		}
	}

	return nil
}

// MARK: Add address on Linux
func (t *TUNDevice) addAddressLinux(cidr string) error {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("parsing CIDR %s: %w", cidr, err)
	}

	link, err := netlink.LinkByName(t.name)
	if err != nil {
		return fmt.Errorf("finding interface %s: %w", t.name, err)
	}

	addr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   ip,
			Mask: ipnet.Mask,
		},
	}

	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("adding address %s: %w", cidr, err)
	}

	return nil
}

// MARK: Add route through this interface
func (t *TUNDevice) AddRoute(destination string) error {
	if runtime.GOOS == "darwin" {
		return t.addRouteDarwin(destination)
	}
	return t.addRouteLinux(destination)
}

// MARK: Add route on macOS
func (t *TUNDevice) addRouteDarwin(destination string) error {
	cmd := exec.Command("route", "add", "-net", destination, "-interface", t.name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("adding route %s via %s: %w", destination, t.name, err)
	}
	return nil
}

// MARK: Add route on Linux
func (t *TUNDevice) addRouteLinux(destination string) error {
	_, destNet, err := net.ParseCIDR(destination)
	if err != nil {
		return fmt.Errorf("parsing destination %s: %w", destination, err)
	}

	link, err := netlink.LinkByName(t.name)
	if err != nil {
		return fmt.Errorf("finding interface %s: %w", t.name, err)
	}

	route := &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       destNet,
	}

	if err := netlink.RouteAdd(route); err != nil {
		if !strings.Contains(err.Error(), "file exists") {
			return fmt.Errorf("adding route %s: %w", destination, err)
		}
	}

	return nil
}

// MARK: Get interface name
func (t *TUNDevice) Name() string {
	return t.name
}

// MARK: Get file descriptor
func (t *TUNDevice) File() *water.Interface {
	return t.iface
}

// MARK: Close TUN device
func (t *TUNDevice) Close() error {
	return t.iface.Close()
}
