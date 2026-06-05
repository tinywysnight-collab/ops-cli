package creds

import (
	"errors"
	"time"
)

// ExpirySkew treats credentials as expired this long before their real expiry,
// so a switch never assumes with about-to-lapse master credentials.
const ExpirySkew = 2 * time.Minute

// ErrMasterExpired indicates cached master STS credentials are missing or
// expired; it is detected via errors.Is and surfaced by the top-level handler
// as an actionable re-login message.
var ErrMasterExpired = errors.New("master credentials expired")

// IsExpired reports whether expiry is at/before now + ExpirySkew. The clock is
// passed in so callers (and tests) control time with no wall-clock dependency.
func IsExpired(expiry, now time.Time) bool {
	return !now.Add(ExpirySkew).Before(expiry)
}
