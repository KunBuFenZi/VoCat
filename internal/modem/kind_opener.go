package modem

import "context"

// KindOpener selects an opener based on a candidate's hardware kind. Integrated
// Qualcomm modems discovered through the wwan subsystem expose raw character
// devices (RawOpener) while USB Quectel modems use RS-232 serial ports
// (SerialOpener). The kind field on Candidate carries the discriminator, so the
// device manager can open every candidate with the right transport.
type KindOpener interface {
	Opener
	For(Candidate) Opener
}

// NewKindOpener returns an opener that delegates to serial for USB candidates
// and to raw for wwan candidates. The first entry is the default used for any
// hardware kind without an explicit mapping.
func NewKindOpener(serial, raw Opener) KindOpener {
	return kindOpener{serial: serial, raw: raw}
}

type kindOpener struct {
	serial Opener
	raw    Opener
}

func (opener kindOpener) For(candidate Candidate) Opener {
	if candidate.HardwareKind == WWANHardwareKind && opener.raw != nil {
		return opener.raw
	}
	if opener.serial != nil {
		return opener.serial
	}
	return opener
}

func (opener kindOpener) Open(ctx context.Context, port Port) (Client, error) {
	return opener.serial.Open(ctx, port)
}
