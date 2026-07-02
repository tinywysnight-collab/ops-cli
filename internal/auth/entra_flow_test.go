package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

func TestFetchAssertionRunsEntraADFSMFAFlow(t *testing.T) {
	t.Setenv(EnvSAMLAssertion, "")
	t.Setenv(EnvSAMLAssertionFile, "")
	t.Setenv(EnvPassword, "s3cret")

	var adfsPassword string
	var sawBeginAuth bool
	var sawEndAuth bool
	var sawPostMFA bool

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/applications/redirecttofederatedapplication.aspx":
			require.Equal(t, "app-123", r.URL.Query().Get("applicationId"))
			require.Empty(t, r.URL.Query().Get("appliationId"))
			writeConfig(t, w, map[string]any{
				"urlGetCredentialType": srv.URL + "/credtype",
				"correlationId":        "corr",
				"sessionId":            "session",
				"apiCanary":            "api-canary",
				"hpgact":               "act",
				"hpgid":                "id",
				"sCtx":                 "bootstrap-ctx",
				"sFT":                  "bootstrap-flow",
				"canary":               "bootstrap-canary",
			})
		case "/credtype":
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Equal(t, "operator@example.com", payload["username"])
			writeJSON(t, w, map[string]any{
				"Credentials": map[string]any{
					"FederationRedirectUrl": srv.URL + "/adfs?ctx=1",
				},
			})
		case "/adfs":
			require.NoError(t, r.ParseForm())
			require.Equal(t, "operator@example.com", r.Form.Get("UserName"))
			adfsPassword = r.Form.Get("Password")
			_, err := w.Write([]byte(`<input name="wresult" Value="relay-one" />`))
			require.NoError(t, err)
		case "/mslogin":
			require.NoError(t, r.ParseForm())
			require.Equal(t, "FormsAuthentication", r.Form.Get("AuthMethod"))
			_, err := w.Write([]byte(`<input name="relay" Value="relay-two" />`))
			require.NoError(t, err)
		case "/myapps":
			require.NoError(t, r.ParseForm())
			require.Equal(t, "relay-two", r.Form.Get("relay"))
			writeConfig(t, w, map[string]any{
				"arrUserProofs":  []any{map[string]any{"authMethodId": "push"}},
				"sCtx":           "mfa-ctx",
				"sFT":            "mfa-flow",
				"sessionId":      "mfa-session",
				"urlBeginAuth":   srv.URL + "/begin",
				"urlEndAuth":     srv.URL + "/end",
				"urlPost":        srv.URL + "/post",
				"sPOST_Username": "operator@example.com",
			})
		case "/begin":
			sawBeginAuth = true
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Equal(t, "BeginAuth", payload["Method"])
			require.Equal(t, "push", payload["AuthMethodId"])
			writeJSON(t, w, map[string]any{
				"Entropy":   "42",
				"Ctx":       "ctx-after-begin",
				"FlowToken": "flow-after-begin",
				"SessionId": "session-after-begin",
			})
		case "/end":
			sawEndAuth = true
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Equal(t, "EndAuth", payload["Method"])
			require.Equal(t, "ctx-after-begin", payload["Ctx"])
			writeJSON(t, w, map[string]any{
				"Success":   true,
				"Retry":     false,
				"Ctx":       "ctx-approved",
				"FlowToken": "flow-approved",
				"SessionId": "session-approved",
			})
		case "/post":
			sawPostMFA = true
			require.NoError(t, r.ParseForm())
			require.Equal(t, "bootstrap-canary", r.Form.Get("canary"))
			require.Equal(t, "operator@example.com", r.Form.Get("login"))
			require.Equal(t, "push", r.Form.Get("mfaAuthMethod"))
			_, err := w.Write([]byte(`<input name="SAMLResponse" Value="ASSERTION-FROM-MFA" />`))
			require.NoError(t, err)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(srv.Close)

	provider := NewEntraSAMLProvider(config.Auth{
		Entra: config.Entra{
			Username:   "operator@example.com",
			AppID:      "app-123",
			Debug:      true,
			BaseURL:    srv.URL + "/",
			MSLoginURL: srv.URL + "/mslogin",
			MyAppsURL:  srv.URL + "/myapps",
		},
	}).
		WithHTTPClient(srv.Client()).
		WithPollInterval(time.Nanosecond)
	var stderr bytes.Buffer
	provider.Stderr = &stderr

	role, err := MasterRoleFromMode("admin")
	require.NoError(t, err)
	got, err := provider.FetchAssertion(context.Background(), role)

	require.NoError(t, err)
	require.Equal(t, "ASSERTION-FROM-MFA", got)
	require.Equal(t, "s3cret", adfsPassword)
	require.True(t, sawBeginAuth, "BeginAuth must be called")
	require.True(t, sawEndAuth, "EndAuth must be called")
	require.True(t, sawPostMFA, "MFA post must be called")
	logs := stderr.String()
	require.Contains(t, logs, "42")
	require.Contains(t, logs, "opsx entra debug: starting assertion fetch role=admin")
	require.Contains(t, logs, "opsx entra debug: bootstrap request")
	require.Contains(t, logs, "opsx entra debug: federation redirect resolved")
	require.Contains(t, logs, "opsx entra debug: adfs form submitted hidden_fields=1")
	require.Contains(t, logs, "opsx entra debug: microsoft relay completed")
	require.Contains(t, logs, "opsx entra debug: mfa required")
	require.Contains(t, logs, "opsx entra debug: mfa approved")
	require.Contains(t, logs, "opsx entra debug: saml assertion extracted")
	require.NotContains(t, logs, "s3cret")
	require.NotContains(t, logs, "ASSERTION-FROM-MFA")
	require.NotContains(t, logs, "bootstrap-canary")
	require.NotContains(t, logs, "ctx-after-begin")
	require.NotContains(t, logs, "flow-after-begin")
	require.NotContains(t, logs, "session-after-begin")
	require.NotContains(t, logs, "app-123")
}

func TestNewEntraSAMLProviderUsesConfiguredURLs(t *testing.T) {
	provider := NewEntraSAMLProvider(config.Auth{
		Entra: config.Entra{
			BaseURL:    "https://login.example.com/",
			MSLoginURL: "https://ms.example.com/login",
			MyAppsURL:  "https://apps.example.com/myapps",
		},
	})

	require.Equal(t, "https://login.example.com", provider.baseURL)
	require.Equal(t, "https://ms.example.com/login", provider.msLoginURL)
	require.Equal(t, "https://apps.example.com/myapps", provider.myAppsURL)
}

func TestFetchAssertionRequiresTopLevelAppID(t *testing.T) {
	t.Setenv(EnvSAMLAssertion, "")
	t.Setenv(EnvSAMLAssertionFile, "")

	provider := NewEntraSAMLProvider(config.Auth{
		Entra: config.Entra{Username: "operator@example.com"},
	})

	role, err := MasterRoleFromMode("admin")
	require.NoError(t, err)
	_, err = provider.FetchAssertion(context.Background(), role)

	require.Error(t, err)
	require.Contains(t, err.Error(), "auth.entra.app_id")
}

func writeConfig(t *testing.T, w http.ResponseWriter, cfg map[string]any) {
	t.Helper()
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	_, err = w.Write([]byte("$Config=" + string(data) + ";"))
	require.NoError(t, err)
}

func writeJSON(t *testing.T, w http.ResponseWriter, body map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(body))
}
