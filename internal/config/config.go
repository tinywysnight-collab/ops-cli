// Package config loads and validates the opsx YAML configuration and derives
// role ARNs from it. Every account ID and role name is config-driven; no ARN
// component is ever hardcoded (see CitizenRoleARN / MasterRoleARN).
package config

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// aliasPattern is the strict charset allowed for account/cluster aliases. It is
// enforced at load so a hostile or mistyped alias can never reach an
// eval'd `export` line (the aliases become profile/KUBECONFIG values).
var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

var accountIDPattern = regexp.MustCompile(`^[0-9]{12}$`)

// supportedModes are the modes every role map must define.
var supportedModes = []string{ModeAdmin, ModeOpr}

// Mode tokens are the single source of truth for spelling (see architecture).
const (
	ModeAdmin = "admin"
	ModeOpr   = "opr"
)

// Account is a citizen account entry, keyed by its CLI alias.
type Account struct {
	AccountID   string `yaml:"account_id"`
	Description string `yaml:"description"`
	// Region is the optional STS/home region for this account's citizen
	// AssumeRole. An account may run EKS in several regions, so this is distinct
	// from each cluster's own Region. Falls back to auth.region / env when empty.
	Region string `yaml:"region"`
}

// Cluster is an EKS cluster entry, keyed by its alias.
type Cluster struct {
	Account string `yaml:"account"` // references an accounts entry
	Region  string `yaml:"region"`
	Name    string `yaml:"name"` // real EKS cluster name for update-kubeconfig
}

// Entra holds the non-secret identifiers used by the Entra HTTP flow. The
// password is never stored here.
type Entra struct {
	AppID      string `yaml:"app_id"`
	Username   string `yaml:"username"`
	Debug      bool   `yaml:"debug"`
	BaseURL    string `yaml:"base_url"`
	MSLoginURL string `yaml:"ms_login_url"`
	MyAppsURL  string `yaml:"myapps_url"`
}

// Auth configures the master account, SAML provider, and the configurable role
// names used to compose ARNs.
type Auth struct {
	MasterAccountID string `yaml:"master_account_id"`
	SAMLProviderARN string `yaml:"saml_provider_arn"`
	// Region is the region used for the master AssumeRoleWithSAML call and the
	// default for citizen assumes. Falls back to AWS_REGION/AWS_DEFAULT_REGION.
	Region       string            `yaml:"region"`
	Entra        Entra             `yaml:"entra"`
	MasterRoles  map[string]string `yaml:"master_roles"`  // mode -> role name
	CitizenRoles map[string]string `yaml:"citizen_roles"` // mode -> role name
}

// Config is the full decoded config.yaml.
type Config struct {
	Regions  []string           `yaml:"regions"`
	Accounts map[string]Account `yaml:"accounts"`
	Clusters map[string]Cluster `yaml:"clusters"`
	Auth     Auth               `yaml:"auth"`
}

// ErrConfigNotFound is returned (wrapped) when the config file is absent.
var ErrConfigNotFound = errors.New("config file not found")

// NormalizeMode validates and canonicalizes a mode token.
func NormalizeMode(s string) (string, error) {
	switch s {
	case ModeAdmin, ModeOpr:
		return s, nil
	default:
		return "", fmt.Errorf("invalid mode %q: must be %q or %q", s, ModeAdmin, ModeOpr)
	}
}

// ValidateAlias enforces the shell-safe account/cluster alias syntax.
func ValidateAlias(alias string) error {
	if !aliasPattern.MatchString(alias) {
		return fmt.Errorf("alias %q contains disallowed characters (allowed: letters, digits, '.', '_', '-')", alias)
	}
	return nil
}

// ValidateAccountID enforces the AWS 12-digit account ID shape.
func ValidateAccountID(accountID string) error {
	if !accountIDPattern.MatchString(accountID) {
		return fmt.Errorf("account_id must be exactly 12 digits")
	}
	return nil
}

// Load reads and decodes config.yaml from path. A missing file yields a clear
// error (wrapping ErrConfigNotFound) naming the expected path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: expected at %s — create it to continue", ErrConfigNotFound, path)
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &c, nil
}

// Validate performs full structural validation up front, failing on the first
// offending entry with a clear, name-bearing message. It covers: alias charset,
// non-empty account IDs (and no duplicates), non-empty cluster region/name,
// cluster→account referential integrity, required auth fields, and mode-keyed
// role maps for every supported mode.
func (c *Config) Validate() error {
	if err := c.validateRegions(); err != nil {
		return err
	}
	if err := c.validateAccounts(); err != nil {
		return err
	}
	if err := c.validateClusters(); err != nil {
		return err
	}
	return c.validateAuth()
}

func (c *Config) validateRegions() error {
	if len(c.Regions) == 0 {
		return fmt.Errorf("regions must contain at least one allowed region")
	}
	seen := make(map[string]struct{}, len(c.Regions))
	for _, region := range c.Regions {
		if containsWhitespace(region) {
			return fmt.Errorf("regions entry %q must not be blank or contain whitespace", region)
		}
		if _, ok := seen[region]; ok {
			return fmt.Errorf("regions contains duplicate region %q", region)
		}
		seen[region] = struct{}{}
	}
	return nil
}

func (c *Config) regionAllowed(region string) bool {
	for _, allowed := range c.Regions {
		if region == allowed {
			return true
		}
	}
	return false
}

// IsRegionAllowed reports whether region is present in the ordered department
// allowlist.
func (c *Config) IsRegionAllowed(region string) bool {
	return c.regionAllowed(region)
}

func (c *Config) validateAccounts() error {
	seenID := map[string]string{} // account_id -> first alias
	for _, alias := range sortedKeys(c.Accounts) {
		if !aliasPattern.MatchString(alias) {
			return fmt.Errorf("account alias %q contains disallowed characters (allowed: letters, digits, '.', '_', '-')", alias)
		}
		acct := c.Accounts[alias]
		if acct.AccountID == "" {
			return fmt.Errorf("account %q: missing account_id", alias)
		}
		if !accountIDPattern.MatchString(acct.AccountID) {
			return fmt.Errorf("account %q: account_id must be exactly 12 digits", alias)
		}
		if acct.Region != "" && containsWhitespace(acct.Region) {
			return fmt.Errorf("account %q: region must not be blank or contain whitespace", alias)
		}
		if acct.Region == "" {
			return fmt.Errorf("account %q: missing region", alias)
		}
		if !c.regionAllowed(acct.Region) {
			return fmt.Errorf("account %q: region %q is not in the regions allowlist", alias, acct.Region)
		}
		if first, dup := seenID[acct.AccountID]; dup {
			return fmt.Errorf("accounts %q and %q share the same account_id %q", first, alias, acct.AccountID)
		}
		seenID[acct.AccountID] = alias
	}
	return nil
}

func (c *Config) validateClusters() error {
	for _, alias := range sortedKeys(c.Clusters) {
		if !aliasPattern.MatchString(alias) {
			return fmt.Errorf("cluster alias %q contains disallowed characters (allowed: letters, digits, '.', '_', '-')", alias)
		}
		cl := c.Clusters[alias]
		if cl.Account == "" {
			return fmt.Errorf("cluster %q: missing 'account' reference", alias)
		}
		if _, ok := c.Accounts[cl.Account]; !ok {
			return fmt.Errorf("cluster %q references unknown account alias %q", alias, cl.Account)
		}
		if strings.TrimSpace(cl.Region) == "" {
			return fmt.Errorf("cluster %q: missing region", alias)
		}
		if strings.ContainsFunc(cl.Region, unicode.IsSpace) {
			return fmt.Errorf("cluster %q: region must not contain whitespace", alias)
		}
		if cl.Name == "" {
			return fmt.Errorf("cluster %q: missing name", alias)
		}
		if !c.regionAllowed(cl.Region) {
			return fmt.Errorf("cluster %q: region %q is not in the regions allowlist", alias, cl.Region)
		}
	}
	return nil
}

func (c *Config) validateAuth() error {
	if c.Auth.MasterAccountID == "" {
		return fmt.Errorf("auth.master_account_id is not set")
	}
	if !accountIDPattern.MatchString(c.Auth.MasterAccountID) {
		return fmt.Errorf("auth.master_account_id must be exactly 12 digits")
	}
	if c.Auth.SAMLProviderARN == "" {
		return fmt.Errorf("auth.saml_provider_arn is not set")
	}
	if c.Auth.Region != "" && containsWhitespace(c.Auth.Region) {
		return fmt.Errorf("auth.region must not be blank or contain whitespace")
	}
	if c.Auth.Region != "" && !c.regionAllowed(c.Auth.Region) {
		return fmt.Errorf("auth.region %q is not in the regions allowlist", c.Auth.Region)
	}
	for _, mode := range supportedModes {
		if strings.TrimSpace(c.Auth.MasterRoles[mode]) == "" {
			return fmt.Errorf("auth.master_roles is missing an entry for mode %q", mode)
		}
		if strings.TrimSpace(c.Auth.CitizenRoles[mode]) == "" {
			return fmt.Errorf("auth.citizen_roles is missing an entry for mode %q", mode)
		}
	}
	return validateEntra(c.Auth.Entra)
}

func validateEntra(entra Entra) error {
	if entra.AppID != "" && strings.TrimSpace(entra.AppID) == "" {
		return fmt.Errorf("auth.entra.app_id must not be blank")
	}
	username := strings.TrimSpace(entra.Username)
	if entra.Username != "" && username == "" {
		return fmt.Errorf("auth.entra.username must not be blank")
	}
	if err := validateOptionalHTTPURL("base_url", entra.BaseURL); err != nil {
		return err
	}
	if err := validateOptionalHTTPURL("ms_login_url", entra.MSLoginURL); err != nil {
		return err
	}
	if err := validateOptionalHTTPURL("myapps_url", entra.MyAppsURL); err != nil {
		return err
	}
	return nil
}

func validateOptionalHTTPURL(field, raw string) error {
	if raw == "" {
		return nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.ContainsFunc(trimmed, unicode.IsSpace) {
		return fmt.Errorf("auth.entra.%s must be an absolute http(s) URL without whitespace", field)
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("auth.entra.%s must be an absolute http(s) URL", field)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("auth.entra.%s must use http or https", field)
	}
	return nil
}

// containsWhitespace reports whether s is blank after trimming or contains any
// interior whitespace — both are invalid for an AWS region token.
func containsWhitespace(s string) bool {
	return strings.TrimSpace(s) == "" || strings.ContainsFunc(s, unicode.IsSpace)
}

// regionFromEnv returns AWS_REGION, then AWS_DEFAULT_REGION, then "".
func regionFromEnv() string {
	if r := strings.TrimSpace(os.Getenv("AWS_REGION")); r != "" {
		return r
	}
	return strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION"))
}

// ResolveMasterRegion resolves the region for the master AssumeRoleWithSAML
// call: auth.region → AWS_REGION/AWS_DEFAULT_REGION. It fails with a clear,
// name-bearing error rather than letting the SDK surface its opaque
// "an AWS region is required" message.
func (c *Config) ResolveMasterRegion() (string, error) {
	if r := strings.TrimSpace(c.Auth.Region); r != "" {
		return r, nil
	}
	if r := regionFromEnv(); r != "" {
		return r, nil
	}
	return "", fmt.Errorf("no AWS region for the master STS call: set auth.region in config (or AWS_REGION/AWS_DEFAULT_REGION)")
}

// ResolveCitizenRegion resolves the required region for an account's citizen
// AssumeRole.
func (c *Config) ResolveCitizenRegion(alias string) (string, error) {
	if acct, ok := c.Accounts[alias]; ok {
		if r := strings.TrimSpace(acct.Region); r != "" {
			return r, nil
		}
	}
	return "", fmt.Errorf("no AWS region for account %q: set accounts.%s.region in config", alias, alias)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
