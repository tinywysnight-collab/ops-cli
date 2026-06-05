package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

// EnvSAMLAssertion lets operators provide a pre-obtained SAMLResponse/assertion
// as an escape hatch when bypassing the interactive Entra+ADFS+MFA flow.
const EnvSAMLAssertion = "OPSX_SAML_ASSERTION"

// EnvSAMLAssertionFile points to a file containing a pre-obtained
// SAMLResponse/assertion. It is checked only when EnvSAMLAssertion is unset.
const EnvSAMLAssertionFile = "OPSX_SAML_ASSERTION_FILE"

var (
	ErrBadCredentials  = errors.New("invalid credentials")
	ErrMFADenied       = errors.New("mfa denied")
	ErrNoSAMLAssertion = errors.New("no SAML assertion in the response")
)

// EntraSAMLProvider is the only company-specific implementation of SAMLProvider.
// It drives the Entra+ADFS+MFA HTTP flow behind the auth seam; everything
// downstream in opsx remains testable against a fake provider.
type EntraSAMLProvider struct {
	Auth         config.Auth
	Client       *http.Client
	Stderr       io.Writer
	baseURL      string
	msLoginURL   string
	myAppsURL    string
	pollInterval time.Duration
}

const (
	defaultEntraBaseURL = "https://auth.entra.io"
	defaultMSLoginURL   = "https://auth.entra.io"
	defaultMyAppsURL    = "https://auth.entra.io"
)

// NewEntraSAMLProvider builds a provider whose HTTP client uses the default
// transport, so all calls honor the system proxy env vars (HTTP(S)_PROXY /
// NO_PROXY) via http.ProxyFromEnvironment. The proxy is never hardcoded.
func NewEntraSAMLProvider(auth config.Auth) *EntraSAMLProvider {
	return &EntraSAMLProvider{
		Auth:         auth,
		Client:       &http.Client{Transport: http.DefaultTransport},
		Stderr:       os.Stderr,
		baseURL:      configuredBaseURL(auth.Entra.BaseURL, defaultEntraBaseURL),
		msLoginURL:   configuredURL(auth.Entra.MSLoginURL, defaultMSLoginURL),
		myAppsURL:    configuredURL(auth.Entra.MyAppsURL, defaultMyAppsURL),
		pollInterval: time.Second,
	}
}

func configuredURL(raw, defaultURL string) string {
	if v := strings.TrimSpace(raw); v != "" {
		return v
	}
	return defaultURL
}

func configuredBaseURL(raw, defaultURL string) string {
	return strings.TrimRight(configuredURL(raw, defaultURL), "/")
}

func (p *EntraSAMLProvider) WithMSLoginURL(url string) *EntraSAMLProvider {
	p.msLoginURL = url
	return p
}

func (p *EntraSAMLProvider) WithPollInterval(interval time.Duration) *EntraSAMLProvider {
	p.pollInterval = interval
	return p
}
func (p *EntraSAMLProvider) WithMyAppsURL(url string) *EntraSAMLProvider {
	p.myAppsURL = url
	return p
}
func (p *EntraSAMLProvider) WithHTTPClient(client *http.Client) *EntraSAMLProvider {
	p.Client = client
	return p
}

func (p *EntraSAMLProvider) debugf(format string, args ...any) {
	if !p.Auth.Entra.Debug || p.Stderr == nil {
		return
	}
	fmt.Fprintf(p.Stderr, "opsx entra debug: "+format+"\n", args...)
}

func safeURLForLog(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid-url>"
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil
	return u.String()
}

func debugf(debugLog func(string, ...any), format string, args ...any) {
	if debugLog != nil {
		debugLog(format, args...)
	}
}

var reConfig = regexp.MustCompile(`(?s)\$Config=(\{.*?\});`)

func parseConfig(body string) (map[string]any, error) {
	m := reConfig.FindStringSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("$config not found in response body")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(m[1]), &out); err != nil {
		return nil, fmt.Errorf("error parsing $config: %w", err)
	}
	return out, nil
}

var reInput = regexp.MustCompile(`(?is)<input\b(?:[^>"']*(?:"[^"]*"|'[^']*')?)*\/?>`)
var reName = regexp.MustCompile(`(?i)\bname="([^"]*)"`)
var reValue = regexp.MustCompile(`(?i)\bvalue="([^"]*)"`)

func parseInputs(body string) map[string]string {
	out := map[string]string{}
	for _, tag := range reInput.FindAllString(body, -1) {
		nm := reName.FindStringSubmatch(tag)
		if nm == nil {
			continue
		}
		val := ""
		if vm := reValue.FindStringSubmatch(tag); vm != nil {
			for _, group := range vm[1:] {
				if group != "" {
					val = html.UnescapeString(group)
					break
				}
			}
		}
		out[nm[1]] = val
	}
	return out
}

func resolveEntraAppID(_ string, entra config.Entra) (string, error) {
	if appID := strings.TrimSpace(entra.AppID); appID != "" {
		return appID, nil
	}
	return "", fmt.Errorf("auth.entra.app_id is not set")
}

func strVal(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func buildHeaders(cfg map[string]any) map[string]string {
	return map[string]string{
		"client-request-id": strVal(cfg, "correlationId"),
		"hpgrequestid":      strVal(cfg, "sessionId"),
		"canary":            strVal(cfg, "apiCanary"),
		"hpgact":            strVal(cfg, "hpgact"),
		"hpgid":             strVal(cfg, "hpgid"),
	}
}

func authMethodIDFromCfg(cfg map[string]any) string {
	if proofs, ok := cfg["arrUserProofs"].([]any); ok && len(proofs) > 0 {
		if proof, ok := proofs[0].(map[string]any); ok {
			if id := strVal(proof, "authMethodId"); id != "" {
				return id
			}
		}
	}
	return strVal(cfg, "authMethodId")
}

func applyHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
}

func doJSON(ctx context.Context, client *http.Client, endpoint string, payload any, headers map[string]string, debugLog func(string, ...any)) (map[string]any, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("json marshal payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("doJSON new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	applyHeaders(req, headers)
	resp, err := client.Do(req)
	if err != nil {
		debugf(debugLog, "http POST url=%s error=%v", safeURLForLog(endpoint), err)
		return nil, fmt.Errorf("do http request: %w", err)
	}
	debugf(debugLog, "http POST url=%s status=%d", safeURLForLog(endpoint), resp.StatusCode)

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("do json decode response: %w", err)
	}
	if err := resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("close response body: %w", err)
	}
	return out, nil
}

func doForm(ctx context.Context, client *http.Client, endpoint string, values url.Values, headers map[string]string, debugLog func(string, ...any)) (string, error) {
	encoded := values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("doForm new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	applyHeaders(req, headers)
	resp, err := client.Do(req)
	if err != nil {
		debugf(debugLog, "http POST url=%s error=%v", safeURLForLog(endpoint), err)
		return "", fmt.Errorf("do http request: %w", err)
	}
	debugf(debugLog, "http POST url=%s status=%d", safeURLForLog(endpoint), resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return "", fmt.Errorf("do read response body: %w", err)
	}
	if err := resp.Body.Close(); err != nil {
		return "", fmt.Errorf("close response body: %w", err)
	}
	return string(b), nil
}

func fetchFederationURL(ctx context.Context, client *http.Client, _ string, username string, bootstrapCfg map[string]any, debugLog func(string, ...any)) (formSubmitURL string, cfg map[string]any, err error) {
	credTypeURL := strVal(bootstrapCfg, "urlGetCredentialType")
	if credTypeURL == "" {
		return "", nil, fmt.Errorf("fetchFederationURL: urlGetCredentialType missing from bootstrapCfg")
	}
	headers := buildHeaders(bootstrapCfg)
	payload := map[string]any{
		"username":             username,
		"isOtherIdpSupported":  true,
		"checkPhones":          true,
		"isRemoteNGCSupported": true,
		"isCookieBannerShown":  false,
		"isFidoSupported":      false,
		"originalRequest":      strVal(bootstrapCfg, "sCtx"),
		"flowToken":            strVal(bootstrapCfg, "sFT"),
	}
	result, err := doJSON(ctx, client, credTypeURL, payload, headers, debugLog)
	if err != nil {
		return "", nil, fmt.Errorf("fetchFederationURL: %w", err)
	}
	creds, _ := result["Credentials"].(map[string]any)
	if creds == nil {
		return "", nil, fmt.Errorf("fetchFederationURL: credentials missing from response")
	}
	fedURL := strVal(creds, "FederationRedirectUrl")
	if fedURL == "" {
		return "", nil, fmt.Errorf("fetchFederationURL: federationRedirectUrl missing from response")
	}

	formSubmitURL = fedURL + "&RedirectToIdentityProvider=AD+Authority"
	return formSubmitURL, bootstrapCfg, nil
}

func adfsAuthenticate(ctx context.Context, client *http.Client, formSubmitURL, username string, password []byte, headers map[string]string, debugLog func(string, ...any)) (map[string]string, error) {
	values := url.Values{
		"UserName":   {username},
		"Password":   {string(password)},
		"AuthMethod": {"FormsAuthentication"},
	}
	for i := range password {
		password[i] = 0
	}
	body, err := doForm(ctx, client, formSubmitURL, values, headers, debugLog)
	if err != nil {
		return nil, err
	}
	if strings.Contains(body, "Incorrect user ID or password") {
		return nil, ErrBadCredentials
	}
	inputs := parseInputs(body)
	return inputs, nil
}

func microsoftRelay(ctx context.Context, client *http.Client, msLoginURL, myAppsURL string, inputs map[string]string, headers map[string]string, debugLog func(string, ...any)) (string, error) {
	v1 := url.Values{}
	for k, v := range inputs {
		v1.Set(k, v)
	}
	v1.Set("AuthMethod", "FormsAuthentication")
	body1, err := doForm(ctx, client, msLoginURL, v1, headers, debugLog)
	if err != nil {
		return "", fmt.Errorf("microsoftRelay step1: %w", err)
	}
	inputs2 := parseInputs(body1)

	merged := make(map[string]string, len(inputs)+len(inputs2))
	for k, v := range inputs {
		merged[k] = v
	}
	for k, v := range inputs2 {
		merged[k] = v
	}
	v2 := url.Values{}
	for k, val := range merged {
		v2.Set(k, val)
	}
	body2, err := doForm(ctx, client, myAppsURL, v2, headers, debugLog)
	if err != nil {
		return "", fmt.Errorf("microsoftRelay step2: %w", err)
	}
	return body2, nil
}

func mfaPoll(ctx context.Context, client *http.Client, mfaCfg map[string]any, bootstrapCanary string, headers map[string]string, stderr io.Writer, pollInterval time.Duration, debugLog func(string, ...any)) (string, error) {
	authMethodID := authMethodIDFromCfg(mfaCfg)
	sCtx := strVal(mfaCfg, "sCtx")
	sFT := strVal(mfaCfg, "sFT")
	sessionID := strVal(mfaCfg, "sessionId")
	beginURL := strVal(mfaCfg, "urlBeginAuth")
	endURL := strVal(mfaCfg, "urlEndAuth")
	postURL := strVal(mfaCfg, "urlPost")

	debugf(debugLog, "mfa begin auth endpoint=%s", safeURLForLog(beginURL))
	beginResp, err := doJSON(ctx, client, beginURL, map[string]any{
		"AuthMethodId": authMethodID,
		"Method":       "BeginAuth",
		"ctx":          sCtx,
		"flowToken":    sFT,
	}, headers, debugLog)

	if err != nil {
		return "", fmt.Errorf("mfaPoll BeginAuth: %w", err)
	}
	entropy := strVal(beginResp, "Entropy")
	fmt.Fprintf(stderr, "Open your Authenticator app and enter the code: %s\n", entropy)
	debugf(debugLog, "mfa challenge received entropy_present=%t", entropy != "")
	if v := strVal(beginResp, "Ctx"); v != "" {
		sCtx = v
	}
	if v := strVal(beginResp, "FlowToken"); v != "" {
		sFT = v
	}
	if v := strVal(beginResp, "SessionId"); v != "" {
		sessionID = v
	}
	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(pollInterval):
		}
		attempt++
		endResp, err := doJSON(ctx, client, endURL, map[string]any{
			"Method":       "EndAuth",
			"AuthMethodId": authMethodID,
			"Ctx":          sCtx,
			"FlowToken":    sFT,
			"SessionId":    sessionID,
		}, headers, debugLog)
		if err != nil {
			return "", fmt.Errorf("mfaPoll EndAuth: %w", err)
		}
		retry, _ := endResp["Retry"].(bool)
		success, _ := endResp["Success"].(bool)
		if retry {
			debugf(debugLog, "mfa poll attempt=%d retry=true", attempt)
			if v := strVal(endResp, "Ctx"); v != "" {
				sCtx = v
			}
			if v := strVal(endResp, "FlowToken"); v != "" {
				sFT = v
			}
			continue
		}
		if !success {
			msg := strVal(endResp, "Message")
			result := strVal(endResp, "ResultValue")
			debugf(debugLog, "mfa denied attempt=%d", attempt)
			return "", fmt.Errorf("%w: %s %s", ErrMFADenied, msg, result)
		}
		debugf(debugLog, "mfa approved attempt=%d", attempt)
		if v := strVal(endResp, "Ctx"); v != "" {
			sCtx = v
		}
		if v := strVal(endResp, "FlowToken"); v != "" {
			sFT = v
		}
		if v := strVal(endResp, "SessionId"); v != "" {
			sessionID = v
		}

		debugf(debugLog, "mfa posting final form endpoint=%s", safeURLForLog(postURL))
		postValues := url.Values{
			"canary":             {bootstrapCanary},
			"hideSmsInMfaProofs": {"false"},
			"hpgrequestid":       {sessionID},
			"flowToken":          {sFT},
			"request":            {sCtx},
			"login":              {strVal(mfaCfg, "sPOST_Username")},
			"mfaAuthMethod":      {authMethodID},
		}
		samlHTML, err := doForm(ctx, client, postURL, postValues, headers, debugLog)
		if err != nil {
			return "", fmt.Errorf("mfaPoll Login: %w", err)
		}
		debugf(debugLog, "mfa final form received")
		return samlHTML, nil
	}
}

func extractSAMLResponse(body string) (string, error) {
	inputs := parseInputs(body)
	if v, ok := inputs["SAMLResponse"]; ok && v != "" {
		return v, nil
	}
	return "", ErrNoSAMLAssertion
}

// FetchAssertion performs Entra auth with interactive MFA and returns a SAML
// assertion, unless OPSX_SAML_ASSERTION(_FILE) supplies one directly:
//   - password via ReadPassword (no echo, OPSX_PASSWORD fallback), zeroized after use;
//   - MFA number/prompt printed to p.Stderr; polling honors ctx timeout/cancellation;
//   - all HTTP via p.Client (system proxy); the assertion is never logged.
func (p *EntraSAMLProvider) FetchAssertion(ctx context.Context, role MasterRole) (string, error) {
	if assertion := strings.TrimSpace(os.Getenv(EnvSAMLAssertion)); assertion != "" {
		p.debugf("using assertion escape hatch source=%s", EnvSAMLAssertion)
		return assertion, nil
	}
	if path := strings.TrimSpace(os.Getenv(EnvSAMLAssertionFile)); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if assertion := strings.TrimSpace(string(data)); assertion != "" {
			p.debugf("using assertion escape hatch source=%s", EnvSAMLAssertionFile)
			return assertion, nil
		}
		return "", errors.New("SAML assertion file is empty")
	}
	p.debugf("starting assertion fetch role=%s", role)
	username := p.Auth.Entra.Username
	if strings.TrimSpace(username) == "" {
		return "", fmt.Errorf("auth.entra.username is not set")
	}
	appID, err := resolveEntraAppID(username, p.Auth.Entra)
	if err != nil {
		return "", err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", fmt.Errorf("cookie jar: %w", err)
	}
	p.debugf("session client initialized")
	sessionClient := &http.Client{
		Transport:     p.Client.Transport,
		CheckRedirect: p.Client.CheckRedirect,
		Timeout:       p.Client.Timeout,
		Jar:           jar,
	}
	password, err := ReadPassword(p.Stderr, "Password: ")
	if err != nil {
		return "", err
	}
	defer Zeroize(password)
	bootstrapURL := fmt.Sprintf("%s/applications/redirecttofederatedapplication.aspx?Operation=LinkedSignIn&applicationId=%s", p.baseURL, appID)
	p.debugf("bootstrap request method=GET url=%s", safeURLForLog(bootstrapURL))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bootstrapURL, nil)
	if err != nil {
		return "", fmt.Errorf("bootstrap request: %w", err)
	}
	resp, err := sessionClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("bootstrap: %w", err)
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return "", fmt.Errorf("read bootstrap response body: %w", err)
	}
	if err := resp.Body.Close(); err != nil {
		return "", fmt.Errorf("close bootstrap response body: %w", err)
	}
	p.debugf("bootstrap response received status=%d bytes=%d", resp.StatusCode, len(bodyBytes))
	bootstrapCfg, err := parseConfig(string(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("parse bootstrap response body: %w", err)
	}
	p.debugf("bootstrap config parsed")

	formSubmitURL, stepCfg, err := fetchFederationURL(ctx, sessionClient, appID, username, bootstrapCfg, p.debugf)
	if err != nil {
		return "", err
	}
	p.debugf("federation redirect resolved url=%s", safeURLForLog(formSubmitURL))
	headers := buildHeaders(stepCfg)
	pwCopy := make([]byte, len(password))
	copy(pwCopy, password)
	inputs, err := adfsAuthenticate(ctx, sessionClient, formSubmitURL, username, pwCopy, headers, p.debugf)
	if err != nil {
		return "", err
	}
	p.debugf("adfs form submitted hidden_fields=%d", len(inputs))
	relayBody, err := microsoftRelay(ctx, sessionClient, p.msLoginURL, p.myAppsURL, inputs, headers, p.debugf)
	if err != nil {
		return "", err
	}
	p.debugf("microsoft relay completed response_bytes=%d", len(relayBody))
	mfaCfg, mfaErr := parseConfig(relayBody)
	if mfaErr == nil && strVal(mfaCfg, "urlBeginAuth") != "" {
		p.debugf("mfa required")
		relayBody, err = mfaPoll(ctx, sessionClient, mfaCfg, strVal(bootstrapCfg, "canary"), headers, p.Stderr, p.pollInterval, p.debugf)
		if err != nil {
			return "", err
		}
	} else {
		p.debugf("mfa not required")
	}
	assertion, err := extractSAMLResponse(relayBody)
	if err != nil {
		return "", err
	}
	p.debugf("saml assertion extracted")
	return assertion, nil
}
