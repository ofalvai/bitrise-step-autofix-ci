package step

import (
	"fmt"
	"path/filepath"
	"strings"
)

// checkForCIConfigChanges aborts if any changed file touches Bitrise CI config,
// to prevent a malicious PR from sneaking CI config changes through autofix.
func checkForCIConfigChanges(changedFiles []string) error {
	for _, f := range changedFiles {
		// Rename entries from git status --porcelain look like "ORIG_PATH -> NEW_PATH";
		// check each side independently so neither endpoint can bypass the block.
		for _, part := range strings.SplitN(f, " -> ", 2) {
			base := filepath.Base(part)
			if base == "bitrise.yml" || base == "bitrise.yaml" {
				return fmt.Errorf("changed files include CI config file %q — refusing to auto-commit", part)
			}
			if strings.HasPrefix(part, ".bitrise/") || strings.HasPrefix(part, ".bitrise\\") || part == ".bitrise" {
				return fmt.Errorf("changed files include CI config path %q — refusing to auto-commit", part)
			}
		}
	}
	return nil
}
