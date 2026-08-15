// Package state persists per-profile expiry/context metadata in state.json,
// kept separate from ~/.aws/credentials so it never pollutes AWS CLI parsing.
// Writes go through the shared advisory lock and are atomic (temp + rename).
package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/tinywysnight-collab/ops-cli/internal/fsutil"
	"github.com/tinywysnight-collab/ops-cli/internal/lock"
)

// Entry records the lifecycle metadata for a written profile.
type Entry struct {
	Expiry    time.Time `json:"expiry"`
	Account   string    `json:"account"`
	Mode      string    `json:"mode"`
	Cluster   string    `json:"cluster,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store reads and writes state.json, keyed by profile name.
type Store struct {
	path     string
	lockPath string
}

// NewStore returns a Store for the state file at path, serialized by the
// advisory lock at lockPath.
func NewStore(path, lockPath string) *Store {
	return &Store{path: path, lockPath: lockPath}
}

// Load returns all entries keyed by profile name (empty map if the file is
// absent). Reads are lock-free: atomic writes guarantee a consistent view.
func (s *Store) Load() (map[string]Entry, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Entry{}, nil
		}
		return nil, fmt.Errorf("read state %s: %w", s.path, err)
	}
	if len(data) == 0 {
		return map[string]Entry{}, nil
	}
	m := map[string]Entry{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse state %s: %w", s.path, err)
	}
	// A literal JSON `null` decodes into a nil map; hand it back as empty so
	// the next Put does not panic on assignment to nil.
	if m == nil {
		m = map[string]Entry{}
	}
	return m, nil
}

// Get returns a single profile's entry and whether it was present.
func (s *Store) Get(profile string) (Entry, bool, error) {
	m, err := s.Load()
	if err != nil {
		return Entry{}, false, err
	}
	e, ok := m[profile]
	return e, ok, nil
}

// Put upserts a single profile's entry under the lock, writing atomically. The
// file ends up 0600 and its directory 0700.
func (s *Store) Put(ctx context.Context, profile string, e Entry) error {
	return lock.With(ctx, s.lockPath, func() error {
		return s.put(profile, e)
	})
}

// PutSharedLockHeld upserts a single profile's entry WITHOUT acquiring the
// shared lock. The caller MUST already hold the opsx lock at this store's
// lockPath — it exists so multi-file transactions (credentials plus state,
// see creds.CitizenService.Use) commit inside one lock window. Acquiring the
// lock through the normal Put from inside such a window would self-deadlock.
func (s *Store) PutSharedLockHeld(profile string, e Entry) error {
	return s.put(profile, e)
}

func (s *Store) put(profile string, e Entry) error {
	m, err := s.Load()
	if err != nil {
		return err
	}
	m[profile] = e
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	return fsutil.AtomicWrite(s.path, data, 0o600)
}

// Update applies fn to a profile entry under the shared state lock, then writes
// the resulting map atomically. It lets callers merge small annotations (for
// example the active cluster) without writing back a stale entry read before
// another process refreshed expiry/account/mode.
func (s *Store) Update(ctx context.Context, profile string, fn func(Entry, bool) (Entry, error)) error {
	return lock.With(ctx, s.lockPath, func() error {
		m, err := s.Load()
		if err != nil {
			return err
		}
		current, ok := m[profile]
		next, err := fn(current, ok)
		if err != nil {
			return err
		}
		m[profile] = next
		data, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return fmt.Errorf("encode state: %w", err)
		}
		return fsutil.AtomicWrite(s.path, data, 0o600)
	})
}

// Delete removes profile entries from state.json under the shared lock.
func (s *Store) Delete(ctx context.Context, profiles []string) error {
	targets := deleteTargets(profiles)
	if len(targets) == 0 {
		return nil
	}
	return lock.With(ctx, s.lockPath, func() error {
		return s.deleteProfiles(targets)
	})
}

// DeleteSharedLockHeld removes profile entries WITHOUT acquiring the shared
// lock. The caller MUST already hold the opsx lock at this store's lockPath
// (logout composes credential and state deletion in one window).
func (s *Store) DeleteSharedLockHeld(profiles []string) error {
	targets := deleteTargets(profiles)
	if len(targets) == 0 {
		return nil
	}
	return s.deleteProfiles(targets)
}

func deleteTargets(profiles []string) map[string]struct{} {
	targets := map[string]struct{}{}
	for _, profile := range profiles {
		targets[profile] = struct{}{}
	}
	return targets
}

func (s *Store) deleteProfiles(targets map[string]struct{}) error {
	m, err := s.Load()
	if err != nil {
		return err
	}
	for profile := range targets {
		delete(m, profile)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	return fsutil.AtomicWrite(s.path, data, 0o600)
}
