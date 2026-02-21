//go:build integrationtests

package integrationtests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoChanges(t *testing.T) {
	repo := setupRepo(t)
	setCommonEnvs(t, repo)

	result, err := runStep(t, repo.workdir)

	require.NoError(t, err)
	assert.False(t, result.AutofixNeeded)
	assert.False(t, result.AutofixPushed)
}

func TestDryRun_ChangesDetected(t *testing.T) {
	repo := setupRepo(t)
	writeFile(t, repo.workdir, "generated.txt", "new content")
	setCommonEnvs(t, repo)
	t.Setenv("dry_run", "true")

	initialCount := commitCount(t, repo.remoteDir)
	result, err := runStep(t, repo.workdir)

	require.NoError(t, err)
	assert.True(t, result.AutofixNeeded)
	assert.False(t, result.AutofixPushed)
	assert.True(t, result.DryRun)
	assert.Equal(t, 1, result.FileCount)
	assert.Equal(t, "Test Autofix", latestCommitSubject(t, repo.workdir))
	assert.Equal(t, initialCount, commitCount(t, repo.remoteDir), "remote should be unchanged")
}

func TestRealPush_ChangesDetected(t *testing.T) {
	repo := setupRepo(t)
	writeFile(t, repo.workdir, "generated.txt", "new content")
	setCommonEnvs(t, repo)

	initialCount := commitCount(t, repo.remoteDir)
	result, err := runStep(t, repo.workdir)

	require.NoError(t, err)
	assert.True(t, result.AutofixNeeded)
	assert.True(t, result.AutofixPushed)
	assert.Equal(t, 1, result.FileCount)
	assert.Equal(t, initialCount+1, commitCount(t, repo.remoteDir))
}

func TestNonPRBuild_Skipped(t *testing.T) {
	repo := setupRepo(t)
	writeFile(t, repo.workdir, "generated.txt", "new content")
	setCommonEnvs(t, repo)
	t.Setenv("BITRISE_PULL_REQUEST", "") // override: not a PR build

	result, err := runStep(t, repo.workdir)

	require.NoError(t, err)
	assert.False(t, result.AutofixNeeded)
	assert.False(t, result.AutofixPushed)
}

func TestForkPR_Skipped(t *testing.T) {
	repo := setupRepo(t)
	writeFile(t, repo.workdir, "generated.txt", "new content")
	setCommonEnvs(t, repo)
	// Different URL from GIT_REPOSITORY_URL triggers the fork PR check
	t.Setenv("BITRISEIO_PULL_REQUEST_REPOSITORY_URL", "https://github.com/fork/repo.git")

	result, err := runStep(t, repo.workdir)

	require.NoError(t, err)
	assert.False(t, result.AutofixNeeded)
}

func TestCIConfigChange_SecurityError(t *testing.T) {
	repo := setupRepo(t)
	writeFile(t, repo.workdir, "bitrise.yml", "format_version: '11'")
	setCommonEnvs(t, repo)
	t.Setenv("dry_run", "true")

	result, err := runStep(t, repo.workdir)

	require.Error(t, err)
	assert.True(t, result.AutofixNeeded)
	assert.False(t, result.AutofixPushed)
	// Security check fires before git commit, so no autofix commit should exist.
	assert.Equal(t, "Initial commit", latestCommitSubject(t, repo.workdir))
}

// TestDetachedHEAD simulates a PR build where Bitrise checks out a temporary
// merge ref instead of the actual branch tip, leaving the repo in detached HEAD.
// The step's stash-fetch-checkout-pop dance must handle this correctly.
func TestDetachedHEAD_ChangesDetected(t *testing.T) {
	repo := setupRepo(t)
	runGit(t, repo.workdir, "checkout", "--detach", "HEAD")
	writeFile(t, repo.workdir, "generated.txt", "new content")
	setCommonEnvs(t, repo)
	t.Setenv("dry_run", "true")

	initialCount := commitCount(t, repo.remoteDir)
	result, err := runStep(t, repo.workdir)

	require.NoError(t, err)
	assert.True(t, result.AutofixNeeded)
	assert.True(t, result.DryRun)
	assert.Equal(t, 1, result.FileCount)
	assert.Equal(t, "Test Autofix", latestCommitSubject(t, repo.workdir))
	assert.Equal(t, "main", currentBranch(t, repo.workdir), "step should land on the target branch, not stay detached")
	assert.Equal(t, initialCount, commitCount(t, repo.remoteDir), "remote should be unchanged")
}

// TestShallowBranch simulates a PR build where Bitrise does a shallow clone
// and leaves HEAD on the branch tip rather than a detached merge ref.
func TestShallowBranch_ChangesDetected(t *testing.T) {
	repo := setupShallowRepo(t)
	writeFile(t, repo.workdir, "generated.txt", "new content")
	setCommonEnvs(t, repo)
	t.Setenv("dry_run", "true")

	initialCount := commitCount(t, repo.remoteDir)
	result, err := runStep(t, repo.workdir)

	require.NoError(t, err)
	assert.True(t, result.AutofixNeeded)
	assert.True(t, result.DryRun)
	assert.Equal(t, 1, result.FileCount)
	assert.Equal(t, "Test Autofix", latestCommitSubject(t, repo.workdir))
	assert.Equal(t, "main", currentBranch(t, repo.workdir))
	assert.Equal(t, initialCount, commitCount(t, repo.remoteDir), "remote should be unchanged")
}

// TestShallowDetachedHEAD is the most realistic simulation of a Bitrise PR build:
// shallow clone with HEAD detached at a merge ref (refs/pull/N/merge).
func TestShallowDetachedHEAD_ChangesDetected(t *testing.T) {
	repo := setupShallowRepo(t)
	runGit(t, repo.workdir, "checkout", "--detach", "HEAD")
	writeFile(t, repo.workdir, "generated.txt", "new content")
	setCommonEnvs(t, repo)
	t.Setenv("dry_run", "true")

	initialCount := commitCount(t, repo.remoteDir)
	result, err := runStep(t, repo.workdir)

	require.NoError(t, err)
	assert.True(t, result.AutofixNeeded)
	assert.True(t, result.DryRun)
	assert.Equal(t, 1, result.FileCount)
	assert.Equal(t, "Test Autofix", latestCommitSubject(t, repo.workdir))
	assert.Equal(t, "main", currentBranch(t, repo.workdir), "step should land on the target branch, not stay detached")
	assert.Equal(t, initialCount, commitCount(t, repo.remoteDir), "remote should be unchanged")
}

// TestLocalMergeCommit simulates Bitrise's "merge" checkout strategy: the git-clone
// step fetches the PR head and creates a local merge commit against the base branch.
// HEAD ends up at a merge commit that has never been pushed to the remote.
// BITRISE_GIT_BRANCH points to the PR source branch, so the step must discard the
// local merge commit, push the autofix to the PR branch, and not touch the base branch.
func TestLocalMergeCommit_ChangesDetected(t *testing.T) {
	repo := setupRepo(t)

	// Feature branch with one commit, pushed to remote (the PR source branch).
	runGit(t, repo.workdir, "checkout", "-b", "feature")
	writeFile(t, repo.workdir, "feature.txt", "feature work")
	runGit(t, repo.workdir, "add", ".")
	runGit(t, repo.workdir, "commit", "-m", "Feature commit")
	runGit(t, repo.workdir, "push", "origin", "feature")

	// Local merge of feature into main — never pushed, purely for CI.
	runGit(t, repo.workdir, "checkout", "main")
	runGit(t, repo.workdir, "merge", "--no-ff", "feature", "-m", "Local merge commit")

	// Formatter output produced on the merged tree.
	writeFile(t, repo.workdir, "generated.txt", "new content")

	setCommonEnvs(t, repo)
	t.Setenv("BITRISE_GIT_BRANCH", "feature") // autofix must target the PR branch, not main
	t.Setenv("dry_run", "true")

	featureTip := runGit(t, repo.remoteDir, "rev-parse", "feature")
	initialFeatureCount := commitCountOnBranch(t, repo.remoteDir, "feature")
	result, err := runStep(t, repo.workdir)

	require.NoError(t, err)
	assert.True(t, result.AutofixNeeded)
	assert.True(t, result.DryRun)
	assert.Equal(t, 1, result.FileCount)
	assert.Equal(t, "Test Autofix", latestCommitSubject(t, repo.workdir))
	assert.Equal(t, "feature", currentBranch(t, repo.workdir), "autofix commit should land on the PR branch, not on main")
	// The autofix commit must be built on top of the remote feature tip, not the local merge commit.
	assert.Equal(t, featureTip, runGit(t, repo.workdir, "rev-parse", "HEAD~1"))
	assert.Equal(t, initialFeatureCount, commitCountOnBranch(t, repo.remoteDir, "feature"), "remote feature should be unchanged")
}
