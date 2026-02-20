package gitcredential

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestWriteHelper(t *testing.T) {
	const username = "testuser"
	const token = "s3cr3t-t0ken"

	helper, err := WriteHelper(username, token)
	if err != nil {
		t.Fatalf("WriteHelper() error = %v", err)
	}
	defer os.Remove(helper.Path)

	// Script file exists and is executable by owner
	info, err := os.Stat(helper.Path)
	if err != nil {
		t.Fatalf("helper script not found: %v", err)
	}
	if info.Mode()&0100 == 0 {
		t.Errorf("helper script is not executable: mode = %v", info.Mode())
	}

	// Script must not contain credentials directly — it should reference env vars.
	// If this fails it means the design regressed back to embedding secrets in the file.
	content, err := os.ReadFile(helper.Path)
	if err != nil {
		t.Fatalf("read helper script: %v", err)
	}
	if strings.Contains(string(content), username) {
		t.Error("helper script contains username directly; expected an env var reference")
	}
	if strings.Contains(string(content), token) {
		t.Error("helper script contains token directly; expected an env var reference")
	}

	// When executed with the provided env vars the script must output the credentials
	// in the format git's credential helper protocol expects.
	cmd := exec.Command(helper.Path)
	cmd.Env = helper.Env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("execute helper script: %v", err)
	}

	output := string(out)
	if !strings.Contains(output, "username="+username) {
		t.Errorf("script output %q missing username=%s", output, username)
	}
	if !strings.Contains(output, "password="+token) {
		t.Errorf("script output %q missing password=%s", output, token)
	}
}
