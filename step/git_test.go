package step

import (
	"testing"

	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_isSSHRemote(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "git@ URL", url: "git@github.com:org/repo.git", want: true},
		{name: "ssh:// URL", url: "ssh://git@github.com/org/repo.git", want: true},
		{name: "https URL", url: "https://github.com/org/repo.git", want: false},
		{name: "empty string", url: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSSHRemote(tt.url))
		})
	}
}

func Test_gitFetchAndCheckout_UsesCredentialHelper(t *testing.T) {
	factory := &fakeCommandFactory{}
	s := Step{
		commandFactory: factory,
		logger:         log.NewLogger(),
	}

	err := s.gitFetchAndCheckout("main", "myuser", "mytoken")
	require.NoError(t, err)

	fetchCall, ok := factory.findCall("fetch")
	require.True(t, ok, "no git fetch command was recorded")

	// The fetch must pass -c credential.helper=<path> so it doesn't rely on
	// ambient credentials such as the .netrc file written by the git-clone step.
	assert.NotEmpty(t, credentialHelperArg(fetchCall.args), "credential.helper arg not found in git fetch args: %v", fetchCall.args)

	require.NotNil(t, fetchCall.opts)
	assert.True(t, envContainsPrefix(fetchCall.opts.Env, "GIT_HELPER_USERNAME="), "GIT_HELPER_USERNAME missing from fetch env")
	assert.True(t, envContainsPrefix(fetchCall.opts.Env, "GIT_HELPER_TOKEN="), "GIT_HELPER_TOKEN missing from fetch env")
}

func Test_gitFetchAndCheckout_NoCredentialHelper_SSH(t *testing.T) {
	factory := &fakeCommandFactory{}
	s := Step{
		commandFactory: factory,
		logger:         log.NewLogger(),
	}

	err := s.gitFetchAndCheckout("main", "", "")
	require.NoError(t, err)

	fetchCall, ok := factory.findCall("fetch")
	require.True(t, ok, "no git fetch command was recorded")

	assert.Empty(t, credentialHelperArg(fetchCall.args), "credential.helper should not be set for SSH remotes: %v", fetchCall.args)
	if fetchCall.opts != nil {
		assert.False(t, envContainsPrefix(fetchCall.opts.Env, "GIT_HELPER_TOKEN="), "GIT_HELPER_TOKEN should not be set for SSH remotes")
	}
}

func Test_gitPush_UsesCredentialHelper(t *testing.T) {
	factory := &fakeCommandFactory{}
	s := Step{
		commandFactory: factory,
		logger:         log.NewLogger(),
		envRepo:        fakeEnvRepo{},
	}

	err := s.gitPush("myuser", "mytoken", "main")
	require.NoError(t, err)

	pushCall, ok := factory.findCall("push")
	require.True(t, ok, "no git push command was recorded")

	assert.NotEmpty(t, credentialHelperArg(pushCall.args), "credential.helper arg not found in git push args: %v", pushCall.args)

	require.NotNil(t, pushCall.opts)
	assert.True(t, envContainsPrefix(pushCall.opts.Env, "GIT_HELPER_TOKEN="), "GIT_HELPER_TOKEN missing from push env")
}

func Test_gitPush_NoCredentialHelper_SSH(t *testing.T) {
	factory := &fakeCommandFactory{}
	s := Step{
		commandFactory: factory,
		logger:         log.NewLogger(),
		envRepo:        fakeEnvRepo{},
	}

	err := s.gitPush("", "", "main")
	require.NoError(t, err)

	pushCall, ok := factory.findCall("push")
	require.True(t, ok, "no git push command was recorded")

	assert.Empty(t, credentialHelperArg(pushCall.args), "credential.helper should not be set for SSH remotes: %v", pushCall.args)
	if pushCall.opts != nil {
		assert.False(t, envContainsPrefix(pushCall.opts.Env, "GIT_HELPER_TOKEN="), "GIT_HELPER_TOKEN should not be set for SSH remotes")
	}
}

func Test_isGitHubAppPermissionDenied(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name: "GitHub App permission denied",
			// Copied from a real Bitrise build failure.
			output: "remote: Permission to ofalvai/bitrise-step-autofix-ci.git denied to bitrise[bot].\nfatal: unable to access 'https://github.com/ofalvai/bitrise-step-autofix-ci.git/': The requested URL returned error: 403",
			want:  true,
		},
		{
			name:   "unrelated push failure",
			output: "fatal: unable to access 'https://github.com/org/repo.git/': Could not resolve host: github.com",
			want:   false,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isGitHubAppPermissionDenied(tt.output))
		})
	}
}

func Test_parseGitStatus(t *testing.T) {
	tests := []struct {
		name             string
		output           string
		includeUntracked bool
		want             []string
	}{
		{
			name:             "empty output means no changes",
			output:           "",
			includeUntracked: true,
			want:             nil,
		},
		{
			name:             "whitespace-only output means no changes",
			output:           "   \n  ",
			includeUntracked: true,
			want:             nil,
		},
		{
			name:             "modified tracked file",
			output:           " M main.go",
			includeUntracked: true,
			want:             []string{"main.go"},
		},
		{
			name:             "staged modification",
			output:           "M  main.go",
			includeUntracked: true,
			want:             []string{"main.go"},
		},
		{
			name:             "untracked new file included",
			output:           "?? newfile.go",
			includeUntracked: true,
			want:             []string{"newfile.go"},
		},
		{
			name:             "untracked new file excluded",
			output:           "?? newfile.go",
			includeUntracked: false,
			want:             nil,
		},
		{
			name:             "mix of tracked changes and untracked files, all included",
			output:           " M existing.go\n?? generated.go\nA  staged-new.go",
			includeUntracked: true,
			want:             []string{"existing.go", "generated.go", "staged-new.go"},
		},
		{
			name:             "mix of tracked changes and untracked files, untracked excluded",
			output:           " M existing.go\n?? generated.go\nA  staged-new.go",
			includeUntracked: false,
			want:             []string{"existing.go", "staged-new.go"},
		},
		{
			name:             "deleted file",
			output:           " D removed.go",
			includeUntracked: true,
			want:             []string{"removed.go"},
		},
		{
			name:             "file with spaces in name",
			output:           " M my file.go",
			includeUntracked: true,
			want:             []string{"my file.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseGitStatus(tt.output, tt.includeUntracked))
		})
	}
}
