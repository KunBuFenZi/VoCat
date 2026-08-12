//go:build linux

package modem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NewWWANDiscoverer returns a discoverer for /sys/class/wwan rpmsg ports.
func NewWWANDiscoverer(sysRoot, devRoot string) *WWANDiscoverer {
	return &WWANDiscoverer{
		SysRoot: filepath.Clean(sysRoot),
		DevRoot: filepath.Clean(devRoot),
	}
}

// WWANDiscoverer scans the kernel wwan class for integrated modem ports.
type WWANDiscoverer struct {
	SysRoot string
	DevRoot string
}

type wwanPortKind int

const (
	wwanPortNet wwanPortKind = iota
	wwanPortAT
	wwanPortQMI
)

type wwanNamedPort struct {
	name string
	path string
}

type wwanState struct {
	name             string
	atPorts          []wwanNamedPort
	qmiPorts         []wwanNamedPort
	networkInterface string
}

func (state *wwanState) qmiDevicePath() string {
	if len(state.qmiPorts) > 0 {
		return state.qmiPorts[0].path
	}
	return ""
}

// Discover lists one candidate per wwan device. The first AT port is selected
// as the control plane; QMI control and the network interface are attached so
// the QMI registration and data backends keep working without any USB node.
func (d *WWANDiscoverer) Discover(ctx context.Context) ([]Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root := filepath.Join(d.SysRoot, "class", "wwan")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("discover wwan devices: %w", err)
	}

	devices := make(map[string]*wwanState)
	for _, entry := range entries {
		name := entry.Name()
		deviceName, kind, ok := parseWWANPortName(name)
		if !ok {
			continue
		}
		state := devices[deviceName]
		if state == nil {
			state = &wwanState{name: deviceName}
			devices[deviceName] = state
		}
		switch kind {
		case wwanPortAT:
			if path := d.portDevicePath(name); path != "" {
				state.atPorts = append(state.atPorts, wwanNamedPort{name: name, path: path})
			}
		case wwanPortQMI:
			if path := d.portDevicePath(name); path != "" {
				state.qmiPorts = append(state.qmiPorts, wwanNamedPort{name: name, path: path})
			}
		case wwanPortNet:
			state.networkInterface = name
		}
	}

	result := make([]Candidate, 0, len(devices))
	for _, state := range devices {
		sort.Slice(state.atPorts, func(i, j int) bool { return state.atPorts[i].name < state.atPorts[j].name })
		sort.Slice(state.qmiPorts, func(i, j int) bool { return state.qmiPorts[i].name < state.qmiPorts[j].name })
		ports := make([]Port, 0, len(state.atPorts)+len(state.qmiPorts))
		for _, atPort := range state.atPorts {
			ports = append(ports, Port{Path: atPort.path, Name: atPort.name, Role: PortRoleAT})
		}
		for _, qmiPort := range state.qmiPorts {
			ports = append(ports, Port{Path: qmiPort.path, Name: qmiPort.name})
		}
		var atPort Port
		if len(state.atPorts) > 0 {
			atPort = ports[0]
		}
		result = append(result, Candidate{
			HardwareKind:     WWANHardwareKind,
			ID:               "wwan-" + state.name,
			USBPath:          d.physicalPath(state.name),
			ATPort:           atPort,
			Ports:            ports,
			QMIControl:       state.qmiDevicePath(),
			NetworkInterface: state.networkInterface,
			Product:          "Qualcomm WWAN",
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// parseWWANPortName splits a /sys/class/wwan entry into its owning wwan device
// name and port kind. "wwan0" is the network interface itself, "wwan0at0" an
// AT port, and "wwan0qmi0" a QMI control port. Unrecognized ports (for example
// "wwan0mbim0") are ignored by the caller.
func parseWWANPortName(name string) (deviceName string, kind wwanPortKind, ok bool) {
	if !strings.HasPrefix(name, "wwan") {
		return "", 0, false
	}
	rest := name[len("wwan"):]
	index := 0
	for index < len(rest) && rest[index] >= '0' && rest[index] <= '9' {
		index++
	}
	if index == 0 {
		return "", 0, false
	}
	deviceName = "wwan" + rest[:index]
	suffix := rest[index:]
	switch {
	case suffix == "":
		return deviceName, wwanPortNet, true
	case strings.HasPrefix(suffix, "at") && allDigits(suffix[len("at"):]):
		return deviceName, wwanPortAT, true
	case strings.HasPrefix(suffix, "qmi") && allDigits(suffix[len("qmi"):]):
		return deviceName, wwanPortQMI, true
	default:
		return "", 0, false
	}
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

// portDevicePath returns the /dev node backing a wwan port when it exists.
func (d *WWANDiscoverer) portDevicePath(name string) string {
	path := filepath.Join(d.DevRoot, name)
	if info, err := os.Stat(path); err == nil && info.Mode()&os.ModeDevice != 0 {
		return path
	}
	return ""
}

// physicalPath resolves the stable device node behind /sys/class/wwan, which
// ATMapper uses to rebind a configured device across node re-enumeration.
func (d *WWANDiscoverer) physicalPath(deviceName string) string {
	link := filepath.Join(d.SysRoot, "class", "wwan", deviceName)
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		return link
	}
	return resolved
}
