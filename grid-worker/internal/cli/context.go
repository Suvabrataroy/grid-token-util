package cli

import "context"

// cmd_ctx returns a background context for CLI commands.
// This can be extended to support timeouts or cancellation in the future.
func cmd_ctx() context.Context {
	return context.Background()
}
