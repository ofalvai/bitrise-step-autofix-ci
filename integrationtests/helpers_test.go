//go:build integrationtests

package integrationtests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/bitrise-steplib/bitrise-step-autofix-ci/step"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/stretchr/testify/require"
)

type gitRepo struct {
	workdir   string
	remoteDir string
}

// setupRepo creates a self-contained git environment in a temp directory:
//
//	tmpdir/
//	  remote.git/   bare repo that acts as "origin" — no server needed,
//	               git treats local paths as valid remotes
//	  workdir/      cloned from remote.git, this is where the step runs
//
// An initial commit is pushed so both sides start in sync.
// Subsequent tests modify workdir (leaving uncommitted changes) and run the step there.
// Push assertions check remote.git's commit log directly.
func setupRepo(t *testing.T) gitRepo {
	t.Helper()
	dir := t.TempDir()
	remoteDir := filepath.Join(dir, "remote.git")
	workdir := filepath.Join(dir, "workdir")

	runGit(t, dir, "-c", "init.defaultBranch=main", "init", "--bare", "remote.git")
	runGit(t, dir, "clone", remoteDir, "workdir")
	runGit(t, workdir, "config", "user.email", "test@test.com")
	runGit(t, workdir, "config", "user.name", "Test")

	writeFile(t, workdir, "README.md", "# Test repo")
	runGit(t, workdir, "add", ".")
	runGit(t, workdir, "commit", "-m", "Initial commit")
	runGit(t, workdir, "push", "origin", "main")

	return gitRepo{workdir: workdir, remoteDir: remoteDir}
}

// setupShallowRepo is like setupRepo but workdir is a shallow clone (--depth 1),
// simulating Bitrise's default PR checkout behaviour.
//
// The remote gets two commits before the shallow clone is made so that --depth 1
// actually truncates history. The clone uses a file:// URL because the local
// path transport does not support shallow clones; file:// forces the Git smart
// protocol which does.
func setupShallowRepo(t *testing.T) gitRepo {
	t.Helper()
	dir := t.TempDir()
	remoteDir := filepath.Join(dir, "remote.git")
	workdir := filepath.Join(dir, "workdir")

	runGit(t, dir, "-c", "init.defaultBranch=main", "init", "--bare", "remote.git")

	// A staging clone seeds the remote with history before the shallow clone is made.
	// The shallow workdir cannot push additional commits to set this up itself.
	staging := filepath.Join(dir, "staging")
	runGit(t, dir, "clone", remoteDir, "staging")
	runGit(t, staging, "config", "user.email", "test@test.com")
	runGit(t, staging, "config", "user.name", "Test")
	writeFile(t, staging, "README.md", "# Test repo")
	runGit(t, staging, "add", ".")
	runGit(t, staging, "commit", "-m", "Initial commit")
	writeFile(t, staging, "second.txt", "second")
	runGit(t, staging, "add", ".")
	runGit(t, staging, "commit", "-m", "Second commit")
	runGit(t, staging, "push", "origin", "main")

	runGit(t, dir, "clone", "--depth", "1", "file://"+remoteDir, "workdir")
	runGit(t, workdir, "config", "user.email", "test@test.com")
	runGit(t, workdir, "config", "user.name", "Test")

	return gitRepo{workdir: workdir, remoteDir: remoteDir}
}

func setCommonEnvs(t *testing.T, r gitRepo) {
	t.Helper()
	t.Setenv("git_token", "dummy")
	t.Setenv("commit_subject", "Test Autofix")
	t.Setenv("dry_run", "false")
	t.Setenv("verbose", "false")
	t.Setenv("BITRISE_GIT_BRANCH", "main")
	t.Setenv("GIT_REPOSITORY_URL", "file://"+r.remoteDir)
}

// runStep changes the working directory to workdir for the duration of the test
// so that git commands inside the step run against the right repo.
// Tests must not run in parallel because os.Chdir is process-global.
func runStep(t *testing.T, workdir string) (step.Result, error) {
	t.Helper()

	orig, err := os.Getwd()
	require.NoError(t, err)

	require.NoError(t, os.Chdir(workdir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(orig))
	})

	envRepo := env.NewRepository()
	logger := log.NewLogger()
	inputParser := stepconf.NewInputParser(envRepo)
	commandFactory := command.NewFactory(envRepo)
	s := step.New(logger, inputParser, commandFactory, envRepo)
	return s.Run()
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v\n%s", args, out)
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

func latestCommitSubject(t *testing.T, dir string) string {
	t.Helper()
	return runGit(t, dir, "log", "--format=%s", "-1")
}

func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	return runGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD")
}

func commitCount(t *testing.T, dir string) int {
	t.Helper()
	return commitCountOnBranch(t, dir, "HEAD")
}

func commitCountOnBranch(t *testing.T, dir, branch string) int {
	t.Helper()
	out := runGit(t, dir, "rev-list", "--count", branch)
	n, err := strconv.Atoi(out)
	require.NoError(t, err, "parse commit count %q", out)
	return n
}
