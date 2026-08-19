package firecracker

import (
	"fmt"
	"net"
	"os/exec"
	"sync"
)

// NetworkConfig describes the host-side network cage sets up for
// Firecracker sandboxes: one shared linux bridge, NATed to the internet
// with each vm getting its own tap device attached to it
type NetworkConfig struct {
	Enabled    bool
	BridgeName string
	BridgeCIDR string
	SubnetCIDR string
	DNSServer  string
}

// NetworkManager owns the shared bridge and hands out per-vm tap devices
// and IP addresses. one instance is shared across every sandbox
type NetworkManager struct {
	cfg NetworkConfig

	mu       sync.Mutex
	nextIP   uint32
	baseIP   net.IP
	released []uint32
}

// NewNetworkManager sets up the shared bridge and NAT rules once
// Safe to call on every server startup - every step is written to be idempotent,
// so re-running against an already-configured bridge is a no-op rather
// than an error
func NewNetworkManager(cfg NetworkConfig) (*NetworkManager, error) {
	if !cfg.Enabled {
		return &NetworkManager{cfg: cfg}, nil
	}

	ip, _, err := net.ParseCIDR(cfg.BridgeCIDR)
	if err != nil {
		return nil, fmt.Errorf("invalid bridge CIDR: %w", err)
	}

	nm := &NetworkManager{cfg: cfg, baseIP: ip.Mask(ip.DefaultMask()), nextIP: 2}

	if err := nm.setupBridge(); err != nil {
		return nil, fmt.Errorf("failed to set up bridge: %w", err)
	}
	if err := nm.setupNAT(); err != nil {
		return nil, fmt.Errorf("failed to set up NAT: %w", err)
	}

	return nm, nil
}

// func (nm *NetworkManager) setupBridge() error {
// 	// "ip link add" errors if the bridge already exists -- check first so
// 	// restarting the server doesn't fail on an already-configured bridge
// 	if err := runQuiet("ip", "link", "show", nm.cfg.BridgeName); err == nil {
// 		return nil
// 	}

// 	if err := run("ip", "link", "add", "name", nm.cfg.BridgeName, "type", "bridge"); err != nil {
// 		return err
// 	}
// 	if err := run("ip", "addr", "add", nm.cfg.BridgeCIDR, "dev", nm.cfg.BridgeName); err == nil {
// 		return err
// 	}
// 	if err := run("ip", "link", "set", nm.cfg.BridgeName, "up"); err != nil {
// 		return err
// 	}
// 	return nil
// }

func (nm *NetworkManager) setupBridge() error {
	if err := runQuiet("ip", "link", "show", nm.cfg.BridgeName); err != nil {
		// Bridge doesn't exist yet — create and address it.
		if err := run("ip", "link", "add", "name", nm.cfg.BridgeName, "type", "bridge"); err != nil {
			return err
		}
		if err := run("ip", "addr", "add", nm.cfg.BridgeCIDR, "dev", nm.cfg.BridgeName); err != nil {
			return err
		}
	}

	// Always ensure it's up, whether newly created or pre-existing —
	// a bridge left DOWN from a previous run must be corrected here,
	// not silently skipped by the idempotency check above.
	return run("ip", "link", "set", nm.cfg.BridgeName, "up")
}

func (nm *NetworkManager) setupNAT() error {
	if err := run("sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward"); err != nil {
		return fmt.Errorf("failed to enable ip_forward: %w", err)
	}

	// -C checks whether the rule already exists; only add it if -C fails
	// so re-running this doesn't stack duplicate NAT rule on every restart
	checkArgs := []string{"-t", "nat", "-C", "POSTROUTING", "-s", nm.cfg.SubnetCIDR, "-j", "MASQUERADE"}
	if err := runQuiet("iptables", checkArgs...); err == nil {
		return nil
	}

	addArgs := []string{"-t", "nat", "-A", "POSTROUTING", "-s", nm.cfg.SubnetCIDR, "-j", "MASQUERADE"}
	return run("iptables", addArgs...)
}

// AllocateTap creates a new tap device for one sandbox, attaches it to the
// shared bridge, and assigns it a guest IP from the pool. Returns the tap
// device name and the guest's assigned IP
func (nm *NetworkManager) AllocateTap(sandboxID string) (tapName string, guestIP net.IP, err error) {
	if !nm.cfg.Enabled {
		return "", nil, fmt.Errorf("networking is not enabled")
	}

	tapName = tapDeviceName(sandboxID)

	if err := run("ip", "tuntap", "add", "dev", tapName, "mode", "tap"); err != nil {
		return "", nil, fmt.Errorf("failed to create tap device: %w", err)
	}
	if err := run("ip", "link", "set", tapName, "master", nm.cfg.BridgeName); err != nil {
		nm.releaseTapDevice(tapName)
		return "", nil, fmt.Errorf("failed to attach tap to bridge: %w", err)
	}
	if err := run("ip", "link", "set", tapName, "up"); err != nil {
		nm.releaseTapDevice(tapName)
		return "", nil, fmt.Errorf("failed to bring tap device up: %w", err)
	}

	guestIP = nm.allocateIP()
	return tapName, guestIP, nil
}

// ReleaseTap tears down a sandbox's tap device and returns its ip to the pool
func (nm *NetworkManager) ReleaseTap(sandboxID string, guestIP net.IP) {
	if !nm.cfg.Enabled {
		return
	}
	nm.releaseTapDevice(tapDeviceName(sandboxID))
	nm.releaseIP(guestIP)
}

func (nm *NetworkManager) releaseTapDevice(tapName string) {
	// Best-effort - a failed tap deletion leaves an orphaned host device,
	// which is a minor leak worth monitoring but shouldn't block sandbox
	// cleanup from proceeding
	_ = run("ip", "link", "delete", tapName)
}

func (nm *NetworkManager) allocateIP() net.IP {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	var offset uint32
	if len(nm.released) > 0 {
		offset, nm.released = nm.released[len(nm.released)-1], nm.released[:len(nm.released)-1]
	} else {
		offset = nm.nextIP
		nm.nextIP++
	}
	return offsetIP(nm.baseIP, offset)
}

func (nm *NetworkManager) releaseIP(ip net.IP) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	offset := ipOffset(nm.baseIP, ip)
	nm.released = append(nm.released, offset)
}

func tapDeviceName(sandboxID string) string {
	// linux interface names are capped at 15 chars - use a short perfix
	// plus the first 10 chars of the sandbox ID, which is unique enough
	// in practice for concurrently-running sandboxes
	if len(sandboxID) > 10 {
		sandboxID = sandboxID[:10]
	}
	return "cg-" + sandboxID
}

func offsetIP(base net.IP, offset uint32) net.IP {
	ip := make(net.IP, len(base.To4()))
	copy(ip, base.To4())
	ip[3] += byte(offset)
	return ip
}

func ipOffset(base net.IP, ip net.IP) uint32 {
	b := base.To4()
	i := ip.To4()
	return uint32(i[3] - b[3])
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v failed: %w (output: %s)", name, args, err, out)
	}
	return nil
}

func runQuiet(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}
