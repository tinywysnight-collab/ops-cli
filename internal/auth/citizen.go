package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/tinywysnight-collab/ops-cli/internal/creds"
)

// STSAssume is the real creds.Assumer: it builds an STS client authenticated
// with the master credentials, in the configured region, and performs
// AssumeRole for the citizen role. The default transport honors system proxy
// env vars; no proxy is hardcoded. The region is set explicitly so the call
// never depends on AWS_REGION being present in the environment.
func STSAssume(ctx context.Context, master creds.Credentials, roleARN, sessionName, region string) (creds.Credentials, time.Time, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(master.AccessKeyID, master.SecretAccessKey, master.SessionToken),
		),
	)
	if err != nil {
		return creds.Credentials{}, time.Time{}, fmt.Errorf("load AWS config for citizen assume: %w", err)
	}

	out, err := sts.NewFromConfig(awsCfg).AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleARN),
		RoleSessionName: aws.String(sessionName),
	})
	if err != nil {
		return creds.Credentials{}, time.Time{}, fmt.Errorf("assume role %s: %w", roleARN, err)
	}
	if out.Credentials == nil {
		return creds.Credentials{}, time.Time{}, fmt.Errorf("assume role %s: STS returned no credentials", roleARN)
	}
	c := out.Credentials
	stsCreds := creds.Credentials{
		AccessKeyID:     aws.ToString(c.AccessKeyId),
		SecretAccessKey: aws.ToString(c.SecretAccessKey),
		SessionToken:    aws.ToString(c.SessionToken),
	}
	expiry := aws.ToTime(c.Expiration)
	if err := ValidateSTSResult("assume role "+roleARN, stsCreds, expiry); err != nil {
		return creds.Credentials{}, time.Time{}, err
	}
	return stsCreds, expiry, nil
}
