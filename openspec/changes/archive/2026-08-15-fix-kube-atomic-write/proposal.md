## Why
`aws eks update-kubeconfig` is an external writer with no atomicity; opsx previously aimed it at the final per-(cluster,mode) path. Two terminals switching the same cluster concurrently, or a cancelled switch killing the CLI mid-write, could leave a torn kubeconfig that a live terminal silently consumes.

## What Changes
- The CLI writes a staging file in the target directory, fsyncs it, chmods 0600, and renames it over the target, all under the shared opsx advisory lock.
- Concurrent same-cluster switches serialize; cancelled or failed runs leave the previous kubeconfig untouched and leak no staging files.

## Capabilities
### Modified Capabilities
- `cluster-switching`: atomic per-cluster kubeconfig publication.

## Impact
internal/kube (staging+lock+rename; LockPath seam); no CLI or config changes.
