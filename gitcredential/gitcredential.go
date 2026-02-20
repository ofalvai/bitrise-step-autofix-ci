// Package gitcredential provides a way to supply git credentials without
// exposing them in process arguments or remote URLs.
package gitcredential

import (
	"fmt"
	"os"
)

// Helper holds everything needed to configure a git subprocess with credentials.
// The script file is benign on its own — it reads from env vars rather than
// containing the credentials directly. Call os.Remove(Helper.Path) after use.
type Helper struct {
	// Path is the executable credential helper script to pass to git via
	// -c credential.helper=<Path>.
	Path string
	// Env is the set of environment variables to add to the git subprocess so
	// the script can read the actual credentials at runtime.
	Env []string
}

const (
	usernameEnvKey = "GIT_HELPER_USERNAME"
	tokenEnvKey    = "GIT_HELPER_TOKEN"
)

// WriteHelper writes a git credential helper script to a temporary file and
// returns a Helper with the script path and the env vars the script expects.
// The caller must delete the file after use (typically via defer os.Remove(h.Path)).
//
// Security rationale:
//
// We need to supply a username and token to `git push` without embedding them in
// the remote URL (which would expose them in `git remote -v`, git reflog, and
// potentially in CI logs) and without passing them as shell-function arguments
// (which would expose them in process listings via `ps aux`).
//
// Git's credential helper protocol solves this: when git needs credentials it
// executes the helper binary and reads "username=..." / "password=..." from its
// stdout. The helper path appears in the process argument list, but not the
// credentials themselves.
//
// Compared to writing credentials directly into the script file, this approach
// keeps the file itself free of secrets: it only references env vars. The actual
// credentials are passed to the git subprocess via its environment, which is the
// standard CI secret-passing mechanism and carries no worse visibility than any
// other env var (e.g. the GIT_HTTP_PASSWORD Bitrise already provides).
//
// Properties of this approach:
//   - The script file contains no secrets and needs only execute permission (0700).
//   - Credentials live in subprocess memory only; nothing sensitive touches disk.
//   - Env vars are scoped to the git subprocess via command options, not set
//     globally in the parent process.
//
// Usage:
//
//	h, err := gitcredential.WriteHelper(username, token)
//	if err != nil { ... }
//	defer os.Remove(h.Path)
//
//	// Pass path and env to the git command:
//	git -c credential.helper=<h.Path> push ...   (with h.Env set on the subprocess)
func WriteHelper(username, token string) (Helper, error) {
	f, err := os.CreateTemp("", "git-credential-*")
	if err != nil {
		return Helper{}, fmt.Errorf("create credential helper: %w", err)
	}
	path := f.Name()

	script := fmt.Sprintf("#!/bin/sh\necho \"username=$%s\"\necho \"password=$%s\"\n", usernameEnvKey, tokenEnvKey)
	_, writeErr := fmt.Fprint(f, script)
	closeErr := f.Close()

	if writeErr != nil {
		os.Remove(path)
		return Helper{}, fmt.Errorf("write credential helper: %w", writeErr)
	}
	if closeErr != nil {
		os.Remove(path)
		return Helper{}, fmt.Errorf("close credential helper: %w", closeErr)
	}

	// os.CreateTemp creates with mode 0600; we need execute permission for git to run it.
	if err := os.Chmod(path, 0700); err != nil {
		os.Remove(path)
		return Helper{}, fmt.Errorf("chmod credential helper: %w", err)
	}

	return Helper{
		Path: path,
		Env: []string{
			fmt.Sprintf("%s=%s", usernameEnvKey, username),
			fmt.Sprintf("%s=%s", tokenEnvKey, token),
		},
	}, nil
}
