package cli

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/tinywysnight-collab/ops-cli/internal/auth"
	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/creds"
	"github.com/tinywysnight-collab/ops-cli/internal/kube"
)

// These package-level seams default to the real implementations and are
// overridden by integration tests to exercise the full command wiring with a
// fake SAMLProvider and fake STS/exec — no real AWS or external tools.
var (
	// samlProviderFactory builds the auth seam used by `opsx login`.
	samlProviderFactory = func(a config.Auth) auth.SAMLProvider { return auth.NewEntraSAMLProvider(a) }

	// masterAssumeFactory builds the STS client used to assume the master role.
	// The region is resolved from config (auth.region) so the SAML assume never
	// depends on AWS_REGION being present in the environment.
	masterAssumeFactory = func(ctx context.Context, region string) (auth.AssumeWithSAMLAPI, error) {
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx, // default transport → system proxy
			awsconfig.WithRegion(region),
		)
		if err != nil {
			return nil, fmt.Errorf("load AWS config: %w", err)
		}
		return sts.NewFromConfig(awsCfg), nil
	}

	// citizenAssumer performs the citizen AssumeRole for `opsx use`/`opsx kube`.
	citizenAssumer creds.Assumer = auth.STSAssume

	// kubeServiceFactory builds the EKS update-kubeconfig runner.
	kubeServiceFactory = func() *kube.Service { return kube.NewService() }
)
