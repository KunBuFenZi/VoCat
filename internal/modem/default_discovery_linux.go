//go:build linux

package modem

// NewSystemDiscoverer merges USB Quectel modem discovery with the kernel wwan
// subsystem so integrated Qualcomm modems (for example MSM8916 "410" sticks
// that expose /dev/wwan0at0 rather than a USB serial port) are discovered too.
func NewSystemDiscoverer() Discoverer {
	return NewCompositeDiscoverer(
		NewSysFSDiscoverer("/sys", "/dev"),
		NewWWANDiscoverer("/sys", "/dev"),
	)
}
