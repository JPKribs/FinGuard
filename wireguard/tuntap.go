package wireguard

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
)

const (
	configTimeout    = 10 * time.Second
	maxConfigRetries = 3
	tunRetryDelay    = 2 * time.Second
)

// TUN device creation and basic configuration

// MARK: CreateTUN
// Creates and configures a TUN device with the specified name and MTU
func CreateTUN(name string, mtu int) (*TUNDevice, error) {
	if mtu <= 0 || mtu > 65536 {
		mtu = 1420
	}

	config := water.Config{
		DeviceType: water.TUN,
	}

	var iface *water.Interface
	var err error

	for attempt := 1; attempt <= maxConfigRetries; attempt++ {
		iface, err = water.New(config)
		if err != nil {
			if attempt < maxConfigRetries {
				time.Sleep(tunRetryDelay)
				continue
			}
			return nil, fmt.Errorf("creating TUN device after %d attempts: %w", maxConfigRetries, err)
		}
		break
	}

	actualName := iface.Name()
	if actualName == "" {
		iface.Close()
		return nil, fmt.Errorf("failed to get interface name")
	}

	device := &TUNDevice{
		iface: iface,
		name:  actualName,
		mtu:   mtu,
	}

	if err := device.configure(); err != nil {
		iface.Close()
		return nil, fmt.Errorf("configuring TUN device: %w", err)
	}

	return device, nil
}

// Platform-specific configuration functions

// MARK: configure
// Configures the TUN device with platform-specific settings and error recovery
func (t *TUNDevice) configure() error {
	var err error
	for attempt := 1; attempt <= maxConfigRetries; attempt++ {
		if runtime.GOOS == "darwin" {
			err = t.configureDarwin()
		} else {
			err = t.configureLinux()
		}

		if err == nil {
			return nil
		}

		if attempt < maxConfigRetries {
			time.Sleep(tunRetryDelay)
			continue
		}
	}

	return fmt.Errorf("failed to configure TUN device after %d attempts: %w", maxConfigRetries, err)
}

// MARK: configureDarwin
// Configures TUN device on macOS using ifconfig with command validation
func (t *TUNDevice) configureDarwin() error {
	if t.name == "" {
		return fmt.Errorf("interface name is empty")
	}

	commands := [][]string{
		{"ifconfig", t.name, "mtu", strconv.Itoa(t.mtu)},
		{"ifconfig", t.name, "up"},
	}

	for _, cmdArgs := range commands {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("command %v failed: %w, output: %s", cmdArgs, err, string(output))
		}
	}

	return t.verifyConfiguration()
}

// MARK: configureLinux
// Configures TUN device on Linux using netlink with proper error handling
func (t *TUNDevice) configureLinux() error {
	if t.name == "" {
		return fmt.Errorf("interface name is empty")
	}

	link, err := netlink.LinkByName(t.name)
	if err != nil {
		return fmt.Errorf("finding interface %s: %w", t.name, err)
	}

	if err := netlink.LinkSetMTU(link, t.mtu); err != nil {
		return fmt.Errorf("setting MTU to %d: %w", t.mtu, err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("bringing interface %s up: %w", t.name, err)
	}

	return t.verifyConfiguration()
}

// MARK: verifyConfiguration
// Verifies that the TUN device was configured correctly
func (t *TUNDevice) verifyConfiguration() error {
	if runtime.GOOS == "linux" {
		link, err := netlink.LinkByName(t.name)
		if err != nil {
			return fmt.Errorf("verification failed - interface not found: %w", err)
		}

		attrs := link.Attrs()
		if attrs.MTU != t.mtu {
			return fmt.Errorf("verification failed - MTU mismatch: expected %d, got %d", t.mtu, attrs.MTU)
		}

		if attrs.Flags&net.FlagUp == 0 {
			return fmt.Errorf("verification failed - interface is not up")
		}
	}

	return nil
}

// Address management functions

// MARK: AddAddress
// Adds an IP address to the TUN interface with validation and retry logic
func (t *TUNDevice) AddAddress(cidr string) error {
	if cidr == "" {
		return fmt.Errorf("CIDR cannot be empty")
	}

	if _, _, err := net.ParseCIDR(cidr); err != nil {
		return fmt.Errorf("invalid CIDR format %s: %w", cidr, err)
	}

	var err error
	for attempt := 1; attempt <= maxConfigRetries; attempt++ {
		if runtime.GOOS == "darwin" {
			err = t.addAddressDarwin(cidr)
		} else {
			err = t.addAddressLinux(cidr)
		}

		if err == nil {
			return nil
		}

		if attempt < maxConfigRetries {
			time.Sleep(tunRetryDelay)
			continue
		}
	}

	return fmt.Errorf("failed to add address %s after %d attempts: %w", cidr, maxConfigRetries, err)
}

// MARK: addAddressDarwin
// Adds IP address on macOS with proper subnet handling
func (t *TUNDevice) addAddressDarwin(cidr string) error {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("parsing CIDR %s: %w", cidr, err)
	}

	var cmd *exec.Cmd
	ones, bits := ipnet.Mask.Size()

	if ones == bits {
		cmd = exec.Command("ifconfig", t.name, "inet", ip.String(), ip.String())
	} else {
		network := ipnet.IP.String()
		cmd = exec.Command("ifconfig", t.name, "inet", ip.String(), network)
	}

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("adding address %s: %w, output: %s", cidr, err, string(output))
	}

	return nil
}

// MARK: addAddressLinux
// Adds IP address on Linux using netlink with duplicate address checking
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
		if strings.Contains(err.Error(), "file exists") {
			return nil
		}
		return fmt.Errorf("adding address %s to interface %s: %w", cidr, t.name, err)
	}

	return nil
}

// Route management functions

// MARK: AddRoute
// Adds a route through this TUN interface with platform-specific handling
func (t *TUNDevice) AddRoute(destination string) error {
	if destination == "" {
		return fmt.Errorf("destination cannot be empty")
	}

	if _, _, err := net.ParseCIDR(destination); err != nil {
		return fmt.Errorf("invalid destination CIDR %s: %w", destination, err)
	}

	var err error
	for attempt := 1; attempt <= maxConfigRetries; attempt++ {
		if runtime.GOOS == "darwin" {
			err = t.addRouteDarwin(destination)
		} else {
			err = t.addRouteLinux(destination)
		}

		if err == nil {
			return nil
		}

		if strings.Contains(err.Error(), "file exists") || strings.Contains(err.Error(), "exists") {
			return nil
		}

		if attempt < maxConfigRetries {
			time.Sleep(tunRetryDelay)
			continue
		}
	}

	return fmt.Errorf("failed to add route %s after %d attempts: %w", destination, maxConfigRetries, err)
}

// MARK: addRouteDarwin
// Adds route on macOS with proper error handling and validation
func (t *TUNDevice) addRouteDarwin(destination string) error {
	cmd := exec.Command("route", "add", "-net", destination, "-interface", t.name)
	if output, err := cmd.CombinedOutput(); err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "File exists") {
			return nil
		}
		return fmt.Errorf("adding route %s via %s: %w, output: %s", destination, t.name, err, outputStr)
	}
	return nil
}

// MARK: addRouteLinux
// Adds route on Linux using netlink with duplicate route handling
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
		if strings.Contains(err.Error(), "file exists") {
			return nil
		}
		return fmt.Errorf("adding route %s via interface %s: %w", destination, t.name, err)
	}

	return nil
}

// MARK: RemoveRoute
// Removes a route from this TUN interface for cleanup operations
func (t *TUNDevice) RemoveRoute(destination string) error {
	if destination == "" {
		return fmt.Errorf("destination cannot be empty")
	}

	if runtime.GOOS == "darwin" {
		return t.removeRouteDarwin(destination)
	}
	return t.removeRouteLinux(destination)
}

// MARK: removeRouteDarwin
// Removes route on macOS with error suppression for non-existent routes
func (t *TUNDevice) removeRouteDarwin(destination string) error {
	cmd := exec.Command("route", "delete", "-net", destination, "-interface", t.name)
	if output, err := cmd.CombinedOutput(); err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "not in table") {
			return nil
		}
		return fmt.Errorf("removing route %s: %w, output: %s", destination, err, outputStr)
	}
	return nil
}

// MARK: removeRouteLinux
// Removes route on Linux using netlink with graceful error handling
func (t *TUNDevice) removeRouteLinux(destination string) error {
	_, destNet, err := net.ParseCIDR(destination)
	if err != nil {
		return fmt.Errorf("parsing destination %s: %w", destination, err)
	}

	link, err := netlink.LinkByName(t.name)
	if err != nil {
		return nil
	}

	route := &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       destNet,
	}

	if err := netlink.RouteDel(route); err != nil {
		if strings.Contains(err.Error(), "no such process") {
			return nil
		}
		return fmt.Errorf("removing route %s: %w", destination, err)
	}

	return nil
}

// Device property accessor functions

// MARK: Name
// Returns the actual interface name assigned by the system
func (t *TUNDevice) Name() string {
	if t == nil {
		return ""
	}
	return t.name
}

// MARK: MTU
// Returns the configured MTU size for this interface
func (t *TUNDevice) MTU() int {
	if t == nil {
		return 0
	}
	return t.mtu
}

// MARK: File
// Returns the underlying water interface for direct access
func (t *TUNDevice) File() *water.Interface {
	if t == nil {
		return nil
	}
	return t.iface
}

// MARK: Close
// Safely closes the TUN device and cleans up resources
func (t *TUNDevice) Close() error {
	if t == nil || t.iface == nil {
		return nil
	}

	return t.iface.Close()
}
