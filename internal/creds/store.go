package creds

import (
	"context"
	"fmt"
	"os"

	"github.com/tinywysnight-collab/ops-cli/internal/fsutil"
	"github.com/tinywysnight-collab/ops-cli/internal/lock"
)

// Profile key names in the AWS shared credentials file.
const (
	keyAccessKeyID     = "aws_access_key_id"
	keySecretAccessKey = "aws_secret_access_key"
	keySessionToken    = "aws_session_token"
)

// Credentials is a set of STS credentials written as one profile.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

// HasSessionToken reports whether these credentials are a complete STS session.
// opsx-managed master and citizen profiles are assumed-role credentials and
// must include a session token before they can be trusted as cached sessions.
func (c Credentials) HasSessionToken() bool {
	return c.AccessKeyID != "" && c.SecretAccessKey != "" && c.SessionToken != ""
}

// Store reads and writes profiles in the AWS shared credentials file. Writes
// happen under the shared advisory lock and are atomic (temp + rename), so
// concurrent writers from multiple terminals cannot corrupt the file.
type Store struct {
	path     string
	lockPath string
}

// NewStore returns a Store for the credentials file at path, serialized by the
// advisory lock at lockPath.
func NewStore(path, lockPath string) *Store {
	return &Store{path: path, lockPath: lockPath}
}

// Write upserts a single profile's STS credentials, preserving comments, blank
// lines, and all other profiles. Empty values are omitted (no `key = ` lines).
// The file is created 0600 and its directory 0700.
func (s *Store) Write(ctx context.Context, profile string, c Credentials) error {
	kv := map[string]string{
		keyAccessKeyID:     c.AccessKeyID,
		keySecretAccessKey: c.SecretAccessKey,
		keySessionToken:    c.SessionToken,
	}
	return lock.With(ctx, s.lockPath, func() error {
		data, err := s.read()
		if err != nil {
			return err
		}
		return fsutil.AtomicWrite(s.path, upsertProfile(data, profile, kv), 0o600)
	})
}

// Delete removes complete profile sections, preserving unrelated content.
func (s *Store) Delete(ctx context.Context, profiles []string) error {
	targets := map[string]struct{}{}
	for _, profile := range profiles {
		targets[profile] = struct{}{}
	}
	if len(targets) == 0 {
		return nil
	}
	return lock.With(ctx, s.lockPath, func() error {
		data, err := s.read()
		if err != nil {
			return err
		}
		return fsutil.AtomicWrite(s.path, deleteProfiles(data, targets), 0o600)
	})
}

// Read returns a profile's credentials and whether it is present. A profile is
// "present" only when both the access key ID and secret access key are set.
// Reads are lock-free: atomic writes guarantee a consistent view.
func (s *Store) Read(profile string) (Credentials, bool, error) {
	data, err := s.read()
	if err != nil {
		return Credentials{}, false, err
	}
	kv := readProfile(data, profile)
	c := Credentials{
		AccessKeyID:     kv[keyAccessKeyID],
		SecretAccessKey: kv[keySecretAccessKey],
		SessionToken:    kv[keySessionToken],
	}
	present := c.AccessKeyID != "" && c.SecretAccessKey != ""
	return c, present, nil
}

func (s *Store) read() ([]byte, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read credentials %s: %w", s.path, err)
	}
	return data, nil
}
