package creds_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/creds"
)

func newTempStore(t *testing.T) (*creds.Store, string) {
	t.Helper()
	dir := t.TempDir()
	credPath := filepath.Join(dir, "aws", "credentials")
	lockPath := filepath.Join(dir, ".opsx.lock")
	return creds.NewStore(credPath, lockPath), credPath
}

func TestStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, _ := newTempStore(t)
	want := creds.Credentials{AccessKeyID: "AKIA", SecretAccessKey: "secret", SessionToken: "token"}
	require.NoError(t, s.Write(ctx, "dev.admin", want))

	got, ok, err := s.Read("dev.admin")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, want, got)

	_, ok, err = s.Read("missing.profile")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestStorePreservesOtherProfiles(t *testing.T) {
	ctx := context.Background()
	s, _ := newTempStore(t)
	require.NoError(t, s.Write(ctx, "dev.admin", creds.Credentials{AccessKeyID: "A1", SecretAccessKey: "S1", SessionToken: "T1"}))
	require.NoError(t, s.Write(ctx, "prod.opr", creds.Credentials{AccessKeyID: "A2", SecretAccessKey: "S2", SessionToken: "T2"}))

	a, _, _ := s.Read("dev.admin")
	b, _, _ := s.Read("prod.opr")
	require.Equal(t, "A1", a.AccessKeyID)
	require.Equal(t, "A2", b.AccessKeyID)
}

// TestStorePreservesCommentsAndFormatting asserts the ini editor preserves
// comments, blank lines, and unrelated hand-formatted profiles verbatim, only
// rewriting the target profile's STS keys.
func TestStorePreservesCommentsAndFormatting(t *testing.T) {
	ctx := context.Background()
	s, credPath := newTempStore(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
	original := `# my AWS creds — hand maintained
; legacy section, do not touch
[legacy]
aws_access_key_id=LEGACYKEY
aws_secret_access_key=LEGACYSECRET
region = us-east-1

# end
`
	require.NoError(t, os.WriteFile(credPath, []byte(original), 0o600))

	require.NoError(t, s.Write(ctx, "dev.admin", creds.Credentials{AccessKeyID: "NEW", SecretAccessKey: "NEWSEC", SessionToken: "NEWTOK"}))

	data, err := os.ReadFile(credPath)
	require.NoError(t, err)
	out := string(data)

	require.Contains(t, out, "# my AWS creds — hand maintained")
	require.Contains(t, out, "; legacy section, do not touch")
	require.Contains(t, out, "[legacy]")
	require.Contains(t, out, "aws_access_key_id=LEGACYKEY") // unrelated key untouched (no reformat)
	require.Contains(t, out, "region = us-east-1")
	require.Contains(t, out, "# end")
	require.Contains(t, out, "[dev.admin]")

	got, ok, err := s.Read("dev.admin")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "NEW", got.AccessKeyID)
}

func TestStoreOmitsEmptySessionToken(t *testing.T) {
	ctx := context.Background()
	s, credPath := newTempStore(t)
	require.NoError(t, s.Write(ctx, "master_admin", creds.Credentials{AccessKeyID: "AK", SecretAccessKey: "SK"}))

	data, err := os.ReadFile(credPath)
	require.NoError(t, err)
	require.NotContains(t, string(data), "aws_session_token")
}

func TestStorePresentRequiresBothKeys(t *testing.T) {
	ctx := context.Background()
	s, _ := newTempStore(t)
	require.NoError(t, s.Write(ctx, "partial", creds.Credentials{AccessKeyID: "AK"})) // no secret
	_, ok, err := s.Read("partial")
	require.NoError(t, err)
	require.False(t, ok, "a profile with no secret access key is not present")
}

func TestStoreUpsertDuplicateTargetProfileLeavesNoStaleSecret(t *testing.T) {
	ctx := context.Background()
	s, credPath := newTempStore(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
	original := `[dev.admin]
aws_access_key_id = OLD1
aws_secret_access_key = OLDSECRET1
aws_session_token = OLDTOKEN1

[other]
aws_access_key_id = OTHER
aws_secret_access_key = OTHERSECRET

[dev.admin]
aws_access_key_id = OLD2
aws_secret_access_key = OLDSECRET2
aws_session_token = OLDTOKEN2
region = ap-southeast-2
`
	require.NoError(t, os.WriteFile(credPath, []byte(original), 0o600))

	require.NoError(t, s.Write(ctx, "dev.admin", creds.Credentials{AccessKeyID: "NEW", SecretAccessKey: "NEWSECRET", SessionToken: "NEWTOKEN"}))

	data, err := os.ReadFile(credPath)
	require.NoError(t, err)
	out := string(data)
	require.NotContains(t, out, "OLDSECRET1")
	require.NotContains(t, out, "OLDTOKEN1")
	require.NotContains(t, out, "OLDSECRET2")
	require.NotContains(t, out, "OLDTOKEN2")
	require.Contains(t, out, "region = ap-southeast-2")

	got, ok, err := s.Read("dev.admin")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, creds.Credentials{AccessKeyID: "NEW", SecretAccessKey: "NEWSECRET", SessionToken: "NEWTOKEN"}, got)
}

func TestStoreDeleteProfilesPreservesUnrelatedContent(t *testing.T) {
	ctx := context.Background()
	s, credPath := newTempStore(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
	original := `# keep
[legacy]
aws_access_key_id=LEGACY
aws_secret_access_key=SECRET

[dev.admin]
aws_access_key_id = DEV
aws_secret_access_key = DEVSECRET
aws_session_token = DEVTOKEN

[master_admin]
aws_access_key_id = MASTER
aws_secret_access_key = MASTERSECRET
aws_session_token = MASTERTOKEN
`
	require.NoError(t, os.WriteFile(credPath, []byte(original), 0o600))

	require.NoError(t, s.Delete(ctx, []string{"dev.admin", "master_admin"}))

	data, err := os.ReadFile(credPath)
	require.NoError(t, err)
	out := string(data)
	require.Contains(t, out, "# keep")
	require.Contains(t, out, "[legacy]")
	require.Contains(t, out, "aws_access_key_id=LEGACY")
	require.NotContains(t, out, "[dev.admin]")
	require.NotContains(t, out, "[master_admin]")
}

func TestStorePermissions(t *testing.T) {
	ctx := context.Background()
	s, credPath := newTempStore(t)
	require.NoError(t, s.Write(ctx, "dev.admin", creds.Credentials{AccessKeyID: "A", SecretAccessKey: "S", SessionToken: "T"}))

	fi, err := os.Stat(credPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), fi.Mode().Perm())

	di, err := os.Stat(filepath.Dir(credPath))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), di.Mode().Perm())
}

// TestConcurrentWrites exercises the flock serialization: many goroutines write
// distinct profiles at once; the file must remain valid and contain every one.
func TestConcurrentWrites(t *testing.T) {
	ctx := context.Background()
	s, _ := newTempStore(t)
	const n = 40

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			profile := fmt.Sprintf("acct%02d.admin", i)
			err := s.Write(ctx, profile, creds.Credentials{
				AccessKeyID:     fmt.Sprintf("AK%02d", i),
				SecretAccessKey: fmt.Sprintf("SK%02d", i),
				SessionToken:    fmt.Sprintf("ST%02d", i),
			})
			require.NoError(t, err)
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		profile := fmt.Sprintf("acct%02d.admin", i)
		got, ok, err := s.Read(profile)
		require.NoError(t, err)
		require.True(t, ok, "profile %s missing after concurrent writes", profile)
		require.Equal(t, fmt.Sprintf("AK%02d", i), got.AccessKeyID)
	}
}

func TestIsExpired(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	require.True(t, creds.IsExpired(now.Add(-time.Second), now))
	require.True(t, creds.IsExpired(now, now)) // exactly now counts as expired
	// Within the skew buffer → treated as expired.
	require.True(t, creds.IsExpired(now.Add(creds.ExpirySkew-time.Second), now))
	// Comfortably in the future → valid.
	require.False(t, creds.IsExpired(now.Add(time.Hour), now))
}
