package auth

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// EnvPassword, when set, supplies the login password instead of prompting.
// Documented trade-off: env vars are process-visible — recommended for CI only.
const EnvPassword = "OPSX_PASSWORD"

// ReadPassword returns the login password as a []byte the caller MUST zeroize
// after use. Resolution order: OPSX_PASSWORD if set, else an interactive no-echo
// prompt. The prompt and trailing newline go to stderr (stdout discipline); the
// password is never echoed, logged, or persisted by opsx.
func ReadPassword(promptW io.Writer, promptText string) ([]byte, error) {
	if v, ok := os.LookupEnv(EnvPassword); ok {
		return []byte(v), nil
	}
	fmt.Fprint(promptW, promptText)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(promptW)
	if err != nil {
		return nil, fmt.Errorf("read password: %w", err)
	}
	return b, nil
}

// Zeroize overwrites a secret byte slice in place. Call it via defer right after
// obtaining the secret.
func Zeroize(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
