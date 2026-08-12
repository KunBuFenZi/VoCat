//go:build !linux

package modem

import (
	"context"
	"errors"
)

// RawOpener keeps the device manager cross-platform. The wwan subsystem only
// exists on Linux, so opening a raw AT port anywhere else is unsupported.
type RawOpener struct {
	SessionOptions SessionOptions
}

func (opener RawOpener) Open(ctx context.Context, port Port) (Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("modem: raw AT ports are only supported on Linux")
}
