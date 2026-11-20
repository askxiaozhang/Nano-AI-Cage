package workbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/diagnostics"
	"github.com/sylvinus/agent-vm/internal/usererror"
)

type aiDesktopConfig struct {
	Pinned []string             `json:"pinned,omitempty"`
	Custom []cage.AIDesktopSpec `json:"custom,omitempty"`
}

type aiDesktopRuntimeState struct {
	Installed     bool   `json:"installed"`
	Version       string `json:"version"`
	Running       bool   `json:"running"`
	Failed        bool   `json:"failed"`
	FailureReason string `json:"failure_reason,omitempty"`
}

type aiDesktopView struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Homepage        string   `json:"homepage,omitempty"`
	Category        string   `json:"category,omitempty"`
	InstallType     string   `json:"install_type"`
	DownloadURL     string   `json:"download_url,omitempty"`
	GitHubRepo      string   `json:"github_repo,omitempty"`
	ExecCommand     string   `json:"exec_command,omitempty"`
	SupportedArches []string `json:"supported_arches"`
	ArchSupported   bool     `json:"arch_supported"`
	DisplayNumber   int      `json:"display_number"`
	Custom          bool     `json:"custom,omitempty"`
	Pinned          bool     `json:"pinned"`
	Installed       bool     `json:"installed"`
	Version         string   `json:"version"`
	Running         bool     `json:"running"`
	Failed          bool     `json:"failed"`
	FailureReason   string   `json:"failure_reason,omitempty"`
}

type aiDesktopGuestSnapshot struct {
	Arch string                           `json:"arch"`
	Apps map[string]aiDesktopRuntimeState `json:"apps"`
}

const aiDesktopSnapshotScript = `python3 - <<'CAGE_AI_DESKTOPS_STATE'
import json, subprocess
from pathlib import Path

def run(*args):
    return subprocess.run(args, capture_output=True, text=True).stdout.strip()

arch = run('dpkg', '--print-architecture')
apps = {}
meta_dir = Path('/etc/claude-cage/ai-desktops')
if meta_dir.is_dir():
    for path in sorted(meta_dir.glob('*.json')):
        try:
            meta = json.loads(path.read_text(encoding='utf-8'))
        except Exception:
            continue
        app_id = meta.get('id') or path.stem
        kind = meta.get('kind', '')
        pkg = meta.get('package', '')
        app_dir = meta.get('app_dir', '')
        installed = False
        version = meta.get('version', '')
        if kind == 'deb' and pkg:
            out = run('dpkg-query', '-W', '-f=${Status}\n${Version}', pkg).splitlines()
            if out and out[0] == 'install ok installed':
                installed = True
                if len(out) > 1 and out[1]:
                    version = out[1]
        elif kind == 'appimage' and app_dir:
            installed = Path(app_dir).is_dir()
        unit = f'cage-ai-desktop-{app_id}.service'
        state = run('systemctl', 'is-active', unit)
        result = run('systemctl', 'show', '--property=Result', '--value', unit) if state == 'failed' else ''
        apps[app_id] = {
            'installed': installed,
            'version': version if installed else '',
            'running': state == 'active',
            'failed': state == 'failed',
            'failure_reason': 'oom-kill' if result == 'oom-kill' else '',
        }

print(json.dumps({'arch': arch, 'apps': apps}))
CAGE_AI_DESKTOPS_STATE`

func (s *Server) aiDesktopConfigPath() string {
	return filepath.Join(s.profile, "ai-desktops.json")
}

func (s *Server) loadAIDesktopConfigLocked() aiDesktopConfig {
	var cfg aiDesktopConfig
	b, err := os.ReadFile(s.aiDesktopConfigPath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(b, &cfg)
	return cfg
}

func (s *Server) saveAIDesktopConfigLocked(cfg aiDesktopConfig) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(s.aiDesktopConfigPath(), string(b)+"\n")
}

func (s *Server) allAIDesktopsLocked(cfg aiDesktopConfig) []cage.AIDesktopSpec {
	builtins := cage.BuiltinAIDesktops()
	all := make([]cage.AIDesktopSpec, 0, len(builtins)+len(cfg.Custom))
	all = append(all, builtins...)
	for _, c := range cfg.Custom {
		c.Custom = true
		all = append(all, c)
	}
	return all
}

func (s *Server) findAIDesktop(id string) (cage.AIDesktopSpec, bool) {
	if !cage.ValidAIDesktopID(id) {
		return cage.AIDesktopSpec{}, false
	}
	s.aiDesktopsMu.Lock()
	cfg := s.loadAIDesktopConfigLocked()
	all := s.allAIDesktopsLocked(cfg)
	s.aiDesktopsMu.Unlock()
	for _, app := range all {
		if app.ID == id {
			return app, true
		}
	}
	return cage.AIDesktopSpec{}, false
}

func (s *Server) readAIDesktopSnapshot(ctx context.Context) (aiDesktopGuestSnapshot, error) {
	var snap aiDesktopGuestSnapshot
	if !s.vmRunning() {
		return snap, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	b, err := s.guestQuick(ctx, aiDesktopSnapshotScript)
	if err != nil {
		return snap, err
	}
	if err := json.Unmarshal(b, &snap); err != nil {
		return snap, fmt.Errorf("invalid guest snapshot: %w", err)
	}
	return snap, nil
}

func (s *Server) buildAIDesktopViews(cfg aiDesktopConfig, snap aiDesktopGuestSnapshot) []aiDesktopView {
	pinnedSet := map[string]bool{}
	for _, id := range cfg.Pinned {
		pinnedSet[id] = true
	}
	all := s.allAIDesktopsLocked(cfg)
	views := make([]aiDesktopView, 0, len(all))
	for _, app := range all {
		rt := snap.Apps[app.ID]
		archSupported := true
		if snap.Arch != "" && len(app.SupportedArches) > 0 {
			archSupported = slices.Contains(app.SupportedArches, snap.Arch)
		}
		views = append(views, aiDesktopView{
			ID:              app.ID,
			Name:            app.Name,
			Description:     app.Description,
			Homepage:        app.Homepage,
			Category:        app.Category,
			InstallType:     app.InstallType,
			DownloadURL:     app.DownloadURL,
			GitHubRepo:      app.GitHubRepo,
			ExecCommand:     app.ExecCommand,
			SupportedArches: app.SupportedArches,
			ArchSupported:   archSupported,
			DisplayNumber:   app.DisplayNumber,
			Custom:          app.Custom,
			Pinned:          pinnedSet[app.ID],
			Installed:       rt.Installed,
			Version:         rt.Version,
			Running:         rt.Running,
			Failed:          rt.Failed,
			FailureReason:   rt.FailureReason,
		})
	}
	return views
}

func (s *Server) listAIDesktops(w http.ResponseWriter, r *http.Request) {
	s.aiDesktopsMu.Lock()
	cfg := s.loadAIDesktopConfigLocked()
	s.aiDesktopsMu.Unlock()

	snap, err := s.readAIDesktopSnapshot(r.Context())
	if err != nil {
		_ = diagnostics.New(s.profile).Record("warn", "ai-desktops", fmt.Sprintf("read state: %v", err))
		// Still return the catalog even if guest status read temporarily fails.
		snap = aiDesktopGuestSnapshot{}
	}
	jsonReply(w, map[string]any{
		"arch":       snap.Arch,
		"vm_running": s.vmRunning(),
		"apps":       s.buildAIDesktopViews(cfg, snap),
	})
}

func (s *Server) aiDesktopStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, ok := s.findAIDesktop(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !s.vmRunning() {
		http.Error(w, "请先启动 Cage 环境", http.StatusConflict)
		return
	}
	snap, err := s.readAIDesktopSnapshot(r.Context())
	if err != nil {
		http.Error(w, usererror.Wrap("读取 "+app.Name+" 状态", err, "请确认环境仍在运行；重新连接后再打开应用。").Error(), http.StatusBadGateway)
		return
	}
	s.aiDesktopsMu.Lock()
	cfg := s.loadAIDesktopConfigLocked()
	s.aiDesktopsMu.Unlock()
	for _, view := range s.buildAIDesktopViews(cfg, snap) {
		if view.ID == id {
			jsonReply(w, view)
			return
		}
	}
	http.NotFound(w, r)
}

var slugSanitizer = regexp.MustCompile(`[^a-z0-9]+`)

func makeCustomAppID(name string, existing map[string]bool) string {
	base := strings.Trim(slugSanitizer.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if base == "" {
		base = "ai-app"
	}
	if len(base) > 24 {
		base = strings.TrimRight(base[:24], "-")
	}
	id := "custom-" + base
	if len(id) > 32 {
		id = strings.TrimRight(id[:32], "-")
	}
	if !existing[id] && cage.ValidAIDesktopID(id) {
		return id
	}
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d", id[:min(len(id), 28)], i)
		if !existing[candidate] && cage.ValidAIDesktopID(candidate) {
			return candidate
		}
	}
	return "custom-" + randomToken()[:8]
}

func (s *Server) addCustomAIDesktop(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Source      string `json:"source"`       // HTTPS URL (.deb / .AppImage) or GitHub "owner/repo"
		InstallType string `json:"install_type"` // optional: "deb", "appimage", "github-deb"
		ExecCommand string `json:"exec_command"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	name := strings.TrimSpace(input.Name)
	source := strings.TrimSpace(input.Source)
	if name == "" || len(name) > 60 {
		http.Error(w, "请输入有效的应用名称（1-60 个字符）", http.StatusBadRequest)
		return
	}
	if source == "" {
		http.Error(w, "请输入下载链接（.deb / .AppImage）或 GitHub 仓库（owner/repo）", http.StatusBadRequest)
		return
	}

	spec := cage.AIDesktopSpec{
		Name:            name,
		Description:     strings.TrimSpace(input.Description),
		Category:        "custom",
		ExecCommand:     strings.TrimSpace(input.ExecCommand),
		Electron:        true,
		SupportedArches: []string{"amd64", "arm64"},
		Custom:          true,
	}
	if spec.Description == "" {
		spec.Description = "用户自定义的 Linux AI Desktop 应用。"
	}

	if strings.HasPrefix(source, "https://") {
		parsed, err := url.Parse(source)
		if err != nil || parsed.Host == "" {
			http.Error(w, "下载链接格式无效，仅支持 HTTPS 链接", http.StatusBadRequest)
			return
		}
		spec.DownloadURL = source
		spec.Homepage = source
		lower := strings.ToLower(parsed.Path)
		switch {
		case input.InstallType == "appimage" || strings.HasSuffix(lower, ".appimage"):
			spec.InstallType = "appimage"
		default:
			spec.InstallType = "deb"
		}
	} else if parts := strings.Split(source, "/"); len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.Contains(source, " ") {
		spec.InstallType = "github-deb"
		spec.GitHubRepo = source
		spec.Homepage = "https://github.com/" + source + "/releases"
	} else {
		http.Error(w, "请提供 HTTPS 下载链接（.deb 或 .AppImage），或 GitHub 仓库路径（如 owner/repo）", http.StatusBadRequest)
		return
	}

	s.aiDesktopsMu.Lock()
	defer s.aiDesktopsMu.Unlock()
	cfg := s.loadAIDesktopConfigLocked()
	all := s.allAIDesktopsLocked(cfg)
	existingIDs := map[string]bool{}
	usedDisplays := map[int]bool{}
	for _, a := range all {
		existingIDs[a.ID] = true
		usedDisplays[a.DisplayNumber] = true
	}
	displayNum := 0
	for d := 87; d <= 99; d++ {
		if !usedDisplays[d] {
			displayNum = d
			break
		}
	}
	if displayNum == 0 {
		http.Error(w, "自定义 AI Desktop 数量已达上限（最多 13 个）", http.StatusConflict)
		return
	}
	spec.ID = makeCustomAppID(name, existingIDs)
	spec.DisplayNumber = displayNum
	cfg.Custom = append(cfg.Custom, spec)
	if !slices.Contains(cfg.Pinned, spec.ID) {
		cfg.Pinned = append(cfg.Pinned, spec.ID)
	}
	if err := s.saveAIDesktopConfigLocked(cfg); err != nil {
		requestError(w, err)
		return
	}
	jsonReply(w, spec)
}

func (s *Server) deleteCustomAIDesktop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !cage.ValidAIDesktopID(id) {
		http.NotFound(w, r)
		return
	}
	s.aiDesktopsMu.Lock()
	defer s.aiDesktopsMu.Unlock()
	cfg := s.loadAIDesktopConfigLocked()
	idx := -1
	for i, c := range cfg.Custom {
		if c.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		http.NotFound(w, r)
		return
	}
	cfg.Custom = append(cfg.Custom[:idx], cfg.Custom[idx+1:]...)
	cfg.Pinned = slices.DeleteFunc(cfg.Pinned, func(p string) bool { return p == id })
	if err := s.saveAIDesktopConfigLocked(cfg); err != nil {
		requestError(w, err)
		return
	}
	jsonReply(w, map[string]bool{"ok": true})
}

func (s *Server) pinAIDesktop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.findAIDesktop(id); !ok {
		http.NotFound(w, r)
		return
	}
	var input struct {
		Pinned bool `json:"pinned"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	s.aiDesktopsMu.Lock()
	defer s.aiDesktopsMu.Unlock()
	cfg := s.loadAIDesktopConfigLocked()
	cfg.Pinned = slices.DeleteFunc(cfg.Pinned, func(p string) bool { return p == id })
	if input.Pinned {
		cfg.Pinned = append(cfg.Pinned, id)
	}
	if err := s.saveAIDesktopConfigLocked(cfg); err != nil {
		requestError(w, err)
		return
	}
	jsonReply(w, map[string]bool{"pinned": input.Pinned})
}

func (s *Server) aiDesktopAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	action := r.PathValue("action")
	app, ok := s.findAIDesktop(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if action != "install" && action != "update" && action != "start" && action != "stop" && action != "uninstall" {
		http.NotFound(w, r)
		return
	}
	var input struct{}
	if !readJSON(w, r, &input) {
		return
	}
	if !s.vmRunning() {
		http.Error(w, "请先启动 Cage 环境", http.StatusConflict)
		return
	}
	unit := fmt.Sprintf("cage-ai-desktop-%s.service", app.ID)
	s.launch(w, "ai-desktop-"+app.ID+"-"+action, func() error {
		switch action {
		case "install", "update":
			s.mu.Lock()
			allowed := s.toolsNetworkAllowedLocked()
			s.mu.Unlock()
			if !allowed {
				return errors.New("工作台正在退出，请稍后重试安装")
			}
			monitorContext, stopMonitor := context.WithCancel(s.operationContext())
			done := make(chan struct{})
			go func() {
				defer close(done)
				ticker := time.NewTicker(10 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-monitorContext.Done():
						return
					case <-ticker.C:
						s.monitorSafety()
					}
				}
			}()
			defer func() {
				stopMonitor()
				s.mu.Lock()
				if s.monitorCancel != nil {
					s.monitorCancel()
				}
				s.mu.Unlock()
				<-done
			}()
			s.setMessage(fmt.Sprintf("正在下载并安装 %s，可在操作详情查看进度…", app.Name))
			script := cage.AIDesktopInstallScript(app)
			if err := RunMaintenanceScript(s.operationContext(), s.configPath, script, s.jobs.execution(), s.jobs.execution()); err != nil {
				return err
			}
			// Auto-pin installed apps to the left sidebar so the user can launch them immediately.
			s.aiDesktopsMu.Lock()
			cfg := s.loadAIDesktopConfigLocked()
			if !slices.Contains(cfg.Pinned, app.ID) {
				cfg.Pinned = append(cfg.Pinned, app.ID)
				_ = s.saveAIDesktopConfigLocked(cfg)
			}
			s.aiDesktopsMu.Unlock()
			return nil
		case "start":
			return s.startAIDesktop(app)
		case "stop":
			s.setMessage(fmt.Sprintf("正在关闭 %s…", app.Name))
			return s.runDesktopScript("sudo systemctl stop "+unit, 30*time.Second)
		case "uninstall":
			s.setMessage(fmt.Sprintf("正在卸载 %s…", app.Name))
			return s.runDesktopScript(cage.AIDesktopUninstallScript(app), 120*time.Second)
		default:
			return nil
		}
	})
}

func (s *Server) startAIDesktop(app cage.AIDesktopSpec) error {
	s.setMessage(fmt.Sprintf("正在打开 %s…", app.Name))
	key, err := s.account.desktopKey()
	if err != nil {
		return err
	}
	defer clear(key)
	script := fmt.Sprintf("test -r /etc/claude-cage/ai-desktops/%s.json || { echo '请先安装 %s。'; exit 1; }\n", app.ID, app.Name) +
		cage.AIDesktopProvisionScript(app)
	if err = s.runDesktopScript(script, 25*time.Second); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(s.operationContext(), 65*time.Second)
	defer cancel()
	cmd := s.sshCommand(ctx, "-T", s.sshTarget(), "--", "/usr/local/bin/cage-ai-desktop-start", app.ID)
	cmd.Stdin = bytes.NewReader(key)
	cmd.Stdout, cmd.Stderr = s.jobs.execution(), s.jobs.execution()
	if err = cmd.Run(); err != nil {
		return executionFailure("打开 "+app.Name, errors.Join(ctx.Err(), err), s.jobs.snapshot().Log)
	}
	return nil
}

func (s *Server) aiDesktopCredentials(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, ok := s.findAIDesktop(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	unit := fmt.Sprintf("cage-ai-desktop-%s.service", app.ID)
	runtimeDir := fmt.Sprintf("/run/cage-ai-desktop-%s", app.ID)
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	b, err := s.guestQuick(ctx, fmt.Sprintf(`set -eu
systemctl is-active --quiet %s
password=$(cat "$HOME/.config/claude-cage/ai-desktop-%s-password")
printf '%%s\n' "$password"
if test -f %s/base-display-scale; then cat %s/base-display-scale
elif test -f %s/display-scale; then cat %s/display-scale
else echo; fi
if test -f %s/xsettings.pid; then echo dynamic; fi`, unit, app.ID, runtimeDir, runtimeDir, runtimeDir, runtimeDir, runtimeDir))
	password, scale, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
	if err != nil || len(password) != 8 {
		http.Error(w, app.Name+" 尚未启动", http.StatusConflict)
		return
	}
	scale, dynamic, _ := strings.Cut(scale, "\n")
	density := 2.0
	if scale == "1.5" {
		density = 1.5
	}
	jsonReply(w, map[string]any{"password": password, "display_scale": density, "dynamic_scale": dynamic == "dynamic"})
}

func (s *Server) aiDesktopResize(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, ok := s.findAIDesktop(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var input struct {
		Width  int     `json:"width"`
		Height int     `json:"height"`
		Scale  float64 `json:"scale"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	if input.Width < 320 || input.Height < 240 || input.Width > 8192 || input.Height > 8192 || !validDisplayDensity(input.Scale) {
		http.Error(w, "画面尺寸无效", http.StatusBadRequest)
		return
	}
	scale := ""
	if input.Scale != 0 {
		scale = strconv.FormatFloat(input.Scale, 'f', -1, 64)
	}
	script := aiDesktopResizeScript(app.ID, app.DisplayNumber, input.Width, input.Height, scale)
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	output, err := s.guestQuick(ctx, script)
	if err != nil {
		http.Error(w, app.Name+" 画面尺寸未能调整", http.StatusBadGateway)
		return
	}
	reply := map[string]any{"ok": true}
	if applied, _, _ := strings.Cut(string(output), "\n"); scale != "" && strings.TrimSpace(applied) == "scale "+scale {
		reply["scale"] = input.Scale
	}
	jsonReply(w, reply)
}

func aiDesktopResizeScript(appID string, displayNum, wid, hei int, scale string) string {
	runtimeDir := fmt.Sprintf("/run/cage-ai-desktop-%s", appID)
	return fmt.Sprintf(`set -eu
export DISPLAY=:%d XAUTHORITY=%s/Xauthority
exec 9>%s/resize.lock
flock 9
requested=%s
manager=
if test -r %s/xsettings.pid; then
  manager=$(cat %s/xsettings.pid)
  if test "$(cat "/proc/$manager/comm" 2>/dev/null)" != xsettingsd; then manager=; fi
fi
scale=
if test -n "$manager"; then
  scale=${requested:-$(cat %s/base-display-scale 2>/dev/null || echo 2)}
  if test -n "$requested"; then echo "scale $scale"; fi
fi
printf '%%s %%s %%s\n' %d %d "$scale" > %s/desktop-size
if test -n "$manager" && test "$(cat %s/display-scale 2>/dev/null)" != "$scale"; then
  dpi=$(awk -v scale="$scale" 'BEGIN {printf "%%.0f", scale * 96 * 1024}')
  { printf 'Xft/DPI %%s\nGdk/WindowScalingFactor 1\nGdk/UnscaledDPI %%s\n' "$dpi" "$dpi"
    cat %s/xsettings.fixed 2>/dev/null || printf 'Gtk/DecorationLayout ":"\n'; } > %s/xsettings.conf.new
  mv %s/xsettings.conf.new %s/xsettings.conf
  kill -HUP "$manager"
  printf '%%s\n' "$scale" > %s/display-scale
fi
name=cage-%dx%d
if ! xrandr --query | grep -q " $name "; then
  xrandr --newmode "$name" %.3f %d %d %d %d %d %d %d %d +HSync -VSync
  xrandr --addmode VNC-0 "$name"
fi
xrandr --output VNC-0 --mode "$name"
for old in $(xrandr --query | awk '/^ +cage-/ {print $1}'); do
  if [ "$old" != "$name" ]; then xrandr --delmode VNC-0 "$old"; xrandr --rmmode "$old"; fi
done`,
		displayNum, runtimeDir, runtimeDir, scale,
		runtimeDir, runtimeDir, runtimeDir,
		wid, hei, runtimeDir,
		runtimeDir, runtimeDir, runtimeDir, runtimeDir, runtimeDir, runtimeDir,
		wid, hei, float64((wid+32)*(hei+8)*60)/1000000, wid, wid+8, wid+16, wid+32, hei, hei+1, hei+2, hei+8)
}

func (s *Server) aiDesktopVNC(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, ok := s.findAIDesktop(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !s.vmRunning() {
		http.Error(w, "环境尚未启动", http.StatusConflict)
		return
	}
	s.streamDesktopSocket(w, r, fmt.Sprintf("/run/cage-ai-desktop-%s/vnc.sock", app.ID))
}
