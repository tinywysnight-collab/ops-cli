# terminal-region-switching Specification

## Purpose
Interactive selection of a department-approved region that exports only the current terminal's AWS_REGION/AWS_DEFAULT_REGION, without persisting a runtime preference.

## Requirements
### Requirement: Interactively switch the current terminal region
The system SHALL expose TTY-only `opsx region`, presenting the top-level `regions` values as a numbered menu in configuration order and marking the current `AWS_REGION` when it is an allowed value. Selecting a region through shell integration SHALL set both `AWS_REGION` and `AWS_DEFAULT_REGION` in only the invoking terminal.

#### Scenario: Allowed region is selected
- **WHEN** the operator runs `opsx region` in a terminal, selects an allowed region, and the shell wrapper applies the emitted assignments
- **THEN** that terminal's `AWS_REGION` and `AWS_DEFAULT_REGION` equal the selected value
- **AND** other terminals are unchanged

#### Scenario: Configured order is preserved
- **WHEN** the region menu is displayed
- **THEN** entries appear in the same order as the top-level `regions` sequence
- **AND** they are not alphabetically reordered

#### Scenario: Current region is marked
- **WHEN** `AWS_REGION` equals a value in the allowlist
- **THEN** that menu entry is visibly marked as active

#### Scenario: Region switching requires a TTY
- **WHEN** `opsx region` runs with piped or redirected stdin
- **THEN** it exits non-zero with a clear interactive-terminal error
- **AND** emits no environment assignments

### Requirement: Region switching is runtime-only
`opsx region` SHALL work with or without an active AWS profile and SHALL modify neither `config.yaml` nor state. It SHALL accept only a selection from the configured allowlist. A later successful `opsx use` or `opsx kube` SHALL replace the manual override with the selected account or cluster's configured region.

#### Scenario: Region switches without an active profile
- **WHEN** no `AWS_PROFILE` is set and the operator selects an allowed region
- **THEN** the invoking terminal's AWS region variables are updated successfully
- **AND** no profile is created or selected

#### Scenario: Region switch is not persisted
- **WHEN** `opsx region` completes successfully
- **THEN** neither configuration nor state files change

#### Scenario: Resource switch supersedes manual region
- **WHEN** an operator manually selects one region and later runs `opsx use` or `opsx kube`
- **THEN** the terminal region becomes the configured region of the newly selected account or cluster

