package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		name   string
		output string
		want   []string
	}{
		{
			name:   "empty output means no changes",
			output: "",
			want:   nil,
		},
		{
			name:   "whitespace-only output means no changes",
			output: "   \n  ",
			want:   nil,
		},
		{
			name:   "modified tracked file",
			output: " M main.go",
			want:   []string{"main.go"},
		},
		{
			name:   "staged modification",
			output: "M  main.go",
			want:   []string{"main.go"},
		},
		{
			name:   "untracked new file",
			output: "?? newfile.go",
			want:   []string{"newfile.go"},
		},
		{
			name:   "mix of tracked changes and untracked files",
			output: " M existing.go\n?? generated.go\nA  staged-new.go",
			want:   []string{"existing.go", "generated.go", "staged-new.go"},
		},
		{
			name:   "deleted file",
			output: " D removed.go",
			want:   []string{"removed.go"},
		},
		{
			name:   "file with spaces in name",
			output: " M my file.go",
			want:   []string{"my file.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseGitStatus(tt.output))
		})
	}
}
