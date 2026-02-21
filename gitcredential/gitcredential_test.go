package gitcredential

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteHelper(t *testing.T) {
	const username = "testuser"
	const token = "s3cr3t-t0ken"

	helper, err := WriteHelper(username, token)
	require.NoError(t, err)
	defer os.Remove(helper.Path)

	// Script file exists and is executable by owner
	info, err := os.Stat(helper.Path)
	require.NoError(t, err, "helper script not found")
	assert.NotZero(t, info.Mode()&0100, "helper script is not executable")

	// Script must not contain credentials directly — it should reference env vars.
	// If this fails it means the design regressed back to embedding secrets in the file.
	content, err := os.ReadFile(helper.Path)
	require.NoError(t, err)
	assert.NotContains(t, string(content), username, "helper script contains username directly; expected an env var reference")
	assert.NotContains(t, string(content), token, "helper script contains token directly; expected an env var reference")

	// When executed with the provided env vars the script must output the credentials
	// in the format git's credential helper protocol expects.
	cmd := exec.Command(helper.Path)
	cmd.Env = helper.Env
	out, err := cmd.Output()
	require.NoError(t, err, "execute helper script")

	assert.Contains(t, string(out), "username="+username)
	assert.Contains(t, string(out), "password="+token)
}

// TestWriteHelper_tokenOnly covers GitHub App installations, which provide a
// short-lived token with no username (GIT_HTTP_USERNAME is not set by Bitrise
// for GitHub App builds). The helper must fall back to "x-access-token" — the
// username GitHub requires for HTTPS git operations with an App installation token
// (documented at https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/authenticating-as-a-github-app-installation).
func TestWriteHelper_tokenOnly(t *testing.T) {
	const token = "ghs_short-lived-app-token"

	helper, err := WriteHelper("", token)
	require.NoError(t, err)
	defer os.Remove(helper.Path)

	cmd := exec.Command(helper.Path)
	cmd.Env = helper.Env
	out, err := cmd.Output()
	require.NoError(t, err, "execute helper script")

	assert.Contains(t, string(out), "username="+gitHubAppUsername)
	assert.Contains(t, string(out), "password="+token)
}
