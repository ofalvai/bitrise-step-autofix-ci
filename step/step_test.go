package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_isPRBuild(t *testing.T) {
	tests := []struct {
		name     string
		prNumber string
		want     bool
	}{
		{name: "not a PR build", prNumber: "", want: false},
		{name: "PR build", prNumber: "123", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Step{envRepo: fakeEnvRepo{"BITRISE_PULL_REQUEST": tt.prNumber}}
			assert.Equal(t, tt.want, s.isPRBuild())
		})
	}
}

func Test_isForkPR(t *testing.T) {
	tests := []struct {
		name      string
		repoURL   string
		prRepoURL string
		want      bool
	}{
		{
			name:      "not a PR build",
			repoURL:   "https://github.com/org/repo.git",
			prRepoURL: "",
			want:      false,
		},
		{
			name:      "PR from same repo",
			repoURL:   "https://github.com/org/repo.git",
			prRepoURL: "https://github.com/org/repo.git",
			want:      false,
		},
		{
			name:      "PR from fork",
			repoURL:   "https://github.com/org/repo.git",
			prRepoURL: "https://github.com/contributor/repo.git",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Step{envRepo: fakeEnvRepo{
				"GIT_REPOSITORY_URL":                    tt.repoURL,
				"BITRISEIO_PULL_REQUEST_REPOSITORY_URL": tt.prRepoURL,
			}}
			assert.Equal(t, tt.want, s.isForkPR())
		})
	}
}
