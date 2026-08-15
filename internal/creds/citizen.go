package creds

import (
	"context"
	"fmt"
	"os/user"
	"regexp"
	"sync"
	"time"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/lock"
	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

// DefaultProfile is the shared AWS default profile. It is opsx-managed only for
// cleanup of older caches; switching commands do not write it because it is a
// single latest-wins section and therefore cannot provide terminal isolation.
const DefaultProfile = "default"

// Assumer performs an STS AssumeRole using the given master credentials as the
// source identity, in the given AWS region, and returns the resulting citizen
// credentials and expiry. It is a seam so the AWS call is injectable in tests.
// The region is required: AWS SDK Go v2 needs it to resolve the STS endpoint.
type Assumer func(ctx context.Context, master Credentials, roleARN, sessionName, region string) (Credentials, time.Time, error)

// CitizenService implements `opsx use`: from cached master credentials, assume a
// citizen role and write the per-(alias,mode) profile.
type CitizenService struct {
	Cfg    *config.Config
	Creds  *Store
	State  *state.Store
	Assume Assumer
	Now    func() time.Time
}

// citizenProfileLocks single-flights AssumeRole per profile within ONE process
// (proven by TestConcurrentUseSingleFlightsAssume): concurrent `Use` calls for
// the same profile share one file-lock window instead of racing to it. The
// cross-process guarantee comes from the shared advisory file lock inside
// useUnderSharedLock (proven by
// TestUseUnderSharedLockSingleFlightsAcrossServiceInstances). The sync.Map is
// intentionally unbounded: opsx is a short-lived CLI, so the handful of profile
// mutexes it accrues are reclaimed when the process exits. See task 12.6.
var citizenProfileLocks sync.Map // profile -> *sync.Mutex

func citizenProfileLock(profile string) *sync.Mutex {
	actual, _ := citizenProfileLocks.LoadOrStore(profile, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// Use ensures a valid citizen profile for alias in the given mode and returns
// its profile name. If an unexpired citizen profile is already cached it is
// reused without a second AssumeRole; otherwise it assumes the citizen role
// using cached master credentials (returning ErrMasterExpired if those are
// missing or stale). The assume-and-write path runs inside the shared advisory
// file lock and re-checks reuse there, so two opsx PROCESSES switching to the
// same profile at the same time issue at most one AssumeRole (credential-store
// "Single-flight citizen switch"); the second process reuses what the first
// wrote. The STS call's duration is spent holding the lock, so other opsx
// writers wait at most the bounded lock-acquisition timeout.
func (s *CitizenService) Use(ctx context.Context, alias, mode string) (string, error) {
	mode, err := config.NormalizeMode(mode)
	if err != nil {
		return "", err
	}
	if _, ok := s.Cfg.Accounts[alias]; !ok {
		return "", fmt.Errorf("unknown account alias %q", alias)
	}
	profile := config.CitizenProfile(alias, mode)

	mu := citizenProfileLock(profile)
	mu.Lock()
	defer mu.Unlock()

	// Lock-free fast path: reuse a cached, unexpired profile without
	// contending on the shared file lock (atomic writes keep reads
	// consistent). Only a miss pays for the locked transaction.
	if reusable, err := s.citizenReusable(profile); err != nil {
		return "", err
	} else if reusable {
		return profile, nil
	}

	if err := s.useUnderSharedLock(ctx, profile, alias, mode); err != nil {
		return "", err
	}
	return profile, nil
}

// useUnderSharedLock runs the miss path of Use as one transaction under the
// shared advisory file lock: re-check reuse (another process may have written
// while we waited), assume, then commit credentials and state in the SAME
// lock window. Everything it calls must be lock-free or *SharedLockHeld —
// acquiring the lock again here would self-deadlock.
func (s *CitizenService) useUnderSharedLock(ctx context.Context, profile, alias, mode string) error {
	return lock.With(ctx, s.Creds.lockPath, func() error {
		if reusable, err := s.citizenReusable(profile); err != nil {
			return err
		} else if reusable {
			return nil
		}

		master, err := s.validMaster(mode)
		if err != nil {
			return err
		}

		roleARN, err := s.Cfg.CitizenRoleARN(alias, mode)
		if err != nil {
			return err
		}

		region, err := s.Cfg.ResolveCitizenRegion(alias)
		if err != nil {
			return err
		}

		citizen, expiry, err := s.Assume(ctx, master, roleARN, SessionName(), region)
		if err != nil {
			return fmt.Errorf("assume citizen role %s: %w", alias, err)
		}

		if err := s.Creds.writeProfile(profile, citizen); err != nil {
			return err
		}
		return s.State.PutSharedLockHeld(profile, state.Entry{
			Expiry:    expiry,
			Account:   alias,
			Mode:      mode,
			UpdatedAt: s.now(),
		})
	})
}

// citizenReusable reports whether the citizen profile is cached and unexpired.
func (s *CitizenService) citizenReusable(profile string) (bool, error) {
	c, ok, err := s.Creds.Read(profile)
	if err != nil || !ok {
		return false, err
	}
	if !c.HasSessionToken() {
		return false, nil
	}
	entry, ok, err := s.State.Get(profile)
	if err != nil {
		return false, err
	}
	return ok && !IsExpired(entry.Expiry, s.now()), nil
}

// validMaster returns the cached master credentials for mode, or ErrMasterExpired.
func (s *CitizenService) validMaster(mode string) (Credentials, error) {
	masterProfile, err := config.MasterProfile(mode)
	if err != nil {
		return Credentials{}, err
	}
	master, ok, err := s.Creds.Read(masterProfile)
	if err != nil {
		return Credentials{}, err
	}
	if !ok {
		return Credentials{}, fmt.Errorf("no master credentials for mode %q: %w", mode, ErrMasterExpired)
	}
	if !master.HasSessionToken() {
		// A master profile without a session token is incomplete rather than
		// strictly expired, but the user-facing remedy is identical (re-login),
		// so it is intentionally surfaced as ErrMasterExpired. Callers must not
		// rely on errors.Is(err, ErrMasterExpired) to mean "the clock lapsed"
		// versus "the credential is corrupt/partial". See task 12.5.
		return Credentials{}, fmt.Errorf("master credentials for mode %q are missing a session token: %w", mode, ErrMasterExpired)
	}
	if entry, ok, err := s.State.Get(masterProfile); err != nil {
		return Credentials{}, err
	} else if !ok || IsExpired(entry.Expiry, s.now()) {
		return Credentials{}, fmt.Errorf("master credentials for mode %q are stale: %w", mode, ErrMasterExpired)
	}
	return master, nil
}

func (s *CitizenService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// stsSessionDisallowed matches characters not permitted in an STS RoleSessionName.
var stsSessionDisallowed = regexp.MustCompile(`[^\w+=,.@-]`)

// SessionName returns an auditable STS session name (opsx-<os-user>) for
// CloudTrail, guaranteed to satisfy the STS constraint [\w+=,.@-]{2,64}.
func SessionName() string {
	raw := ""
	if u, err := user.Current(); err == nil {
		raw = u.Username
	}
	return SessionNameFrom(raw)
}

// SessionNameFrom builds a valid STS session name from a raw username,
// sanitizing disallowed characters and truncating to 64 (exported for testing).
func SessionNameFrom(raw string) string {
	name := "opsx-" + stsSessionDisallowed.ReplaceAllString(raw, "-")
	if len(name) > 64 {
		name = name[:64]
	}
	return name // "opsx-" prefix guarantees the 2..64 length and charset
}
