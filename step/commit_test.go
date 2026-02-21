package step

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_buildCommitMessage(t *testing.T) {
	msg := buildCommitMessage("Bitrise CI Autofix", []string{"main.go", "step/step.go"})

	assert.True(t, strings.HasPrefix(msg, "Bitrise CI Autofix\n"), "message should start with the subject line")
	assert.Contains(t, msg, "Previous steps in this CI workflow")
	assert.Contains(t, msg, stepRepoURL)
	assert.Contains(t, msg, "- main.go\n")
	assert.Contains(t, msg, "- step/step.go\n")

	// Files must appear after the URL, not mixed into the header.
	urlPos := strings.Index(msg, stepRepoURL)
	filesPos := strings.Index(msg, "- main.go")
	assert.Greater(t, filesPos, urlPos, "file list should appear after the step URL")
}
