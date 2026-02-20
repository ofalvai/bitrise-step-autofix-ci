package step

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bitrise-steplib/bitrise-step-autofix-ci/gitcredential"

	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
)

const (
	botName  = "Bitrise Autofix"
	botEmail = "autofix@bitrise.io"
)

type Input struct {
	GitUsername   string `env:"git_username"`
	GitToken      string `env:"git_token"`
	CommitMessage string `env:"commit_message,required"`
	Verbose       bool   `env:"verbose,required"`
}

type Result struct {
	AutofixNeeded bool
	AutofixPushed bool
	FileCount     int
}

type Step struct {
	logger         log.Logger
	inputParser    stepconf.InputParser
	commandFactory command.Factory
	envRepo        env.Repository
}

func New(
	logger log.Logger,
	inputParser stepconf.InputParser,
	commandFactory command.Factory,
	envRepo env.Repository,
) Step {
	return Step{
		logger:         logger,
		inputParser:    inputParser,
		commandFactory: commandFactory,
		envRepo:        envRepo,
	}
}

func (s Step) Run() (Result, error) {
	var input Input
	if err := s.inputParser.Parse(&input); err != nil {
		return Result{}, fmt.Errorf("parse inputs: %w", err)
	}
	stepconf.Print(input)
	s.logger.EnableDebugLog(input.Verbose)

	// The step.yml defaults expand $GIT_HTTP_USERNAME/$GIT_HTTP_PASSWORD before the binary runs,
	// so these are already resolved by the time we get here.
	// Username is optional: GitHub App installations provide only a short-lived token.
	if input.GitToken == "" {
		return Result{}, fmt.Errorf("git token is required: set git_token input or ensure GIT_HTTP_PASSWORD is available in the environment")
	}

	// Bitrise system env vars — not step inputs, read directly from the environment.
	gitRepoURL := s.envRepo.Get("GIT_REPOSITORY_URL")
	prRepoURL := s.envRepo.Get("BITRISEIO_PULL_REQUEST_REPOSITORY_URL")
	gitBranch := s.envRepo.Get("BITRISE_GIT_BRANCH")

	if isForkPR(gitRepoURL, prRepoURL) {
		s.logger.Println()
		s.logger.Infof("Skipping: this build is for a fork PR. Autofix cannot push to a forked repository.")
		return Result{}, nil
	}

	changedFiles, err := s.getChangedFiles()
	if err != nil {
		return Result{}, fmt.Errorf("detect changes: %w", err)
	}

	if len(changedFiles) == 0 {
		s.logger.Println()
		s.logger.Infof("No changes detected, nothing to commit.")
		return Result{}, nil
	}

	s.logger.Println()
	s.logger.Infof("Detected %d changed file(s):", len(changedFiles))
	for _, f := range changedFiles {
		s.logger.Printf("  %s", f)
	}

	if err := checkForCIConfigChanges(changedFiles); err != nil {
		return Result{AutofixNeeded: true}, fmt.Errorf("security check failed: %w", err)
	}

	if gitBranch == "" {
		return Result{AutofixNeeded: true}, fmt.Errorf("could not determine push target branch: BITRISE_GIT_BRANCH is empty")
	}

	s.logger.Println()
	s.logger.Infof("Committing and pushing changes to branch: %s", gitBranch)

	if err := s.gitAddAll(); err != nil {
		return Result{AutofixNeeded: true}, fmt.Errorf("git add: %w", err)
	}

	if err := s.gitCommit(input.CommitMessage); err != nil {
		return Result{AutofixNeeded: true}, fmt.Errorf("git commit: %w", err)
	}

	if err := s.gitPush(input.GitUsername, input.GitToken, gitBranch); err != nil {
		return Result{AutofixNeeded: true}, fmt.Errorf("git push: %w", err)
	}

	s.logger.Println()
	s.logger.Donef("Successfully pushed autofix commit to %s", gitBranch)

	return Result{
		AutofixNeeded: true,
		AutofixPushed: true,
		FileCount:     len(changedFiles),
	}, nil
}

// isForkPR returns true when the PR comes from a fork (different repo URL).
// An empty PR repo URL means this is not a PR build at all.
func isForkPR(repoURL, prRepoURL string) bool {
	if prRepoURL == "" {
		return false
	}
	return repoURL != prRepoURL
}

// checkForCIConfigChanges aborts if any changed file touches Bitrise CI config,
// to prevent a malicious PR from sneaking CI config changes through autofix.
func checkForCIConfigChanges(changedFiles []string) error {
	for _, f := range changedFiles {
		// Rename entries from git status --porcelain look like "ORIG_PATH -> NEW_PATH";
		// check each side independently so neither endpoint can bypass the block.
		for _, part := range strings.SplitN(f, " -> ", 2) {
			base := filepath.Base(part)
			if base == "bitrise.yml" || base == "bitrise.yaml" {
				return fmt.Errorf("changed files include CI config file %q — refusing to auto-commit", part)
			}
			if strings.HasPrefix(part, ".bitrise/") || strings.HasPrefix(part, ".bitrise\\") || part == ".bitrise" {
				return fmt.Errorf("changed files include CI config path %q — refusing to auto-commit", part)
			}
		}
	}
	return nil
}


func (s Step) getChangedFiles() ([]string, error) {
	// git status --porcelain covers both modified tracked files and new untracked files.
	// git diff HEAD --name-only would miss untracked files, which are common output from
	// code generators and formatters that create new files.
	cmd := s.commandFactory.Create("git", []string{"status", "--porcelain"}, nil)
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("run git status: %w", err)
	}
	return parseGitStatus(out), nil
}

// parseGitStatus extracts filenames from `git status --porcelain` output.
// Each line is "XY filename" where X is the index (staged) status and Y is the
// worktree status. The filename always starts at position 3.
//
// Important: we split before trimming because the status characters themselves
// can be spaces (e.g. " M file" = unstaged modification), so trimming the full
// output string would corrupt the fixed-column format.
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

func (s Step) gitAddAll() error {
	cmd := s.commandFactory.Create("git", []string{"add", "--all"}, nil)
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}
	return nil
}

func (s Step) gitCommit(message string) error {
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
	return nil
}

func (s Step) gitPush(username, token, branch string) error {
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
