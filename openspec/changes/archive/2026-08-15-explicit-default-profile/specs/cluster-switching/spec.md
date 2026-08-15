## MODIFIED Requirements

### Requirement: Default kubeconfig is not a switch target
The system SHALL NOT merge into the default kubeconfig at `~/.kube/config` during `opsx kube`. `~/.kube/config` has a single `current-context`, so writing it during switches creates latest-wins behavior across terminals and cannot provide multi-terminal isolation.

Current-terminal cluster switching MUST happen through shell integration that exports `KUBECONFIG` to the generated per-(cluster,mode) file. The generated file still carries `--profile <alias.mode>` in its exec block so `kubectl` authenticates as the selected account.

Legacy residue from older opsx versions that merged contexts into `~/.kube/config` SHALL be left untouched: opsx neither writes nor removes contexts there, and `opsx logout` SHALL NOT clean `~/.kube/config`. This is a deliberate asymmetry with the `[default]` profile compatibility cleanup, because editing structured kubeconfig YAML is out of scope.

#### Scenario: kube writes only the per-terminal kubeconfig
- **WHEN** `opsx kube dev-syd` runs
- **THEN** the cluster is written to the per-(cluster,mode) kubeconfig
- **AND** `~/.kube/config` is not written or merged

#### Scenario: legacy merged contexts are preserved, not cleaned
- **WHEN** `~/.kube/config` contains contexts merged by an older opsx version and any current command (including `opsx kube` and `opsx logout`) runs
- **THEN** `~/.kube/config` is not modified by opsx
- **AND** the legacy contexts remain for the user to clean up manually
