package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/tinywysnight-collab/ops-cli/internal/creds"
)

// AssumeWithSAMLAPI is the subset of the STS client used to exchange a SAML
// assertion for master credentials. It is an interface so tests inject a fake.
type AssumeWithSAMLAPI interface {
	AssumeRoleWithSAML(ctx context.Context, in *sts.AssumeRoleWithSAMLInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleWithSAMLOutput, error)
}

// assumeMaster performs AssumeRoleWithSAML and returns the master credentials
// and their expiration.
func assumeMaster(ctx context.Context, api AssumeWithSAMLAPI, principalARN, roleARN, assertion string) (creds.Credentials, time.Time, error) {
	out, err := api.AssumeRoleWithSAML(ctx, &sts.AssumeRoleWithSAMLInput{
		PrincipalArn:  aws.String(principalARN),
		RoleArn:       aws.String(roleARN),
		SAMLAssertion: aws.String(assertion),
	})
	if err != nil {
		return creds.Credentials{}, time.Time{}, fmt.Errorf("assume master role %s with SAML: %w", roleARN, err)
	}
	if out.Credentials == nil {
		return creds.Credentials{}, time.Time{}, fmt.Errorf("assume master role %s: STS returned no credentials", roleARN)
	}
	c := out.Credentials
	stsCreds := creds.Credentials{
		AccessKeyID:     aws.ToString(c.AccessKeyId),
		SecretAccessKey: aws.ToString(c.SecretAccessKey),
		SessionToken:    aws.ToString(c.SessionToken),
	}
	expiry := aws.ToTime(c.Expiration)
	if err := ValidateSTSResult("assume master role "+roleARN, stsCreds, expiry); err != nil {
		return creds.Credentials{}, time.Time{}, err
	}
	return stsCreds, expiry, nil
}

// ValidateSTSResult rejects an STS response that is structurally unusable:
// empty token/key fields or a missing/non-future expiry must never be written
// as a cached session. label names the call site in the error.
func ValidateSTSResult(label string, c creds.Credentials, expiry time.Time) error {
	if !c.HasSessionToken() {
		return fmt.Errorf("%s: STS returned incomplete credentials (empty access key, secret key, or session token)", label)
	}
	// A zero expiry means STS omitted Expiration; the write would poison the
	// cache with an entry that never expires. A merely past expiry is left to
	// the normal IsExpired path so injected-clock tests stay deterministic.
	if expiry.IsZero() {
		return fmt.Errorf("%s: STS returned no expiry", label)
	}
	return nil
}
