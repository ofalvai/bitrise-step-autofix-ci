package step

import "strings"

const stepRepoURL = "https://github.com/bitrise-steplib/bitrise-step-autofix-ci"

func buildCommitMessage(subject string, changedFiles []string) string {
	var sb strings.Builder
	sb.WriteString(subject)
	sb.WriteString("\n\nPrevious steps in this CI workflow created uncommitted file changes\n")
	sb.WriteString("(e.g. a code formatter, linter, or code generator). This commit\n")
	sb.WriteString("captures those changes.\n")
	sb.WriteString("\n")
	sb.WriteString(stepRepoURL)
	sb.WriteString("\n\nModified files:\n")
	for _, f := range changedFiles {
		sb.WriteString("- ")
		sb.WriteString(f)
		sb.WriteString("\n")
	}
	return sb.String()
}
