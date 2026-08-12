package modem

import (
	"context"
	"errors"
)

// WWANHardwareKind identifies candidates whose control plane comes from an
// integrated Qualcomm modem exposed through the kernel wwan subsystem (for
// example MSM8916 "410" sticks driven over BAM-DMUX). Such modems have no
// idVendor=2c7c USB node, so USB-based discovery cannot see them; the kernel
// instead exposes named at/qmi ports plus the network interface under
// /sys/class/wwan.
const WWANHardwareKind = "wwan"

// compositeDiscoverer merges the candidates of several discoverers. The wwan
// rpmsg ports of integrated Qualcomm modems are invisible to USB-based
// discovery, so the system discoverer combines both sources. Candidates are
// de-duplicated by ID so a USB Quectel modem never appears twice.
type compositeDiscoverer struct {
	discoverers []Discoverer
}

// NewCompositeDiscoverer returns a Discoverer that merges candidates from each
// supplied source. A source that reports no candidates (for example a platform
// without /sys/class/wwan) does not fail the merge.
func NewCompositeDiscoverer(discoverers ...Discoverer) Discoverer {
	return compositeDiscoverer{discoverers: append([]Discoverer(nil), discoverers...)}
}

func (discoverer compositeDiscoverer) Discover(ctx context.Context) ([]Candidate, error) {
	var result []Candidate
	var failures []error
	seen := make(map[string]struct{})
	for _, source := range discoverer.discoverers {
		candidates, err := source.Discover(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			failures = append(failures, err)
			continue
		}
		for _, candidate := range candidates {
			if _, duplicate := seen[candidate.ID]; duplicate {
				continue
			}
			seen[candidate.ID] = struct{}{}
			result = append(result, candidate)
		}
	}
	if len(result) == 0 && len(failures) > 0 {
		return nil, errors.Join(failures...)
	}
	return result, nil
}
