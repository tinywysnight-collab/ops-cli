package creds

import (
	"bytes"
	"fmt"
	"strings"
)

// Line-based AWS shared-credentials ini editing. It preserves comments, blank
// lines, key casing, and unrelated profiles/keys verbatim; only the managed STS
// keys of the *target* profile are written or removed. opsx never reformats the
// rest of the file.

// managedKeys are the only keys opsx writes/removes within a target profile.
var managedKeys = []string{keyAccessKeyID, keySecretAccessKey, keySessionToken}

func isManagedKey(k string) bool {
	for _, mk := range managedKeys {
		if k == mk {
			return true
		}
	}
	return false
}

func isHeader(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
		return strings.TrimSpace(t[1 : len(t)-1]), true
	}
	return "", false
}

func lineKey(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, ";") {
		return "", false
	}
	k, _, ok := strings.Cut(line, "=")
	if !ok {
		return "", false
	}
	return strings.TrimSpace(k), true
}

func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

// readProfile returns the managed STS keys present in the target profile.
func readProfile(data []byte, profile string) map[string]string {
	out := map[string]string{}
	lines := splitLines(data)
	inTarget := false
	for _, line := range lines {
		if name, ok := isHeader(line); ok {
			inTarget = name == profile
			continue
		}
		if !inTarget {
			continue
		}
		if k, ok := lineKey(line); ok && isManagedKey(k) {
			_, v, _ := strings.Cut(line, "=")
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}

// upsertProfile sets the managed keys present in kv (non-empty only) on the
// target profile and removes any managed key not in kv, preserving everything
// else verbatim. The section is created if absent.
func upsertProfile(data []byte, profile string, kv map[string]string) []byte {
	lines := splitLines(data)

	seenTarget := false
	insertedManaged := false
	inTarget := false
	out := make([]string, 0, len(lines)+len(managedKeys)+2)
	for _, line := range lines {
		if name, ok := isHeader(line); ok {
			inTarget = name == profile
			if inTarget {
				if !seenTarget {
					seenTarget = true
					out = append(out, line)
					out = append(out, managedLines(kv)...)
					insertedManaged = true
					continue
				}
				out = append(out, line)
				continue
			}
			out = append(out, line)
			continue
		}
		if inTarget {
			if k, ok := lineKey(line); ok && isManagedKey(k) {
				continue
			}
		}
		out = append(out, line)
	}

	if !seenTarget {
		return renderAppendedSection(lines, profile, kv)
	}
	if !insertedManaged {
		out = append(out, managedLines(kv)...)
	}
	return joinLines(out)
}

func deleteProfiles(data []byte, profiles map[string]struct{}) []byte {
	lines := splitLines(data)
	out := make([]string, 0, len(lines))
	dropping := false
	for _, line := range lines {
		if name, ok := isHeader(line); ok {
			_, dropping = profiles[name]
			if dropping {
				continue
			}
			out = append(out, line)
			continue
		}
		if dropping {
			continue
		}
		out = append(out, line)
	}
	return joinLines(out)
}

func renderAppendedSection(lines []string, profile string, kv map[string]string) []byte {
	out := append([]string{}, lines...)
	if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
		out = append(out, "")
	}
	out = append(out, fmt.Sprintf("[%s]", profile))
	out = append(out, managedLines(kv)...)
	return joinLines(out)
}

// managedLines renders the managed keys in canonical order, skipping empties.
func managedLines(kv map[string]string) []string {
	var out []string
	for _, k := range managedKeys {
		if v := kv[k]; v != "" {
			out = append(out, fmt.Sprintf("%s = %s", k, v))
		}
	}
	return out
}

func joinLines(lines []string) []byte {
	var b bytes.Buffer
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.Bytes()
}
