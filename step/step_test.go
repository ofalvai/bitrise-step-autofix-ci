package step

import (
	"testing"
)

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
			got := isForkPR(tt.repoURL, tt.prRepoURL)
			if got != tt.want {
				t.Errorf("isForkPR() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_resolvePushBranch(t *testing.T) {
	tests := []struct {
		name         string
		prHeadBranch string
		gitBranch    string
		want         string
	}{
		{
			name:         "PR build uses PR head branch",
			prHeadBranch: "feature/my-pr",
			gitBranch:    "main",
			want:         "feature/my-pr",
		},
		{
			name:         "push build falls back to git branch",
			prHeadBranch: "",
			gitBranch:    "main",
			want:         "main",
		},
		{
			name:         "both empty returns empty",
			prHeadBranch: "",
			gitBranch:    "",
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePushBranch(tt.prHeadBranch, tt.gitBranch)
			if got != tt.want {
				t.Errorf("resolvePushBranch() = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_checkForCIConfigChanges(t *testing.T) {
	tests := []struct {
		name         string
		changedFiles []string
		wantErr      bool
	}{
		{
			name:         "no CI config changes",
			changedFiles: []string{"main.go", "README.md", "go.sum"},
			wantErr:      false,
		},
		{
			name:         "root-level bitrise.yml changed",
			changedFiles: []string{"main.go", "bitrise.yml"},
			wantErr:      true,
		},
		{
			name:         "root-level bitrise.yaml changed",
			changedFiles: []string{"bitrise.yaml"},
			wantErr:      true,
		},
		{
			name:         "bitrise.yml in subdirectory is also blocked",
			changedFiles: []string{"subdir/bitrise.yml"},
			wantErr:      true,
		},
		{
			name:         "file in .bitrise dir changed",
			changedFiles: []string{".bitrise/workflows/deploy.yml"},
			wantErr:      true,
		},
		{
			name:         "rename to bitrise.yml is blocked",
			changedFiles: []string{"old.go -> bitrise.yml"},
			wantErr:      true,
		},
		{
			name:         "rename from bitrise.yml is blocked",
			changedFiles: []string{"bitrise.yml -> new.go"},
			wantErr:      true,
		},
		{
			name:         "rename from .bitrise dir is blocked",
			changedFiles: []string{".bitrise/workflows/deploy.yml -> deploy.yml"},
			wantErr:      true,
		},
		{
			name:         "innocent rename is allowed",
			changedFiles: []string{"old.go -> new.go"},
			wantErr:      false,
		},
		{
			name:         "empty diff",
			changedFiles: []string{},
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkForCIConfigChanges(tt.changedFiles)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkForCIConfigChanges() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
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
			got := isGitHubAppPermissionDenied(tt.output)
			if got != tt.want {
				t.Errorf("isGitHubAppPermissionDenied() = %v, want %v", got, tt.want)
			}
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
			got := parseGitStatus(tt.output)
			if len(got) != len(tt.want) {
				t.Errorf("parseGitStatus() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseGitStatus()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
