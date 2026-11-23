package cage

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAntigravityDownloadSelectsIDE(t *testing.T) {
	script := AIDesktopInstallScript(AIDesktopSpec{ID: "antigravity", InstallType: "antigravity-tarball"})
	_, script, ok := strings.Cut(script, "<<'CAGE_AG_URL'\n")
	if !ok {
		t.Fatal("missing URL resolver")
	}
	resolver, _, ok := strings.Cut(script, "\nCAGE_AG_URL")
	if !ok {
		t.Fatal("missing URL resolver end")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "download.html")
	html := `<a href="https://storage.googleapis.com/antigravity-public/antigravity-hub/1/linux-x64/Antigravity.tar.gz">App</a><a href="https://edgedl.me.gvt1.com/edgedl/release2/a/antigravity/stable/2/linux-x64/Antigravity%20IDE.tar.gz">IDE</a><a href="https://edgedl.me.gvt1.com/edgedl/release2/a/antigravity/stable/2/linux-arm/Antigravity%20IDE.tar.gz">IDE ARM</a>`
	if err := os.WriteFile(path, []byte(html), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ arch, want string }{{"amd64", "/linux-x64/Antigravity%20IDE.tar.gz"}, {"arm64", "/linux-arm/Antigravity%20IDE.tar.gz"}} {
		cmd := exec.Command("python3", "-c", resolver, path, tc.arch)
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.HasSuffix(strings.TrimSpace(string(out)), tc.want) {
			t.Errorf("%s: %s: %v", tc.arch, out, err)
		}
	}
}

func TestAntigravityDownloadPageForbidden(t *testing.T) {
	script := AIDesktopInstallScript(AIDesktopSpec{ID: "antigravity", InstallType: "antigravity-tarball"})
	start := strings.Index(script, "resolve_antigravity_url()")
	end := strings.Index(script, "mkdir -p \"$temporary/extracted\"")
	if start < 0 || end <= start {
		t.Fatal("missing Antigravity download steps")
	}
	download := `set -eu
temporary=$1
ARCH=$2
cage_curl_download() {
  case "$1" in
    "$temporary/download.html") return 22 ;;
    "$temporary/antigravity.tar.gz") printf '%s\n' "$@" > "$temporary/args" ;;
    *) return 1 ;;
  esac
}
` + script[start:end]
	for _, tc := range []struct{ arch, platform string }{{"amd64", "linux-x64"}, {"arm64", "linux-arm"}} {
		t.Run(tc.arch, func(t *testing.T) {
			dir := t.TempDir()
			if out, err := exec.Command("bash", "-c", download, "bash", dir, tc.arch).CombinedOutput(); err != nil {
				t.Fatalf("download after HTTP 403: %v\n%s", err, out)
			}
			args, err := os.ReadFile(filepath.Join(dir, "args"))
			if err != nil {
				t.Fatal(err)
			}
			for _, arg := range strings.Split(string(args), "\n") {
				if strings.HasPrefix(arg, "https://edgedl.me.gvt1.com/edgedl/release2/") &&
					strings.HasSuffix(arg, "/"+tc.platform+"/Antigravity%20IDE.tar.gz") {
					return
				}
			}
			t.Fatalf("missing official %s IDE download in %s", tc.arch, args)
		})
	}
}

func TestAntigravitySelectsDesktopExecutable(t *testing.T) {
	script := AIDesktopInstallScript(AIDesktopSpec{ID: "antigravity", InstallType: "antigravity-tarball"})
	start := strings.Index(script, "app_root=\"$temporary/extracted\"")
	if start < 0 {
		t.Fatal("missing archive layout detection")
	}
	selectExec, _, ok := strings.Cut(script[start:], "\nsudo install -d")
	if !ok {
		t.Fatal("missing end of executable selection")
	}
	for _, tc := range []struct{ layout, name string }{{"", "antigravity"}, {"Antigravity IDE", "antigravity-ide"}, {"cli-only", ""}} {
		t.Run(tc.layout+tc.name, func(t *testing.T) {
			dir := t.TempDir()
			root := filepath.Join(dir, "extracted", tc.layout)
			if err := os.MkdirAll(filepath.Join(root, "bin"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "bin", "antigravity-ide"), nil, 0755); err != nil {
				t.Fatal(err)
			}
			if tc.name != "" {
				if err := os.WriteFile(filepath.Join(root, tc.name), nil, 0755); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("bash", "-c", "set -eu\ntemporary=$1\nAPP_DIR=/app\n"+selectExec+"\necho \"$EXEC_CMD\"", "bash", dir)
			out, err := cmd.CombinedOutput()
			if tc.name == "" {
				if err == nil {
					t.Fatal("accepted a CLI-only archive")
				}
			} else if err != nil || strings.TrimSpace(string(out)) != "/app/"+tc.name {
				t.Fatalf("unexpected executable: %s (%v)", out, err)
			}
		})
	}
}
