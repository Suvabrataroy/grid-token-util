//go:build windows

package daemon

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
)

// registerSignals registers only SIGTERM and SIGINT on Windows (no SIGHUP).
func registerSignals(sigs chan<- os.Signal) {
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
}

// handlePlatformSignal is a no-op on Windows (no platform-specific signals).
func handlePlatformSignal(_ os.Signal, _ func(), _ zerolog.Logger) {
	// Windows does not support SIGHUP
}
