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
	DialectCmd        Dialect = "cmd"
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

// cmdSafeValue allows opsx-managed tokens plus Windows path separators. Values
// are emitted with `set "KEY=value"` quoting, so cmd metacharacters such as &,
// |, <, >, quotes, whitespace, and command substitution are rejected outright.
var cmdSafeValue = regexp.MustCompile(`^[A-Za-z0-9%._/@:=+~\\-]+$`)

// spacedPathSafeValue is the one relaxation of the strict charsets: real
// filesystem paths (Windows user directories) legitimately contain spaces.
// It adds ONLY the space character — quotes and every other metacharacter
// stay rejected — and because the charset still excludes the single quote,
// wrapping the value in single quotes needs no escaping rule at all. Only
// KUBECONFIG (the one opsx-constructed path value) may use it; profile and
// mode tokens stay under the strict charsets.
var (
	spacedPosixSafeValue      = regexp.MustCompile(`^[A-Za-z0-9%._/@:=+~ -]+$`)
	spacedPowerShellSafeValue = regexp.MustCompile(`^[A-Za-z0-9%._/@:=+~\\ -]+$`)
	spacedCmdSafeValue        = regexp.MustCompile(`^[A-Za-z0-9%._/@:=+~\\ -]+$`)
)

// spacedPathKey reports whether key may carry a space-containing path value.
func spacedPathKey(key string) bool { return key == "KUBECONFIG" }

// ParseDialect resolves a user-facing dialect token. Empty defaults to POSIX to
// preserve existing zsh/manual-eval behavior.
func ParseDialect(name string) (Dialect, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "posix", "sh", "bash", "zsh":
		return DialectPOSIX, nil
	case "powershell", "pwsh":
		return DialectPowerShell, nil
	case "cmd", "cmd.exe", "command-prompt":
		return DialectCmd, nil
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
		// On Windows, opsx-built paths (the kubeconfig path) use backslash
		// separators. A backslash is an escape character in POSIX/Bash shells
		// (Git Bash), so normalize separators to forward slashes — Bash treats
		// them literally and Windows kubectl/aws accept them — rather than
		// rejecting the path. opsx only ever emits paths, profile names, and the
		// mode token as POSIX values, so no legitimate value loses meaning here.
		value := strings.ReplaceAll(update.Value, `\`, "/")
		if spacedPathKey(update.Key) && strings.Contains(value, " ") {
			if !spacedPosixSafeValue.MatchString(value) {
				return "", fmt.Errorf("refusing to export unsafe value for %s: %q contains shell metacharacters", update.Key, value)
			}
			// The charset forbids single quotes, so this quoting is escape-free.
			return fmt.Sprintf("export %s='%s'", update.Key, value), nil
		}
		if !posixSafeValue.MatchString(value) {
			return "", fmt.Errorf("refusing to export unsafe value for %s: %q contains shell metacharacters", update.Key, value)
		}
		return fmt.Sprintf("export %s=%s", update.Key, value), nil
	case DialectPowerShell:
		if spacedPathKey(update.Key) && strings.Contains(update.Value, " ") {
			if !spacedPowerShellSafeValue.MatchString(update.Value) {
				return "", fmt.Errorf("refusing to emit unsafe PowerShell value for %s: %q contains shell metacharacters", update.Key, update.Value)
			}
			return fmt.Sprintf(`$env:%s = '%s'`, update.Key, update.Value), nil
		}
		if !powerShellSafeValue.MatchString(update.Value) {
			return "", fmt.Errorf("refusing to emit unsafe PowerShell value for %s: %q contains shell metacharacters", update.Key, update.Value)
		}
		return fmt.Sprintf(`$env:%s = "%s"`, update.Key, update.Value), nil
	case DialectCmd:
		if spacedPathKey(update.Key) && strings.Contains(update.Value, " ") {
			if !spacedCmdSafeValue.MatchString(update.Value) {
				return "", fmt.Errorf("refusing to emit unsafe cmd value for %s: %q contains shell metacharacters", update.Key, update.Value)
			}
			return fmt.Sprintf(`set "%s=%s"`, update.Key, update.Value), nil
		}
		if !cmdSafeValue.MatchString(update.Value) {
			return "", fmt.Errorf("refusing to emit unsafe cmd value for %s: %q contains shell metacharacters", update.Key, update.Value)
		}
		return fmt.Sprintf(`set "%s=%s"`, update.Key, update.Value), nil
	default:
		return "", fmt.Errorf("unsupported shell dialect %q", dialect)
	}
}
