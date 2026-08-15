package auth

import (
	"context"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// EnvPassword, when set, supplies the login password instead of prompting.
// Documented trade-off: env vars are process-visible — recommended for CI only.
const EnvPassword = "OPSX_PASSWORD"

// ReadPassword returns the login password as a []byte the caller MUST zeroize
// after use. Resolution order: OPSX_PASSWORD if set (empty does not count),
// else an interactive no-echo prompt. The prompt and trailing newline go to
// stderr (stdout discipline); the password is never echoed, logged, or
// persisted by opsx. The read observes ctx so SIGINT during the prompt fails
// fast instead of blocking in term.ReadPassword.
func ReadPassword(ctx context.Context, promptW io.Writer, promptText string) ([]byte, error) {
	if v := os.Getenv(EnvPassword); v != "" {
		return []byte(v), nil
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("password prompt cancelled: %w", ctx.Err())
	}
	fmt.Fprint(promptW, promptText)
	type readResult struct {
		password []byte
		err      error
	}
	ch := make(chan readResult, 1)
	go func() {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		ch <- readResult{password: b, err: err}
	}()
	var r readResult
	select {
	case <-ctx.Done():
		fmt.Fprintln(promptW)
		return nil, fmt.Errorf("password prompt cancelled: %w", ctx.Err())
	case r = <-ch:
	}
	fmt.Fprintln(promptW)
	if r.err != nil {
		return nil, fmt.Errorf("read password: %w", r.err)
	}
	return r.password, nil
}

// Zeroize overwrites a secret byte slice in place. Call it via defer right after
// obtaining the secret.
func Zeroize(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
