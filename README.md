# Autofix CI

Commits and pushes any local file changes made by previous steps, then fails the build so the current (unfixed) commit doesn't pass CI.

## Overview

Many CI workflows run tools that modify files as a side effect: code formatters, linters with auto-fix, code generators, lockfile updaters, and so on. Normally these changes are lost when the build finishes, requiring a developer to run the tool locally, commit the result, and push again.

Autofix CI closes that loop automatically:

1. It detects any uncommitted file changes left by previous steps.
2. It commits those changes under a bot identity and pushes them to the PR's source branch. The push triggers a new CI build on the fixed commit.
3. It **intentionally fails the current build** so the unfixed commit doesn't pass any quality gates downstream.

This means your PR author never has to manually run `prettier`, `ktlint`, or similar tools. The bot does it for them.

### Typical workflow placement

Place this step **after** any steps that may modify files (formatters, linters, generators) and **before** any steps that enforce quality gates (tests, lint checks). When the step finds changes to push, the build fails at this point and the quality gates run on the corrected commit in the next build.

```
┌─────────────────────────────────┐
│  Git Clone                      │
│  Run formatter / linter / gen   │
│  Autofix CI  ◄── this step      │   ← fails here if fixes were needed
│  Run tests                      │   ← only reached if no fixes needed
│  Deploy / notify                │
└─────────────────────────────────┘
```

### What triggers a skip

The step exits successfully without doing anything in these cases:

- **Not a PR build** — the `BITRISE_PULL_REQUEST` environment variable is not set. The step is designed for PR workflows; push builds are left untouched.
- **Fork PR** — the PR source repository differs from the target repository. The step cannot push to a forked repository using the provided credentials.
- **No changes detected** — there are no uncommitted modifications to commit.

## Authentication

The step supports both HTTPS and SSH remotes, matching however the preceding Git Clone step configured the repository.

**HTTPS:** Supply a `git_token` (and optionally a `git_username`). Bitrise provides these automatically via `GIT_HTTP_PASSWORD` and `GIT_HTTP_USERNAME`, so the defaults work out of the box. Credentials are passed to `git push` through git's credential helper protocol — they are never written to disk or embedded in the remote URL.

**SSH:** If the repository was cloned over SSH and an SSH key is already loaded in the agent, the push uses SSH automatically. The `git_token` and `git_username` inputs are ignored.

## Security

The step includes a guard against CI config tampering: if any changed file is `bitrise.yml`, `bitrise.yaml`, or anything under `.bitrise/`, the step aborts with an error instead of committing. This prevents a malicious PR from using the autofix mechanism to sneak CI configuration changes through an auto-commit.

## Inputs

### `commit_subject`

**Default:** `Bitrise CI Autofix`

The subject line of the autofix commit. The step automatically appends a body listing the modified files and a link to this repository, so you don't need to include that in the subject.

---

### `git_username`

**Default:** `$GIT_HTTP_USERNAME`
**Category:** Authentication
**Sensitive**

Username for HTTPS git push authentication. Bitrise populates `GIT_HTTP_USERNAME` automatically for most repository integrations, so the default works without any configuration. Not used for SSH remotes.

---

### `git_token`

**Default:** `$GIT_HTTP_PASSWORD`
**Category:** Authentication
**Sensitive**

Token or password for HTTPS git push authentication. Bitrise populates `GIT_HTTP_PASSWORD` automatically, so the default works without any configuration. Not used for SSH remotes.

---

### `include_untracked`

**Default:** `true`
**Values:** `true` | `false`

Controls whether new files that are not yet tracked by git are included in the autofix commit.

- `true` (default): All new and modified files are committed. This is the right choice for most use cases, including code generators that create new files.
- `false`: Only modifications to already-tracked files are committed. Use this if previous steps might create temporary files or build artifacts that should not end up in the repository.

---

### `dry_run`

**Default:** `false`
**Values:** `true` | `false`
**Category:** Debug

When enabled, the step runs all detection and security checks and creates a local commit, but skips the `git push`. The build exits with success rather than the usual intentional failure.

Use this to verify the step's behavior without affecting your repository. Note that `AUTOFIX_PUSHED` is always `false` in dry run mode.

---

### `verbose`

**Default:** `false`
**Values:** `true` | `false`
**Category:** Debug

Enables additional log output for troubleshooting. Useful when diagnosing why the step is or isn't detecting changes, or investigating authentication issues.

## Outputs

### `AUTOFIX_NEEDED`

`true` if uncommitted changes were detected, `false` otherwise. Set regardless of whether the push succeeded.

### `AUTOFIX_PUSHED`

`true` if the autofix commit was successfully pushed to the remote. `false` when no changes were found, when the build was skipped, or when running in dry run mode.

### `AUTOFIX_FILE_COUNT`

The number of files included in the autofix commit. `0` when no changes were found or the step was skipped.
