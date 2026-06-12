//go:build !windows

package control

import (
	"fmt"
	"net"
	"os"
)

// Listen creates a Unix domain socket at the given path.
// Any existing socket file is removed first, and the socket is chmod'd to 0600.
func Listen(path string) (net.Listener, error) {
	// Remove any stale socket file
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale socket %q: %w", path, err)
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen unix %q: %w", path, err)
	}

	// Restrict socket file to owner only
	if err := os.Chmod(path, 0600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("chmod socket %q: %w", path, err)
	}

	return ln, nil
}

// Connect creates a client connection to a Unix domain socket.
func Connect(path string) (net.Conn, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, fmt.Errorf("connect to unix socket %q: %w", path, err)
	}
	return conn, nil
}
