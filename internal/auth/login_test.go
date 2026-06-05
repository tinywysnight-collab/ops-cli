package auth_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/auth"
	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/creds"
	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

// fakeProvider records calls and returns a canned assertion.
type fakeProvider struct {
	assertion string
	calls     []auth.MasterRole
}

func (f *fakeProvider) FetchAssertion(_ context.Context, role auth.MasterRole) (string, error) {
	f.calls = append(f.calls, role)
	return f.assertion, nil
}

// fakeSTS returns deterministic credentials expiring 1h after `now`.
type fakeSTS struct {
	now      time.Time
	gotInput *ststypes.AssumeRoleWithSAMLInput
}

func (f *fakeSTS) AssumeRoleWithSAML(_ context.Context, in *ststypes.AssumeRoleWithSAMLInput, _ ...func(*ststypes.Options)) (*ststypes.AssumeRoleWithSAMLOutput, error) {
	f.gotInput = in
	return &ststypes.AssumeRoleWithSAMLOutput{
		Credentials: &types.Credentials{
			AccessKeyId:     aws.String("AKIA-" + aws.ToString(in.RoleArn)),
			SecretAccessKey: aws.String("secret"),
			SessionToken:    aws.String("token"),
			Expiration:      aws.Time(f.now.Add(time.Hour)),
		},
	}, nil
}

func testConfig() *config.Config {
	return &config.Config{
		Auth: config.Auth{
			MasterAccountID: "000000000000",
			SAMLProviderARN: "arn:aws:iam::000000000000:saml-provider/EntraID",
			MasterRoles:     map[string]string{"admin": "master_admin", "opr": "master_AWSOpr"},
			CitizenRoles:    map[string]string{"admin": "Admin", "opr": "AWSOpr"},
		},
	}
}

func newStores(t *testing.T) (*creds.Store, *state.Store) {
	t.Helper()
	dir := t.TempDir()
	lock := filepath.Join(dir, ".opsx.lock")
	return creds.NewStore(filepath.Join(dir, "credentials"), lock),
		state.NewStore(filepath.Join(dir, "state.json"), lock)
}

func TestLoginCachesMasterThroughSeam(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	prov := &fakeProvider{assertion: "ASSERTION"}
	sts := &fakeSTS{now: now}
	cs, ss := newStores(t)

	svc := &auth.LoginService{
		Cfg: testConfig(), Provider: prov, STS: sts, Creds: cs, State: ss,
		Now: func() time.Time { return now },
	}

	require.NoError(t, svc.Login(context.Background(), "admin"))

	// Auth went through the seam for the admin role.
	require.Equal(t, []auth.MasterRole{auth.RoleAdmin}, prov.calls)
	// AssumeRoleWithSAML used the assertion + composed master ARN + principal.
	require.Equal(t, "ASSERTION", aws.ToString(sts.gotInput.SAMLAssertion))
	require.Equal(t, "arn:aws:iam::000000000000:role/master_admin", aws.ToString(sts.gotInput.RoleArn))
	require.Equal(t, "arn:aws:iam::000000000000:saml-provider/EntraID", aws.ToString(sts.gotInput.PrincipalArn))

	// Master creds cached to [master_admin] with a 1h expiry recorded in state.
	got, ok, err := cs.Read("master_admin")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "AKIA-arn:aws:iam::000000000000:role/master_admin", got.AccessKeyID)

	entry, ok, err := ss.Get("master_admin")
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, now.Add(time.Hour).Equal(entry.Expiry))
}

func TestBothMasterRolesCoexist(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newStores(t)
	cfg := testConfig()

	for _, mode := range []string{"admin", "opr"} {
		svc := &auth.LoginService{
			Cfg: cfg, Provider: &fakeProvider{assertion: "A"}, STS: &fakeSTS{now: now},
			Creds: cs, State: ss, Now: func() time.Time { return now },
		}
		require.NoError(t, svc.Login(context.Background(), mode))
	}

	_, ok, _ := cs.Read("master_admin")
	require.True(t, ok, "master_admin must remain after opr login")
	_, ok, _ = cs.Read("master_awsopr")
	require.True(t, ok, "master_awsopr must be present")
}
