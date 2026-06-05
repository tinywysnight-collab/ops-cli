// Package shell produces the only output that affects the parent terminal.
//
// shell-switch consumers apply this output to the user's live parent shell.
// Therefore emitted output MUST be nothing but environment-assignment lines for
// the selected shell dialect, and values MUST be validated shell-safe (not
// assumed) so a hostile/mistyped alias can never inject commands.
package shell

import (
	"fmt"
	"regexp"
	"strings"
)

// Dialect names the shell assignment syntax used for shell-switch stdout.
type Dialect string

const (
	DialectPOSIX      Dialect = "posix"
	DialectPowerShell Dialect = "powershell"
)

// Assignment is a shell-neutral environment update. Business logic should
// produce these key/value pairs; this package is the boundary that turns them
// into shell syntax.
type Assignment struct {
	Key   string
	Value string
}

var safeKey = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// posixSafeValue is the strict charset opsx ever emits as an export value: profile
// names (alias.mode), filesystem paths, and the mode token. It deliberately
// excludes every shell metacharacter (whitespace, ; | & $ ` ( ) < > quotes …).
//
// "%" is permitted because the per-(cluster,mode) kubeconfig file name
// percent-encodes the cluster alias (e.g. a dotted alias "dev.syd" becomes
// "dev%2Esyd.yaml"), and that path is emitted as `export KUBECONFIG=<path>`. In
// a single `export KEY=value` assignment "%" is not a shell metacharacter, so
// allowing it keeps a config-valid alias from producing a path the emitter would
// otherwise reject. The alias encoding and this charset must agree (see the
// cluster-switching "Unambiguous kubeconfig file naming" requirement).
var posixSafeValue = regexp.MustCompile(`^[A-Za-z0-9%._/@:=+~-]+$`)

// powerShellSafeValue allows the same opsx-managed tokens as POSIX plus the
// Windows path separator. Values are emitted in double quotes, so quotes,
// backticks, dollar signs, semicolons, whitespace, and other command syntax stay
// rejected rather than escaped.
var powerShellSafeValue = regexp.MustCompile(`^[A-Za-z0-9%._/@:=+~\\-]+$`)

// ParseDialect resolves a user-facing dialect token. Empty defaults to POSIX to
// preserve existing zsh/manual-eval behavior.
func ParseDialect(name string) (Dialect, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "posix", "sh", "zsh":
		return DialectPOSIX, nil
	case "powershell", "pwsh":
		return DialectPowerShell, nil
	default:
		return "", fmt.Errorf("unsupported shell dialect %q", name)
	}
}

// Export formats a single shell export statement: `export KEY=value`. It
// returns an error if value contains anything outside the shell-safe charset,
// so an unsafe value is never written to stdout for eval.
func Export(key, value string) (string, error) {
	lines, err := EmitAssignments(DialectPOSIX, []Assignment{{Key: key, Value: value}})
	if err != nil {
		return "", err
	}
	return lines[0], nil
}

// EmitAssignments formats environment updates for the selected shell dialect.
// It validates keys and values before producing any output.
func EmitAssignments(dialect Dialect, updates []Assignment) ([]string, error) {
	lines := make([]string, 0, len(updates))
	for _, update := range updates {
		line, err := emitAssignment(dialect, update)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func emitAssignment(dialect Dialect, update Assignment) (string, error) {
	if !safeKey.MatchString(update.Key) {
		return "", fmt.Errorf("refusing to emit unsafe environment key %q", update.Key)
	}
	switch dialect {
	case DialectPOSIX:
		if !posixSafeValue.MatchString(update.Value) {
			return "", fmt.Errorf("refusing to export unsafe value for %s: %q contains shell metacharacters", update.Key, update.Value)
		}
		return fmt.Sprintf("export %s=%s", update.Key, update.Value), nil
	case DialectPowerShell:
		if !powerShellSafeValue.MatchString(update.Value) {
			return "", fmt.Errorf("refusing to emit unsafe PowerShell value for %s: %q contains shell metacharacters", update.Key, update.Value)
		}
		return fmt.Sprintf(`$env:%s = "%s"`, update.Key, update.Value), nil
	default:
		return "", fmt.Errorf("unsupported shell dialect %q", dialect)
	}
}
