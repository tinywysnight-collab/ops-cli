// Command opsx is the single-binary entrypoint. It wires the Cobra root and
// owns the one and only error-to-exit-code translation: domain code and command
// RunE functions return errors; they never call os.Exit or panic for expected
// conditions. Expiry sentinels are rendered as an actionable re-login hint.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tinywysnight-collab/ops-cli/internal/cli"
	"github.com/tinywysnight-collab/ops-cli/internal/creds"
)

func main() {
	ctx, stop := signalContext(context.Background())
	defer stop()
	if err := cli.NewRootCommand().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "opsx: "+formatError(err))
		os.Exit(1)
	}
}

func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

// formatError renders an error for the user. Expiry sentinels become a clear
// re-login instruction; everything else is shown as-is.
func formatError(err error) string {
	if errors.Is(err, creds.ErrMasterExpired) {
		return "master credentials expired — run: opsx login [--opr]"
	}
	return err.Error()
}
