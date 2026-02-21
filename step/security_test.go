package step

import "testing"

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
