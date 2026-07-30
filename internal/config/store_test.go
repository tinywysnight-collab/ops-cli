package config_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

func TestConfigStoreTransactions(t *testing.T) {
	t.Run("concurrent additions merge", func(t *testing.T) {
		path := writeConfig(t, validYAML)
		store := config.NewStore(path, filepath.Join(filepath.Dir(path), ".lock"))
		accounts := map[string]config.Account{
			"qa":    {AccountID: "333333333333", Region: "us-east-1"},
			"stage": {AccountID: "444444444444", Region: "ap-southeast-2"},
		}
		var wg sync.WaitGroup
		errs := make(chan error, len(accounts))
		for alias, account := range accounts {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- store.AddAccount(context.Background(), alias, account)
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		cfg, err := config.Load(path)
		require.NoError(t, err)
		require.Contains(t, cfg.Accounts, "qa")
		require.Contains(t, cfg.Accounts, "stage")
	})

	t.Run("latest duplicate is detected inside transaction", func(t *testing.T) {
		path := writeConfig(t, validYAML)
		store := config.NewStore(path, filepath.Join(filepath.Dir(path), ".lock"))
		require.NoError(t, store.AddAccount(context.Background(), "qa", config.Account{
			AccountID: "333333333333",
			Region:    "us-east-1",
		}))
		err := store.AddAccount(context.Background(), "qa", config.Account{
			AccountID: "444444444444",
			Region:    "us-east-1",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "already exists")
	})

	t.Run("preserves permissions and creates no backup", func(t *testing.T) {
		path := writeConfig(t, validYAML)
		require.NoError(t, os.Chmod(path, 0o640))
		store := config.NewStore(path, filepath.Join(filepath.Dir(path), ".lock"))
		require.NoError(t, store.AddAccount(context.Background(), "qa", config.Account{
			AccountID: "333333333333",
			Region:    "us-east-1",
		}))
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o640), info.Mode().Perm())
		_, err = os.Stat(path + ".bak")
		require.True(t, os.IsNotExist(err))
	})

	t.Run("preserves symlink", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privileges on Windows")
		}
		dir := t.TempDir()
		target := filepath.Join(dir, "real.yaml")
		require.NoError(t, os.WriteFile(target, []byte(validYAML), 0o600))
		link := filepath.Join(dir, "config.yaml")
		require.NoError(t, os.Symlink(target, link))
		store := config.NewStore(link, filepath.Join(dir, ".lock"))
		require.NoError(t, store.AddAccount(context.Background(), "qa", config.Account{
			AccountID: "333333333333",
			Region:    "us-east-1",
		}))
		info, err := os.Lstat(link)
		require.NoError(t, err)
		require.NotZero(t, info.Mode()&os.ModeSymlink)
		cfg, err := config.Load(target)
		require.NoError(t, err)
		require.Contains(t, cfg.Accounts, "qa")
	})
}
