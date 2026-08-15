## ADDED Requirements

### Requirement: Atomic per-cluster kubeconfig publication
The system SHALL generate each per-(cluster,mode) kubeconfig by having `aws eks update-kubeconfig` write a staging file in the target directory and then publishing it with fsync + `0600` + atomic rename, while holding the shared advisory lock. Concurrent switches to the same cluster SHALL serialize, and a cancelled or failed generation SHALL leave any existing kubeconfig untouched with no staging files left behind.

#### Scenario: Cancelled switch keeps the previous kubeconfig
- **WHEN** `opsx kube <alias>` is cancelled while the AWS CLI is mid-write
- **THEN** the existing per-(cluster,mode) kubeconfig is unchanged and complete
- **AND** no staging file remains

#### Scenario: Concurrent same-cluster switches stay intact
- **WHEN** two terminals run `opsx kube <same-alias>` simultaneously
- **THEN** the resulting kubeconfig is exactly one complete generation
- **AND** no staging file remains
