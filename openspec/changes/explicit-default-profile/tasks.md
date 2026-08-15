## 1. Verify recorded behavior against shipped code (retroactive — no implementation tasks)

- [x] 1.1 Confirm `opsx use` writes only `[<alias>.<mode>]` and never `[default]` (TestUseDoesNotWriteDefaultProfile, TestUseDoesNotRewriteDefaultOnReuse)
- [x] 1.2 Confirm `opsx default <alias>` ensures the citizen profile (including cache reuse without a forced AssumeRole), then copies the credentials into `[default]` (TestIntegrationDefaultCommandWritesDefaultProfile)
- [x] 1.3 Confirm `opsx default` failure paths: unknown alias error, expired master surfaced as the re-login hint, incomplete profile refused (internal/cli/default.go)
- [x] 1.4 Confirm `opsx kube` writes only the per-(cluster,mode) kubeconfig and never touches `~/.kube/config` (TestIntegrationKubeDoesNotMergeDefaultKubeconfig)
- [x] 1.5 Confirm wrappers execute `default` directly without changing the invoking terminal's environment (wrapper dispatch lists cover only use/kube/mode/region)
- [x] 1.6 Confirm `opsx logout` clears `[default]` as compatibility cleanup but performs no kubeconfig cleanup (TestRunLogoutRemovesDefaultProfile)

## 2. Spec artifacts and validation

- [x] 2.1 Write the three deltas (account-switching, cluster-switching, shell-integration) as complete replacement requirements matching the current canonical text plus the new scenarios
- [x] 2.2 Run `openspec validate explicit-default-profile --strict` and `make spec`
