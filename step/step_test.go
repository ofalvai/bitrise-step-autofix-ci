package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockEnvRepo is a simple map-backed env.Repository for tests.
type mockEnvRepo map[string]string

func (m mockEnvRepo) Get(key string) string  { return m[key] }
func (m mockEnvRepo) List() []string         { return nil }
func (m mockEnvRepo) Set(k, v string) error  { return nil }
func (m mockEnvRepo) Unset(k string) error   { return nil }

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
			s := Step{envRepo: mockEnvRepo{
				"GIT_REPOSITORY_URL":                    tt.repoURL,
				"BITRISEIO_PULL_REQUEST_REPOSITORY_URL": tt.prRepoURL,
			}}
			assert.Equal(t, tt.want, s.isForkPR())
		})
	}
}
