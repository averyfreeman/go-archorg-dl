---
name: githabits
description: Apply the repository's explicit Git workflow policy.
---

# Githabits

Read `.githabits.yaml` before any Git mutation. The profile is only a default; the action-level switches are authoritative and may revoke autonomous behavior.

## Action contract

- `init`: initialize Git only when `actions.init` is enabled.
- `branch`: create or switch branches using the configured branch convention.
- `stage`: stage only the reviewed repository changes; never stage secrets or unrelated paths.
- `commit`: use the configured commit style and explain the material change.
- `tag`: use the configured SemVer tag convention, beginning at `initial_tag`.
- `remote`: configure the declared provider and alias only after the remote is known.
- `push`: push only when `actions.push` is enabled, the remote status is `configured`, and validation has passed.

All actions remain subject to user approval, repository safety checks, and the
Codex host's permissions. Never expose credentials, rewrite shared history,
force-push, delete branches, or weaken validation to make a workflow pass.
