package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPromptSession(t *testing.T) {
	t.Run("non tty is rejected", func(t *testing.T) {
		p := newPromptSession(strings.NewReader(""), &strings.Builder{}, false)
		require.Error(t, p.requireTTY())
	})

	t.Run("text retries validation", func(t *testing.T) {
		var out strings.Builder
		p := newPromptSession(strings.NewReader("bad\ngood\n"), &out, true)
		got, cancelled, err := p.text("Alias", false, func(value string) error {
			if value != "good" {
				return errors.New("not good")
			}
			return nil
		})
		require.NoError(t, err)
		require.False(t, cancelled)
		require.Equal(t, "good", got)
		require.Contains(t, out.String(), "not good")
	})

	t.Run("selection retains supplied order and marks active", func(t *testing.T) {
		var out strings.Builder
		p := newPromptSession(strings.NewReader("9\n2\n"), &out, true)
		got, cancelled, err := p.selectOne("Region", []string{"z-region", "a-region"}, "a-region")
		require.NoError(t, err)
		require.False(t, cancelled)
		require.Equal(t, "a-region", got)
		require.Less(t, strings.Index(out.String(), "z-region"), strings.Index(out.String(), "a-region"))
		require.Contains(t, out.String(), "active")
		require.Contains(t, out.String(), "invalid selection")
	})

	tests := []struct {
		name      string
		input     string
		confirmed bool
		cancelled bool
	}{
		{name: "yes short", input: "Y\n", confirmed: true},
		{name: "yes long", input: "yes\n", confirmed: true},
		{name: "no", input: "n\n", cancelled: true},
		{name: "blank", input: "\n", cancelled: true},
		{name: "eof", input: "", cancelled: true},
		{name: "ctrl c byte", input: "\x03\n", cancelled: true},
	}
	for _, tt := range tests {
		t.Run("confirm "+tt.name, func(t *testing.T) {
			p := newPromptSession(strings.NewReader(tt.input), &strings.Builder{}, true)
			confirmed, cancelled, err := p.confirm()
			require.NoError(t, err)
			require.Equal(t, tt.confirmed, confirmed)
			require.Equal(t, tt.cancelled, cancelled)
		})
	}
}
