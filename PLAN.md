# Autofix CI step

Rough idea: Like https://autofix.ci, but as a Bitrise step.

Differences:
- Because we want to focus on private repos only, the architecture is vastly simpler. There is no need for a server and a GitHub OAuth app, the step can directly push changes from CI. Bitrise provides the git http username and password (token) as secret env vars that the step can provide to git.
- For fork PRs, refuse to run. Detect by comparing `$BITRISEIO_PULL_REQUEST_REPOSITORY_URL` with `$GIT_REPOSITORY_URL`: if they differ, the PR comes from a fork. There is no dedicated "is_private" env var from Bitrise.
- Set a proper user.name and user.email before committing because we do the commit directly from the CI machine
- Try not to make lasting changes to global and system-wide configs and files because Bitrise steps could also run in non-ephemeral environments and we don't want to override anything. For example, no `git config user.name ...`, but `git -c user.name ... commit ...`, no `.netrc` file shenanigans, etc.
- Try to be git force agnostic, do not rely on GitHub's REST API or anything GitHub-specific

Step inputs:
- git_username: default to $GIT_HTTP_USERNAME
- git_token: default to $$GIT_HTTP_PASSWORD
- commit_message: default to Bitrise CI Autofix

Step outputs:
- AUTOFIX_NEEDED: true if dirty, regardless of the outcome of the git push
- AUTOFIX_PUSHED: true if successful
- AUTOFIX_FILE_COUNT

Flow:
- validate inputs
- detect fork PRs: if `$BITRISEIO_PULL_REQUEST_REPOSITORY_URL` is set and differs from `$GIT_REPOSITORY_URL`, exit successfully with a clear log message (not a failure — the step correctly determined it shouldn't run, same pattern as "nothing to do" in other steps)
- run `git diff HEAD --name-only --no-renames` to detect dirty state. Using `HEAD` as the base covers both staged and unstaged changes in a single call.
- if not, early exit
- if dirty, continue
- if changed files include `bitrise.yml`, `bitrise.yaml`, or files in `.bitrise` dir, abort for security reasons
- determine push target branch: use `$BITRISEIO_PULL_REQUEST_HEAD_BRANCH` for PR builds (the source branch of the PR), fall back to `$BITRISE_GIT_BRANCH` for push builds. The build may be in a detached HEAD state (shallow clone of merge ref), so always push to the resolved branch name rather than relying on the current HEAD ref.
- git add + commit using `-c user.name=... -c user.email=...` inline flags (no global config changes). Use a fixed bot identity ("Bitrise Autofix" / noreply address) rather than forwarding the triggering committer's identity, to make autofix commits clearly distinguishable.
- git push using `git -c credential.helper='!f() { echo username=$GIT_HTTP_USERNAME; echo password=$GIT_HTTP_PASSWORD; }; f' push origin HEAD:<branch>` where `<branch>` is the resolved push target. This avoids global config changes and avoids embedding credentials in the remote URL (which would expose them in process listings).
- return with exit code 1 so that the Bitrise workflow fails (the pushed commit triggers a new Bitrise build, which hopefully succeeds)
- note: if two parallel builds both detect a dirty state and push, one will get a rejected push. this is acceptable and outside the step's control; the surviving push will trigger a new build.


Architecture:
- Go step
- Unit tests whereever possible, mostly the business logic. Don't overdo it at all costs
- Use https://github.com/bitrise-io/go-steputils for common Go utilities
- Use https://github.com/bitrise-steplib/bitrise-step-restore-cache as a blueprint for how a typical Go Bitrise step looks like and what are the common patterns

Security:
autofix.ci's architecture complexity (external server + GitHub App) exists solely to handle fork PRs on public repos. Fork PRs on GitHub Actions get a read-only GITHUB_TOKEN by default, so autofix.ci can't push directly. Instead it uploads an `autofix.json` artifact to its server, which validates it and pushes using its own GitHub App credentials — keeping auth tokens away from untrusted PR code.

For our Bitrise step, scoped to private repos only:
- Fork PRs are rare and contributors typically have repo write access anyway
- Bitrise provides `GIT_HTTP_USERNAME` / `GIT_HTTP_PASSWORD` directly to the build as secrets, which already have write access — no external server needed
- The main security risks to address:
  - **CI config tampering**: already mitigated by aborting if `bitrise.yml`, `bitrise.yaml`, or `.bitrise/**` files are in the diff
  - **Credential leakage**: use `credential.helper` shell function (see push step above) rather than embedding token in the remote URL
  - **Privilege escalation via committed files**: a malicious PR could try to sneak in changes to CI config that get auto-committed and pushed — the file blocklist above mitigates this
  - **Token scope**: the Bitrise-provided token has write access to the entire repo; we can't narrow it further, which is acceptable for private repos where contributors are trusted

Notable risks that autofix.ci addresses but do NOT apply to us:
- **CVE-2021-21300**: a git vulnerability triggered by processing untrusted repository content (e.g. applying patches from a fork). autofix.ci avoids this by using GitHub's GraphQL API instead of git commands on untrusted data. We are not applying any remote content — we only commit and push local changes, so this doesn't apply.
- **Artifact upload spoofing**: autofix.ci requires the artifact to come from a workflow named exactly "autofix.ci" to prevent a compromised workflow from spoofing it. We have no server and no artifact handoff, so this attack vector doesn't exist.
