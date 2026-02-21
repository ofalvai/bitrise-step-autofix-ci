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

func (s Step) getChangedFiles(includeUntracked bool) ([]string, error) {
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
	return parseGitStatus(outBuf.String(), includeUntracked), nil
}

// parseGitStatus extracts filenames from `git status --porcelain` output.
// Each line is "XY filename" where X is the index (staged) status and Y is the
// worktree status. The filename always starts at position 3.
//
// Callers must pass the raw output without TrimSpace: status characters can be
// spaces (e.g. " M file" = unstaged modification), so stripping leading
// whitespace from the whole string corrupts the fixed-column format.
func parseGitStatus(output string, includeUntracked bool) []string {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	var files []string
	for _, line := range strings.Split(output, "\n") {
		// Minimum valid line: "XY f" = 4 chars (2 status + space + 1 char filename)
		if len(line) < 4 {
			continue
		}
		// Untracked files are marked with "??" in porcelain format.
		if !includeUntracked && strings.HasPrefix(line, "??") {
			continue
		}
		files = append(files, line[3:])
	}
	return files
}

func (s Step) gitFetchAndCheckout(branch, username, token string) error {
	// PR builds check out refs/pull/N/merge — a temporary merge commit GitHub
	// creates for CI. Its parent chain includes base-branch commits, so pushing
	// HEAD directly to the PR branch would be a non-fast-forward, and the
	// working tree state includes base-branch changes that should not land in
	// the autofix commit.
	//
	// We use commit + cherry-pick to isolate only the formatter's changes:
	// 1. Stage all changes and commit them on the merge ref (temporary commit).
	// 2. Fetch and switch to the actual PR branch tip.
	// 3. Cherry-pick the temp commit with --no-commit, which replays only the
	//    formatter's delta on top of the PR branch via a 3-way merge, leaving
	//    the working tree staged and ready for the real autofix commit.

	s.logger.Debugf("$ git add --all")
	if out, err := s.commandFactory.Create("git", []string{"add", "--all"}, nil).RunAndReturnTrimmedCombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}

	s.logger.Debugf("$ git commit (temporary, on merge ref)")
	if out, err := s.commandFactory.Create("git", []string{
		"-c", fmt.Sprintf("user.name=%s", botName),
		"-c", fmt.Sprintf("user.email=%s", botEmail),
		"commit", "--no-verify", "-m", "autofix-temp",
	}, nil).RunAndReturnTrimmedCombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}

	tempCommit, err := s.commandFactory.Create("git", []string{"rev-parse", "HEAD"}, nil).RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return fmt.Errorf("get temp commit hash: %w", err)
	}
	s.logger.Debugf("Temporary commit on merge ref: %s", tempCommit)

	helper, err := gitcredential.WriteHelper(username, token)
	if err != nil {
		return err
	}
	defer os.Remove(helper.Path)

	s.logger.Debugf("$ git fetch --depth 1 origin %s", branch)
	if out, err := s.commandFactory.Create("git", []string{
		"-c", fmt.Sprintf("credential.helper=%s", helper.Path),
		"fetch", "--depth", "1", "origin", branch,
	}, &command.Opts{Env: helper.Env}).RunAndReturnTrimmedCombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}

	s.logger.Debugf("$ git checkout -B %s origin/%s", branch, branch)
	if out, err := s.commandFactory.Create("git", []string{"checkout", "-B", branch, "origin/" + branch}, nil).RunAndReturnTrimmedCombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}

	s.logger.Debugf("$ git cherry-pick --no-commit %s", tempCommit)
	if out, err := s.commandFactory.Create("git", []string{"cherry-pick", "--no-commit", tempCommit}, nil).RunAndReturnTrimmedCombinedOutput(); err != nil {
		// Cherry-pick leaves the repo in an in-progress state on failure; abort to clean up.
		s.commandFactory.Create("git", []string{"cherry-pick", "--abort"}, nil).RunAndReturnTrimmedCombinedOutput() //nolint:errcheck
		return fmt.Errorf("cherry-pick failed (changes conflict with base branch changes): %w\n%s", err, out)
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
