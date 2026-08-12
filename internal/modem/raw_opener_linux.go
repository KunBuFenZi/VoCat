//go:build linux

package modem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const defaultRawReadTimeout = 100 * time.Millisecond

// RawOpener opens AT control ports that are raw character devices rather than
// RS-232 serial lines. Integrated Qualcomm modems exposed through the kernel
// wwan subsystem (for example the MSM8916 "410" sticks) provide /dev/wwan0at0
// rpmsg endpoints where termios settings are unsupported and go.bug.st/serial
// cannot open the node. Reads poll with a short per-port deadline, mirroring
// the blocking-serial behavior Session already expects.
type RawOpener struct {
	SessionOptions SessionOptions
}

func (opener RawOpener) Open(ctx context.Context, port Port) (Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := port.OpenPath()
	if path == "" {
		return nil, errors.New("modem: candidate has no AT port")
	}
	file, err := os.OpenFile(path, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("open AT port %s: %w", path, err)
	}
	session, err := NewSession(&rawPort{file: file, readTimeout: defaultRawReadTimeout}, opener.SessionOptions)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return session, nil
}

// rawPort adapts a raw character device to the Session Transport contract.
// Termios configuration is deliberately skipped: wwan rpmsg endpoints reject
// ioctls. Reads set a short per-port deadline and treat a deadline expiry as a
// timeout with no data, exactly like a blocking serial port, so Session keeps
// polling until its command deadline instead of poisoning the session. os.File
// deadlines are safe to set on every read because Session holds its own mutex
// around each exchange.
type rawPort struct {
	file        *os.File
	readTimeout time.Duration
}

func (port *rawPort) Read(buffer []byte) (int, error) {
	_ = port.file.SetReadDeadline(time.Now().Add(port.readTimeout))
	count, err := port.file.Read(buffer)
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return 0, nil
	}
	return count, err
}

func (port *rawPort) Write(payload []byte) (int, error) {
	return port.file.Write(payload)
}

func (port *rawPort) Close() error {
	return port.file.Close()
}

func (port *rawPort) Drain() error {
	return nil
}

// ResetInputBuffer flushes pending input using the tcflush ioctl when the
// kernel supports it. Raw wwan endpoints that reject the ioctl are left
// untouched: a non-blocking drain cannot discard already-buffered rpmsg bytes,
// but the session's URC queue still tolerates spurious lines.
func (port *rawPort) ResetInputBuffer() error {
	return resetRawInput(port.file)
}

func (port *rawPort) SetReadTimeout(timeout time.Duration) error {
	if timeout <= 0 {
		return errors.New("modem: read timeout must be positive")
	}
	port.readTimeout = timeout
	return nil
}

// resetRawInput discards unread input on a character device. The tcflush ioctl
// is unsupported by rpmsg endpoints, so those failures are intentionally
// swallowed and reported as success.
func resetRawInput(file *os.File) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, file.Fd(), unix.TCFLSH, unix.TCIOFLUSH)
	if errno != 0 && errno != unix.ENOTTY && errno != unix.EINVAL && errno != unix.ENOSYS && errno != unix.EOPNOTSUPP {
		return errno
	}
	return nil
}
