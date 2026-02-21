package step

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/bitrise-steplib/bitrise-step-autofix-ci/gitcredential"

	"github.com/bitrise-io/go-utils/v2/command"
)

const (
	botName  = "Bitrise Autofix"
	botEmail = "autofix@bitrise.io"
)

func (s Step) getChangedFiles() ([]string, error) {
	// git status --porcelain covers both modified tracked files and new untracked files.
	// git diff HEAD --name-only would miss untracked files, which are common output from
	// code generators and formatters that create new files.
	//
	// We capture stdout into a buffer instead of using RunAndReturnTrimmedCombinedOutput
	// because TrimSpace strips the leading space from the first line, which corrupts the
	// fixed-column porcelain format (e.g. " M file" → "M file", then line[3:] = "ile").
	var outBuf bytes.Buffer
	cmd := s.commandFactory.Create("git", []string{"status", "--porcelain"}, &command.Opts{Stdout: &outBuf})
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run git status: %w", err)
	}
	return parseGitStatus(outBuf.String()), nil
}

// parseGitStatus extracts filenames from `git status --porcelain` output.
// Each line is "XY filename" where X is the index (staged) status and Y is the
// worktree status. The filename always starts at position 3.
//
// Callers must pass the raw output without TrimSpace: status characters can be
// spaces (e.g. " M file" = unstaged modification), so stripping leading
// whitespace from the whole string corrupts the fixed-column format.
func parseGitStatus(output string) []string {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	var files []string
	for _, line := range strings.Split(output, "\n") {
		// Minimum valid line: "XY f" = 4 chars (2 status + space + 1 char filename)
		if len(line) < 4 {
			continue
		}
		files = append(files, line[3:])
	}
	return files
}

func (s Step) gitFetchAndCheckout(branch string) error {
	// PR builds check out refs/pull/N/merge — a temporary merge commit GitHub
	// creates for CI. Its parent chain includes base-branch commits, so pushing
	// HEAD directly to the PR branch would be a non-fast-forward. We fetch and
	// checkout the actual branch tip so our autofix commit lands on top of it.
	//
	// Some autofix files may also differ between the merge ref and the branch tip
	// (when the base branch touched the same file), which makes a plain checkout
	// fail with "your local changes would be overwritten". Stash before checkout
	// and pop after — git does a 3-way merge on pop so conflicts are very unlikely
	// in practice (formatters rarely touch the exact same lines as base changes).
	// This method is only called when changed files have been detected, so the
	// working tree is guaranteed dirty and the stash will always create an entry.
	s.logger.Debugf("$ git stash push --include-untracked")
	if out, err := s.commandFactory.Create("git", []string{"stash", "push", "--include-untracked"}, nil).RunAndReturnTrimmedCombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}

	s.logger.Debugf("$ git fetch --depth 1 origin %s", branch)
	if out, err := s.commandFactory.Create("git", []string{"fetch", "--depth", "1", "origin", branch}, nil).RunAndReturnTrimmedCombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}

	s.logger.Debugf("$ git checkout -B %s origin/%s", branch, branch)
	if out, err := s.commandFactory.Create("git", []string{"checkout", "-B", branch, "origin/" + branch}, nil).RunAndReturnTrimmedCombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}

	s.logger.Debugf("$ git stash pop")
	if out, err := s.commandFactory.Create("git", []string{"stash", "pop"}, nil).RunAndReturnTrimmedCombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}

	return nil
}

func (s Step) gitAddAll() error {
	s.logger.Debugf("$ git add --all")
	cmd := s.commandFactory.Create("git", []string{"add", "--all"}, nil)
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}
	return nil
}

func (s Step) gitCommit(message string) error {
	s.logger.Debugf("$ git commit -m %q", message)
	cmd := s.commandFactory.Create("git", []string{
		"-c", fmt.Sprintf("user.name=%s", botName),
		"-c", fmt.Sprintf("user.email=%s", botEmail),
		"commit",
		"-m", message,
	}, nil)
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}
	s.logger.Debugf("%s", out)
	return nil
}

func (s Step) gitPush(username, token, branch string) error {
	s.logger.Debugf("$ git push origin HEAD:%s", branch)
	helper, err := gitcredential.WriteHelper(username, token)
	if err != nil {
		return err
	}
	defer os.Remove(helper.Path)

	cmd := s.commandFactory.Create("git", []string{
		"-c", fmt.Sprintf("credential.helper=%s", helper.Path),
		"push", "origin", fmt.Sprintf("HEAD:%s", branch),
	}, &command.Opts{Env: helper.Env})
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		if isGitHubAppPermissionDenied(out) {
			appSlug := s.envRepo.Get("BITRISE_APP_SLUG")
			return fmt.Errorf(
				"push failed: the GitHub App token does not have write access to this repository.\n"+
					"Go to https://app.bitrise.io/app/%s/settings/repository and enable "+
					"\"Extend GitHub App permissions to builds\".\n\nOriginal error: %w\n%s",
				appSlug, err, out,
			)
		}
		return fmt.Errorf("%w\n%s", err, out)
	}
	return nil
}

// isGitHubAppPermissionDenied detects the specific 403 error that GitHub returns
// when a build's GitHub App token lacks write permission to the repository.
// This is common on Bitrise because write access must be explicitly enabled in
// the repository settings ("Extend GitHub App permissions to builds").
func isGitHubAppPermissionDenied(gitOutput string) bool {
	return strings.Contains(gitOutput, "remote: Permission to") && strings.Contains(gitOutput, "denied")
}
