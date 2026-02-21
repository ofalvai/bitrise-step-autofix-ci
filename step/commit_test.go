package step

import (
	"strings"
	"testing"
)

func Test_buildCommitMessage(t *testing.T) {
	msg := buildCommitMessage("Bitrise CI Autofix", []string{"main.go", "step/step.go"})

	if !strings.HasPrefix(msg, "Bitrise CI Autofix\n") {
		t.Errorf("message should start with the subject line, got: %q", msg)
	}
	if !strings.Contains(msg, "Previous steps in this CI workflow") {
		t.Errorf("message should explain the commit was created by previous CI steps, got: %q", msg)
	}
	if !strings.Contains(msg, stepRepoURL) {
		t.Errorf("message should contain step repo URL %q", stepRepoURL)
	}
	if !strings.Contains(msg, "- main.go\n") {
		t.Errorf("message should list main.go")
	}
	if !strings.Contains(msg, "- step/step.go\n") {
		t.Errorf("message should list step/step.go")
	}
	// Files must appear after the URL, not mixed into the header.
	urlPos := strings.Index(msg, stepRepoURL)
	filesPos := strings.Index(msg, "- main.go")
	if filesPos < urlPos {
		t.Errorf("file list should appear after the step URL")
	}
}
