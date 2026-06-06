package creds_test

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/creds"
	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

func citizenConfig() *config.Config {
	return &config.Config{
		Accounts: map[string]config.Account{"dev": {AccountID: "111111111111"}},
		Auth: config.Auth{
			MasterAccountID: "000000000000",
			Region:          "us-east-1",
			MasterRoles:     map[string]string{"admin": "master_admin", "opr": "master_AWSOpr"},
			CitizenRoles:    map[string]string{"admin": "Admin", "opr": "AWSOpr"},
		},
	}
}

func newCitizenStores(t *testing.T) (*creds.Store, *state.Store) {
	t.Helper()
	dir := t.TempDir()
	lock := dir + "/.opsx.lock"
	return creds.NewStore(dir+"/credentials", lock), state.NewStore(dir+"/state.json", lock)
}

func seedMaster(t *testing.T, cs *creds.Store, ss *state.Store, now time.Time) {
	t.Helper()
	require.NoError(t, cs.Write(context.Background(), "master_admin", creds.Credentials{AccessKeyID: "MASTER", SecretAccessKey: "ms", SessionToken: "mt"}))
	require.NoError(t, ss.Put(context.Background(), "master_admin", state.Entry{Expiry: now.Add(30 * time.Minute), Account: "master", Mode: "admin", UpdatedAt: now}))
}

func TestUseAssumesCitizenAndWritesProfile(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newCitizenStores(t)
	seedMaster(t, cs, ss, now)

	var gotRoleARN, gotSession string
	var gotMaster creds.Credentials
	svc := &creds.CitizenService{
		Cfg: citizenConfig(), Creds: cs, State: ss, Now: func() time.Time { return now },
		Assume: func(_ context.Context, master creds.Credentials, roleARN, session, _ string) (creds.Credentials, time.Time, error) {
			gotMaster, gotRoleARN, gotSession = master, roleARN, session
			return creds.Credentials{AccessKeyID: "CITIZEN", SecretAccessKey: "cs", SessionToken: "ct"}, now.Add(time.Hour), nil
		},
	}

	profile, err := svc.Use(context.Background(), "dev", "admin")
	require.NoError(t, err)
	require.Equal(t, "dev.admin", profile)
	require.Equal(t, "MASTER", gotMaster.AccessKeyID)
	require.Equal(t, "arn:aws:iam::111111111111:role/Admin", gotRoleARN)
	require.Contains(t, gotSession, "opsx-")

	got, ok, err := cs.Read("dev.admin")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "CITIZEN", got.AccessKeyID)

	entry, ok, err := ss.Get("dev.admin")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "dev", entry.Account)
	require.True(t, now.Add(time.Hour).Equal(entry.Expiry))
}

// TestUseAlsoWritesDefaultProfile asserts that `opsx use` mirrors the assumed
// citizen credentials into the shared [default] profile (single-user escape
// hatch for shells where AWS_PROFILE cannot be injected).
func TestUseAlsoWritesDefaultProfile(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newCitizenStores(t)
	seedMaster(t, cs, ss, now)
	svc := &creds.CitizenService{
		Cfg: citizenConfig(), Creds: cs, State: ss, Now: func() time.Time { return now },
		Assume: func(context.Context, creds.Credentials, string, string, string) (creds.Credentials, time.Time, error) {
			return creds.Credentials{AccessKeyID: "CITIZEN", SecretAccessKey: "cs", SessionToken: "ct"}, now.Add(time.Hour), nil
		},
	}

	_, err := svc.Use(context.Background(), "dev", "admin")
	require.NoError(t, err)

	def, ok, err := cs.Read("default")
	require.NoError(t, err)
	require.True(t, ok, "[default] must be written on use")
	require.Equal(t, "CITIZEN", def.AccessKeyID)
	require.Equal(t, "ct", def.SessionToken)
}

// TestUseRewritesDefaultOnReuse asserts that even when the citizen profile is
// reused (no fresh AssumeRole), [default] is refreshed to point at this account
// so it always reflects the latest `opsx use`.
func TestUseRewritesDefaultOnReuse(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newCitizenStores(t)
	seedMaster(t, cs, ss, now)
	assumeCalls := 0
	svc := &creds.CitizenService{
		Cfg: citizenConfig(), Creds: cs, State: ss, Now: func() time.Time { return now },
		Assume: func(context.Context, creds.Credentials, string, string, string) (creds.Credentials, time.Time, error) {
			assumeCalls++
			return creds.Credentials{AccessKeyID: "CITIZEN", SecretAccessKey: "cs", SessionToken: "ct"}, now.Add(time.Hour), nil
		},
	}

	_, err := svc.Use(context.Background(), "dev", "admin")
	require.NoError(t, err)
	// Simulate another account having claimed [default] since the first use.
	require.NoError(t, cs.Write(context.Background(), "default", creds.Credentials{AccessKeyID: "OTHER", SecretAccessKey: "x", SessionToken: "y"}))

	_, err = svc.Use(context.Background(), "dev", "admin")
	require.NoError(t, err)

	require.Equal(t, 1, assumeCalls, "second use reuses cached citizen creds")
	def, ok, err := cs.Read("default")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "CITIZEN", def.AccessKeyID, "[default] must be refreshed even on cache reuse")
}

// TestUsePassesConfiguredRegionToAssume asserts the per-account region resolved
// from config reaches the Assume seam (so the STS client is built with the
// configured region rather than depending on ambient AWS_REGION).
func TestUsePassesConfiguredRegionToAssume(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newCitizenStores(t)
	seedMaster(t, cs, ss, now)
	cfg := citizenConfig()
	cfg.Accounts["dev"] = config.Account{AccountID: "111111111111", Region: "ap-southeast-2"}

	var gotRegion string
	svc := &creds.CitizenService{
		Cfg: cfg, Creds: cs, State: ss, Now: func() time.Time { return now },
		Assume: func(_ context.Context, _ creds.Credentials, _, _, region string) (creds.Credentials, time.Time, error) {
			gotRegion = region
			return creds.Credentials{AccessKeyID: "CITIZEN", SecretAccessKey: "cs", SessionToken: "ct"}, now.Add(time.Hour), nil
		},
	}

	_, err := svc.Use(context.Background(), "dev", "admin")
	require.NoError(t, err)
	require.Equal(t, "ap-southeast-2", gotRegion)
}

// TestUseReusesUnexpiredCitizen asserts no redundant AssumeRole within the
// validity window (R13).
func TestUseReusesUnexpiredCitizen(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newCitizenStores(t)
	seedMaster(t, cs, ss, now)

	assumeCalls := 0
	svc := &creds.CitizenService{
		Cfg: citizenConfig(), Creds: cs, State: ss, Now: func() time.Time { return now },
		Assume: func(context.Context, creds.Credentials, string, string, string) (creds.Credentials, time.Time, error) {
			assumeCalls++
			return creds.Credentials{AccessKeyID: "CITIZEN", SecretAccessKey: "cs", SessionToken: "ct"}, now.Add(time.Hour), nil
		},
	}

	_, err := svc.Use(context.Background(), "dev", "admin")
	require.NoError(t, err)
	_, err = svc.Use(context.Background(), "dev", "admin")
	require.NoError(t, err)

	require.Equal(t, 1, assumeCalls, "second use within validity window must reuse cached citizen creds")
}

// TestUseReassumesWhenCitizenStale asserts a stale cached citizen triggers a
// fresh AssumeRole rather than reuse.
func TestUseReassumesWhenCitizenStale(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newCitizenStores(t)
	seedMaster(t, cs, ss, now)
	// Pre-seed a stale citizen profile.
	require.NoError(t, cs.Write(context.Background(), "dev.admin", creds.Credentials{AccessKeyID: "OLD", SecretAccessKey: "x", SessionToken: "y"}))
	require.NoError(t, ss.Put(context.Background(), "dev.admin", state.Entry{Expiry: now.Add(-time.Minute), Account: "dev", Mode: "admin", UpdatedAt: now}))

	assumeCalls := 0
	svc := &creds.CitizenService{
		Cfg: citizenConfig(), Creds: cs, State: ss, Now: func() time.Time { return now },
		Assume: func(context.Context, creds.Credentials, string, string, string) (creds.Credentials, time.Time, error) {
			assumeCalls++
			return creds.Credentials{AccessKeyID: "FRESH", SecretAccessKey: "cs", SessionToken: "ct"}, now.Add(time.Hour), nil
		},
	}
	_, err := svc.Use(context.Background(), "dev", "admin")
	require.NoError(t, err)
	require.Equal(t, 1, assumeCalls)
	got, _, _ := cs.Read("dev.admin")
	require.Equal(t, "FRESH", got.AccessKeyID)
}

func TestUseDoesNotReuseOrphanedCredentialWithoutState(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newCitizenStores(t)
	seedMaster(t, cs, ss, now)
	require.NoError(t, cs.Write(context.Background(), "dev.admin", creds.Credentials{AccessKeyID: "ORPHAN", SecretAccessKey: "old", SessionToken: "oldtoken"}))

	assumeCalls := 0
	svc := &creds.CitizenService{
		Cfg: citizenConfig(), Creds: cs, State: ss, Now: func() time.Time { return now },
		Assume: func(context.Context, creds.Credentials, string, string, string) (creds.Credentials, time.Time, error) {
			assumeCalls++
			return creds.Credentials{AccessKeyID: "FRESH", SecretAccessKey: "cs", SessionToken: "ct"}, now.Add(time.Hour), nil
		},
	}

	profile, err := svc.Use(context.Background(), "dev", "admin")
	require.NoError(t, err)
	require.Equal(t, "dev.admin", profile)
	require.Equal(t, 1, assumeCalls)
	got, _, _ := cs.Read("dev.admin")
	require.Equal(t, "FRESH", got.AccessKeyID)
}

func TestUseValidatesAliasBeforeCachedReuse(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newCitizenStores(t)
	require.NoError(t, cs.Write(context.Background(), "removed.admin", creds.Credentials{AccessKeyID: "OLD", SecretAccessKey: "old", SessionToken: "oldtoken"}))
	require.NoError(t, ss.Put(context.Background(), "removed.admin", state.Entry{Expiry: now.Add(time.Hour), Account: "removed", Mode: "admin", UpdatedAt: now}))

	called := false
	svc := &creds.CitizenService{
		Cfg: citizenConfig(), Creds: cs, State: ss, Now: func() time.Time { return now },
		Assume: func(context.Context, creds.Credentials, string, string, string) (creds.Credentials, time.Time, error) {
			called = true
			return creds.Credentials{}, time.Time{}, nil
		},
	}

	_, err := svc.Use(context.Background(), "removed", "admin")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown account")
	require.False(t, called)
}

func TestUseDoesNotReuseCitizenProfileWithoutSessionToken(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newCitizenStores(t)
	seedMaster(t, cs, ss, now)
	require.NoError(t, cs.Write(context.Background(), "dev.admin", creds.Credentials{AccessKeyID: "OLD", SecretAccessKey: "old"}))
	require.NoError(t, ss.Put(context.Background(), "dev.admin", state.Entry{Expiry: now.Add(time.Hour), Account: "dev", Mode: "admin", UpdatedAt: now}))

	assumeCalls := 0
	svc := &creds.CitizenService{
		Cfg: citizenConfig(), Creds: cs, State: ss, Now: func() time.Time { return now },
		Assume: func(context.Context, creds.Credentials, string, string, string) (creds.Credentials, time.Time, error) {
			assumeCalls++
			return creds.Credentials{AccessKeyID: "FRESH", SecretAccessKey: "cs", SessionToken: "ct"}, now.Add(time.Hour), nil
		},
	}

	_, err := svc.Use(context.Background(), "dev", "admin")
	require.NoError(t, err)
	require.Equal(t, 1, assumeCalls)
}

func TestUseFailsWhenMasterHasNoSessionToken(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newCitizenStores(t)
	require.NoError(t, cs.Write(context.Background(), "master_admin", creds.Credentials{AccessKeyID: "M", SecretAccessKey: "s"}))
	require.NoError(t, ss.Put(context.Background(), "master_admin", state.Entry{Expiry: now.Add(time.Hour), Mode: "admin", UpdatedAt: now}))

	svc := &creds.CitizenService{
		Cfg: citizenConfig(), Creds: cs, State: ss, Now: func() time.Time { return now },
		Assume: func(context.Context, creds.Credentials, string, string, string) (creds.Credentials, time.Time, error) {
			return creds.Credentials{}, time.Time{}, nil
		},
	}
	_, err := svc.Use(context.Background(), "dev", "admin")
	require.ErrorIs(t, err, creds.ErrMasterExpired)
}

func TestConcurrentUseSingleFlightsAssume(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newCitizenStores(t)
	seedMaster(t, cs, ss, now)

	var mu sync.Mutex
	assumeCalls := 0
	svc := &creds.CitizenService{
		Cfg: citizenConfig(), Creds: cs, State: ss, Now: func() time.Time { return now },
		Assume: func(context.Context, creds.Credentials, string, string, string) (creds.Credentials, time.Time, error) {
			mu.Lock()
			assumeCalls++
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			return creds.Credentials{AccessKeyID: "FRESH", SecretAccessKey: "cs", SessionToken: "ct"}, now.Add(time.Hour), nil
		},
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Use(context.Background(), "dev", "admin")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, 1, assumeCalls)
}

func TestUseFailsWhenMasterExpired(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newCitizenStores(t)
	require.NoError(t, cs.Write(context.Background(), "master_admin", creds.Credentials{AccessKeyID: "M", SecretAccessKey: "s", SessionToken: "t"}))
	require.NoError(t, ss.Put(context.Background(), "master_admin", state.Entry{Expiry: now.Add(-time.Minute), Mode: "admin", UpdatedAt: now}))

	called := false
	svc := &creds.CitizenService{
		Cfg: citizenConfig(), Creds: cs, State: ss, Now: func() time.Time { return now },
		Assume: func(context.Context, creds.Credentials, string, string, string) (creds.Credentials, time.Time, error) {
			called = true
			return creds.Credentials{}, time.Time{}, nil
		},
	}

	_, err := svc.Use(context.Background(), "dev", "admin")
	require.ErrorIs(t, err, creds.ErrMasterExpired)
	require.False(t, called, "must not attempt assume when master is expired")
}

func TestUseFailsWhenNoMaster(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newCitizenStores(t)
	svc := &creds.CitizenService{
		Cfg: citizenConfig(), Creds: cs, State: ss, Now: func() time.Time { return now },
		Assume: func(context.Context, creds.Credentials, string, string, string) (creds.Credentials, time.Time, error) {
			return creds.Credentials{}, time.Time{}, nil
		},
	}
	_, err := svc.Use(context.Background(), "dev", "admin")
	require.ErrorIs(t, err, creds.ErrMasterExpired)
}

func TestSessionNameFromSanitizes(t *testing.T) {
	stsRe := regexp.MustCompile(`^[\w+=,.@-]{2,64}$`)
	cases := []string{`ACME\jane doe`, "", "плохой", "a/b:c*d", "x", string(make([]byte, 200))}
	for _, raw := range cases {
		got := creds.SessionNameFrom(raw)
		require.Truef(t, stsRe.MatchString(got), "session name %q (from %q) must satisfy STS regex", got, raw)
	}
}

func TestSessionNameFromTruncatesSafely(t *testing.T) {
	stsRe := regexp.MustCompile(`^[\w+=,.@-]{2,64}$`)
	cases := []struct {
		name string
		raw  string
	}{
		// Non-ASCII followed by a long NUL pad: exercises rune-safe truncation.
		{name: "non-ascii then nul pad", raw: "用户" + string(make([]byte, 200))},
		// Mixed shell- and STS-illegal metacharacters padded past the limit:
		// exercises real sanitization, not just NUL stripping.
		{name: "shell metachars then pad", raw: `ACME\jane; rm -rf ~ && echo $(whoami) | cat #` + strings.Repeat("a", 100)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := creds.SessionNameFrom(tc.raw)
			require.True(t, utf8.ValidString(got))
			require.LessOrEqual(t, len(got), 64)
			require.Regexp(t, stsRe, got)
		})
	}
}
