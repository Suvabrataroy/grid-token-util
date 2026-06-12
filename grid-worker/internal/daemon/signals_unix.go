//go:build !windows

package daemon

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
)

// registerSignals registers SIGTERM, SIGINT, and SIGHUP on Unix systems.
func registerSignals(sigs chan<- os.Signal) {
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
}

// handlePlatformSignal handles platform-specific signals (SIGHUP on Unix).
func handlePlatformSignal(sig os.Signal, onHUP func(), log zerolog.Logger) {
	if sig == syscall.SIGHUP {
		log.Info().Msg("received SIGHUP, reloading configuration")
		if onHUP != nil {
			onHUP()
		}
	}
}
