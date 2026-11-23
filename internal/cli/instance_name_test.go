package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sylvinus/agent-vm/internal/cage"
)

// A Cage profile has one shared environment. Its VM is the one the workbench
// manages, named after the profile's environment directory, so the CLI has to
// resolve the same name from any directory: the profile-level commands
// (`up`, `login`, `update-tools`) are run from wherever the user happens to be.
func TestCageProfileResolvesTheSharedEnvironmentFromAnyDirectory(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	environment := filepath.Join(root, "profile-downloads")
	project := filepath.Join(root, "a-project")
	state := filepath.Join(profile, "state")
	for _, dir := range []string{profile, environment, project, state} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	record, err := json.Marshal(map[string]string{"directory": environment})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(profile, "environment.json"), record, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(cage.ConfigEnv, filepath.Join(profile, "config.json"))
	t.Setenv("AGENT_VM_STATE_DIR", state)
	t.Setenv("AGENT_VM_LIMA_HOME", filepath.Join(state, "lima"))
	want := cage.InstanceName(profile, environment)
	for _, dir := range []string{project, environment, root} {
		var out, stderr bytes.Buffer
		if code := Main([]string{"name", dir}, IO{Stdout: &out, Stderr: &stderr}); code != 0 {
			t.Fatalf("name %s: %s", dir, stderr.String())
		}
		if got := strings.TrimSpace(out.String()); got != want {
			t.Fatalf("name %s resolved %q, want the shared environment %q", dir, got, want)
		}
	}
}