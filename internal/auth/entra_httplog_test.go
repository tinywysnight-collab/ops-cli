package auth

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// bufLogger returns a debugLog func mirroring EntraSAMLProvider.debugf so the
// HTTP helpers can be unit-tested without a full provider.
func bufLogger(buf *bytes.Buffer) func(string, ...any) {
	return func(format string, args ...any) {
		fmt.Fprintf(buf, "opsx entra debug: "+format+"\n", args...)
	}
}

func TestDoFormLogsMethodAndStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway) // 502
		_, _ = w.Write([]byte("body"))
	}))
	t.Cleanup(srv.Close)

	var buf bytes.Buffer
	_, err := doForm(context.Background(), srv.Client(), srv.URL+"/relay?flowToken=secret",
		url.Values{"AuthMethod": {"FormsAuthentication"}}, nil, bufLogger(&buf))
	require.NoError(t, err)

	logs := buf.String()
	require.Contains(t, logs, "http POST")
	require.Contains(t, logs, "status=502")
	require.Contains(t, logs, "/relay")
	require.NotContains(t, logs, "flowToken=secret") // query string must be redacted
}

func TestDoJSONLogsMethodAndStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	var buf bytes.Buffer
	_, err := doJSON(context.Background(), srv.Client(), srv.URL+"/begin?ctx=secret",
		map[string]any{"Method": "BeginAuth"}, nil, bufLogger(&buf))
	require.NoError(t, err)

	logs := buf.String()
	require.Contains(t, logs, "http POST")
	require.Contains(t, logs, "status=200")
	require.Contains(t, logs, "/begin")
	require.NotContains(t, logs, "ctx=secret")
}

// A nil logger must be a no-op (the helpers are also called outside the debug path).
func TestHTTPHelpersTolerateNilLogger(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	_, err := doJSON(context.Background(), srv.Client(), srv.URL, map[string]any{"k": "v"}, nil, nil)
	require.NoError(t, err)
	_, err = doForm(context.Background(), srv.Client(), srv.URL, url.Values{"a": {"b"}}, nil, nil)
	require.NoError(t, err)
}
