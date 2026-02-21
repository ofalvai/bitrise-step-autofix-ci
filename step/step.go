package step

import (
	"fmt"

	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
)

type Input struct {
	GitUsername   string `env:"git_username"`
	GitToken      string `env:"git_token"`
	CommitSubject string `env:"commit_subject,required"`
	DryRun        bool   `env:"dry_run,required"`
	Verbose       bool   `env:"verbose,required"`
}

type Result struct {
	AutofixNeeded bool
	AutofixPushed bool
	FileCount     int
	DryRun        bool
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

	gitBranch := s.envRepo.Get("BITRISE_GIT_BRANCH")

	if !s.isPRBuild() {
		s.logger.Println()
		s.logger.Infof("Skipping: this step is intended for PR builds only (BITRISE_PULL_REQUEST is not set).")
		return Result{}, nil
	}

	if s.isForkPR() {
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

	if err := s.gitFetchAndCheckout(gitBranch, input.GitUsername, input.GitToken); err != nil {
		return Result{AutofixNeeded: true}, fmt.Errorf("checkout branch: %w", err)
	}

	if err := s.gitAddAll(); err != nil {
		return Result{AutofixNeeded: true}, fmt.Errorf("git add: %w", err)
	}

	if err := s.gitCommit(buildCommitMessage(input.CommitSubject, changedFiles)); err != nil {
		return Result{AutofixNeeded: true}, fmt.Errorf("git commit: %w", err)
	}

	if input.DryRun {
		s.logger.Println()
		s.logger.Infof("Dry run: skipping git push. The commit was created locally but not pushed.")
		return Result{
			AutofixNeeded: true,
			AutofixPushed: false,
			FileCount:     len(changedFiles),
			DryRun:        true,
		}, nil
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

func (s Step) isPRBuild() bool {
	return s.envRepo.Get("BITRISE_PULL_REQUEST") != ""
}

// isForkPR returns true when the PR comes from a fork (different repo URL).
// An empty PR repo URL means this is not a PR build at all.
func (s Step) isForkPR() bool {
	repoURL := s.envRepo.Get("GIT_REPOSITORY_URL")
	prRepoURL := s.envRepo.Get("BITRISEIO_PULL_REQUEST_REPOSITORY_URL")
	if prRepoURL == "" {
		return false
	}
	return repoURL != prRepoURL
}
