//go:build windows

package control

import (
	"fmt"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// Listen creates a Windows named pipe at the given path.
// Path should be in the form \\.\pipe\<name>.
func Listen(path string) (net.Listener, error) {
	cfg := &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;OW)(A;;GA;;;SY)(A;;GA;;;BA)", // Owner, SYSTEM, Admins
		MessageMode:        false,
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	}

	ln, err := winio.ListenPipe(path, cfg)
	if err != nil {
		return nil, fmt.Errorf("listen named pipe %q: %w", path, err)
	}

	return ln, nil
}

// Connect creates a client connection to a Windows named pipe.
func Connect(path string) (net.Conn, error) {
	const timeout = 5 * time.Second
	conn, err := winio.DialPipe(path, &timeout)
	if err != nil {
		return nil, fmt.Errorf("connect to named pipe %q: %w", path, err)
	}
	return conn, nil
}
