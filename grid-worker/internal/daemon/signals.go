package daemon

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
)

// HandleSignals sets up OS signal handlers for graceful shutdown and config reload.
//
// SIGTERM / SIGINT → cancel context (graceful shutdown)
// SIGHUP → call onHUP (config reload); Unix only
// Windows: only SIGTERM/SIGINT are handled
func HandleSignals(ctx context.Context, cancel context.CancelFunc, onHUP func(), log zerolog.Logger) {
	sigLog := log.With().Str("component", "signal-handler").Logger()

	sigs := make(chan os.Signal, 1)
	registerSignals(sigs)

	go func() {
		for {
			select {
			case <-ctx.Done():
				signal.Stop(sigs)
				return
			case sig := <-sigs:
				switch sig {
				case syscall.SIGTERM, syscall.SIGINT:
					sigLog.Info().
						Str("signal", sig.String()).
						Msg("received shutdown signal, initiating graceful shutdown")
					cancel()
					return
				default:
					handlePlatformSignal(sig, onHUP, sigLog)
				}
			}
		}
	}()
}
