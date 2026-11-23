package workbench

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"al.essio.dev/pkg/shellescape"
	"github.com/sylvinus/agent-vm/internal/diagnostics"
	"github.com/sylvinus/agent-vm/internal/managedproxy"
)

//go:embed linux-google-session.sh
var linuxGoogleSession string

type googleAccount struct {
	Number      uint32 `json:"number"`
	ID          string `json:"id"`
	Label       string `json:"label"`
	Email       string `json:"email,omitempty"`
	ProxyNodeID string `json:"proxy_node_id,omitempty"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   int64  `json:"created_at,omitempty"`
}

type googleAccountList struct {
	LastNumber uint32          `json:"last_number,omitempty"`
	Accounts   []googleAccount `json:"accounts"`
}

type googleAccountRuntime struct {
	AntigravityRunning bool   `json:"antigravity_running"`
	BrowserRunning     bool   `json:"browser_running"`
	DetectedEmail      string `json:"detected_email,omitempty"`
	HasProfile         bool   `json:"has_profile"`
}

type googleAccountsGuestSnapshot struct {
	AntigravityInstalled bool                            `json:"antigravity_installed"`
	Accounts             map[string]googleAccountRuntime `json:"accounts"`
}

type googleAccountView struct {
	Number             uint32 `json:"number"`
	ID                 string `json:"id"`
	Label              string `json:"label"`
	Email              string `json:"email,omitempty"`
	DetectedEmail      string `json:"detected_email,omitempty"`
	ProxyNodeID        string `json:"proxy_node_id,omitempty"`
	Enabled            bool   `json:"enabled"`
	CreatedAt          int64  `json:"created_at,omitempty"`
	AntigravityRunning bool   `json:"antigravity_running"`
	BrowserRunning     bool   `json:"browser_running"`
	HasProfile         bool   `json:"has_profile"`
}

const googleAccountsSnapshotScript = `python3 - <<'CAGE_GOOGLE_ACCOUNTS_STATE'
import json, re, subprocess
from pathlib import Path

def run(*args):
    return subprocess.run(args, capture_output=True, text=True).stdout.strip()

ag_installed = False
ag_meta = Path('/etc/claude-cage/ai-desktops/antigravity.json')
if ag_meta.is_file():
    try:
        meta = json.loads(ag_meta.read_text(encoding='utf-8'))
        kind = meta.get('kind', '')
        pkg = meta.get('package', '')
        app_dir = meta.get('app_dir', '')
        if kind == 'deb' and pkg:
            out = run('dpkg-query', '-W', '-f=${Status}', pkg)
            ag_installed = out == 'install ok installed'
        elif app_dir:
            ag_installed = Path(app_dir).is_dir()
    except Exception:
        pass
if not ag_installed and Path('/opt/cage-ai-desktops/antigravity').is_dir():
    ag_installed = True

email_re = re.compile(r'[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}')

def detect_email(base: Path) -> str:
    # 1) Chromium profile Preferences -> account_info[0].email
    for pref in [base / 'chromium-data/Default/Preferences', base / 'home/.config/chromium/Default/Preferences']:
        if pref.is_file():
            try:
                data = json.loads(pref.read_text(encoding='utf-8', errors='ignore'))
                for info in data.get('account_info') or []:
                    if isinstance(info, dict) and info.get('email'):
                        return str(info['email']).strip().lower()
            except Exception:
                pass
    # 2) Gemini / Antigravity local config files
    for candidate in [
        base / 'home/.gemini/google_accounts.json',
        base / 'home/.gemini/antigravity-cli/settings.json',
        base / 'home/.gemini/config/config.json',
        base / 'antigravity-data/User/globalStorage/storage.json',
    ]:
        if candidate.is_file():
            try:
                raw = candidate.read_text(encoding='utf-8', errors='ignore')
                m = email_re.search(raw)
                if m:
                    return m.group(0).lower()
            except Exception:
                pass
    return ''

accounts = {}
root = Path.home() / '.local/share/cage/google-accounts'
ids = set()
if root.is_dir():
    for child in root.iterdir():
        if child.is_dir() and re.fullmatch(r'[0-9a-f]{24}', child.name):
            ids.add(child.name)

# Also discover any active systemd units even if directories were created elsewhere.
units_out = run('systemctl', 'list-units', '--type=service', '--state=active', '--no-legend', '--no-pager', 'google-*')
for line in units_out.splitlines():
    parts = line.split()
    if not parts:
        continue
    name = parts[0]
    m = re.fullmatch(r'google-(?:antigravity|browser)-([0-9a-f]{24})\.service', name)
    if m:
        ids.add(m.group(1))

for aid in sorted(ids):
    base = root / aid
    ag_running = run('systemctl', 'is-active', f'google-antigravity-{aid}.service') == 'active'
    br_running = run('systemctl', 'is-active', f'google-browser-{aid}.service') == 'active'
    has_profile = (base / 'home').is_dir() or (base / 'chromium-data').is_dir() or (base / 'antigravity-data').is_dir()
    accounts[aid] = {
        'antigravity_running': ag_running,
        'browser_running': br_running,
        'detected_email': detect_email(base) if has_profile else '',
        'has_profile': has_profile,
    }

print(json.dumps({'antigravity_installed': ag_installed, 'accounts': accounts}))
CAGE_GOOGLE_ACCOUNTS_STATE`

func (s *Server) googleAccountsPath() string {
	return filepath.Join(s.profile, "google-accounts.json")
}

func (s *Server) loadGoogleAccountsLocked() (googleAccountList, error) {
	list := googleAccountList{Accounts: []googleAccount{}}
	b, err := os.ReadFile(s.googleAccountsPath())
	if os.IsNotExist(err) {
		return list, nil
	}
	if err != nil || json.Unmarshal(b, &list) != nil || list.Accounts == nil {
		return list, errors.New("Google 账号配置文件无法读取，请检查配置目录")
	}
	if err := list.assignNumbers(); err != nil {
		return list, err
	}
	return list, nil
}

func (list *googleAccountList) assignNumbers() error {
	seen := make(map[uint32]bool, len(list.Accounts))
	for _, a := range list.Accounts {
		if a.Number == 0 {
			continue
		}
		seen[a.Number] = true
		list.LastNumber = max(list.LastNumber, a.Number)
	}
	for i := range list.Accounts {
		if list.Accounts[i].Number == 0 {
			if list.LastNumber == math.MaxUint32 {
				return errors.New("Google 账号编号已用尽")
			}
			list.LastNumber++
			list.Accounts[i].Number = list.LastNumber
		}
	}
	return nil
}

func (list googleAccountList) find(id string) *googleAccount {
	for i := range list.Accounts {
		if list.Accounts[i].ID == id {
			return &list.Accounts[i]
		}
	}
	return nil
}

func (s *Server) saveGoogleAccountsLocked(list googleAccountList) error {
	if err := list.assignNumbers(); err != nil {
		return err
	}
	if err := writePrivate(s.googleAccountsPath(), list); err != nil {
		return errors.New("Google 账号配置保存失败")
	}
	return nil
}

func (s *Server) findGoogleAccount(id string) (googleAccount, bool) {
	if !validClaudeID(id) {
		return googleAccount{}, false
	}
	s.googleAccountsMu.Lock()
	defer s.googleAccountsMu.Unlock()
	list, err := s.loadGoogleAccountsLocked()
	if err != nil {
		return googleAccount{}, false
	}
	a := list.find(id)
	if a == nil {
		return googleAccount{}, false
	}
	return *a, true
}

func validGoogleLabel(label string) bool {
	return label != "" && utf8.RuneCountInString(label) <= 64 && strings.IndexFunc(label, unicode.IsControl) < 0
}

func (s *Server) readGoogleAccountsSnapshot(ctx context.Context) (googleAccountsGuestSnapshot, error) {
	var snap googleAccountsGuestSnapshot
	if !s.vmRunning() {
		return snap, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	b, err := s.guestQuick(ctx, googleAccountsSnapshotScript)
	if err != nil {
		return snap, err
	}
	if err := json.Unmarshal(b, &snap); err != nil {
		return snap, fmt.Errorf("invalid google accounts snapshot: %w", err)
	}
	return snap, nil
}

func (s *Server) listGoogleAccounts(w http.ResponseWriter, r *http.Request) {
	s.googleAccountsMu.Lock()
	list, err := s.loadGoogleAccountsLocked()
	s.googleAccountsMu.Unlock()
	if err != nil {
		requestError(w, err)
		return
	}
	snap, snapErr := s.readGoogleAccountsSnapshot(r.Context())
	if snapErr != nil {
		_ = diagnostics.New(s.profile).Record("warn", "google-accounts", fmt.Sprintf("read state: %v", snapErr))
		snap = googleAccountsGuestSnapshot{}
	}
	views := make([]googleAccountView, 0, len(list.Accounts))
	emailUpdated := false
	for i := range list.Accounts {
		a := &list.Accounts[i]
		rt := snap.Accounts[a.ID]
		if a.Email == "" && rt.DetectedEmail != "" {
			a.Email = rt.DetectedEmail
			emailUpdated = true
		}
		views = append(views, googleAccountView{
			Number:             a.Number,
			ID:                 a.ID,
			Label:              a.Label,
			Email:              a.Email,
			DetectedEmail:      rt.DetectedEmail,
			ProxyNodeID:        a.ProxyNodeID,
			Enabled:            a.Enabled,
			CreatedAt:          a.CreatedAt,
			AntigravityRunning: rt.AntigravityRunning,
			BrowserRunning:     rt.BrowserRunning,
			HasProfile:         rt.HasProfile,
		})
	}
	if emailUpdated {
		s.googleAccountsMu.Lock()
		if latest, err := s.loadGoogleAccountsLocked(); err == nil {
			for i := range latest.Accounts {
				if latest.Accounts[i].Email == "" {
					if rt := snap.Accounts[latest.Accounts[i].ID]; rt.DetectedEmail != "" {
						latest.Accounts[i].Email = rt.DetectedEmail
					}
				}
			}
			_ = s.saveGoogleAccountsLocked(latest)
		}
		s.googleAccountsMu.Unlock()
	}
	jsonReply(w, map[string]any{
		"supported":             s.accountDesktopsSupported(),
		"vm_running":            s.vmRunning(),
		"antigravity_installed": snap.AntigravityInstalled,
		"accounts":              views,
	})
}

func (s *Server) mutateGoogleAccounts(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Action      string  `json:"action"`
		ID          string  `json:"id,omitempty"`
		Label       string  `json:"label,omitempty"`
		Email       *string `json:"email,omitempty"`
		ProxyNodeID *string `json:"proxy_node_id,omitempty"`
		Enabled     *bool   `json:"enabled,omitempty"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	if !s.accountDesktopsSupported() {
		http.Error(w, "当前环境暂不支持 Google 多账号管理", http.StatusNotImplemented)
		return
	}
	s.googleAccountsMu.Lock()
	defer s.googleAccountsMu.Unlock()
	list, err := s.loadGoogleAccountsLocked()
	if err != nil {
		requestError(w, err)
		return
	}

	switch input.Action {
	case "create":
		node := ""
		if input.ProxyNodeID != nil {
			node = strings.TrimSpace(*input.ProxyNodeID)
			if node != "" {
				store, err := managedproxy.Load(s.profile)
				if err != nil {
					requestError(w, err)
					return
				}
				if _, ok := store.Find(node); !ok || !validClaudeID(node) {
					http.Error(w, "所选代理节点不可用，请刷新列表", http.StatusBadRequest)
					return
				}
			}
		}
		email := ""
		if input.Email != nil {
			email = strings.TrimSpace(*input.Email)
		}
		label := strings.TrimSpace(input.Label)
		id := randomToken()[:24]
		nextNum := list.LastNumber + 1
		if label == "" {
			if email != "" {
				label = email
			} else {
				label = fmt.Sprintf("Google #%02d", nextNum)
			}
		}
		if !validGoogleLabel(label) {
			http.Error(w, "备注最多 64 个字符，不能含控制字符", http.StatusBadRequest)
			return
		}
		acc := googleAccount{
			Number:      nextNum,
			ID:          id,
			Label:       label,
			Email:       email,
			ProxyNodeID: node,
			Enabled:     true,
			CreatedAt:   time.Now().Unix(),
		}
		list.LastNumber = nextNum
		list.Accounts = append(list.Accounts, acc)
		if err := s.saveGoogleAccountsLocked(list); err != nil {
			requestError(w, err)
			return
		}
		jsonReply(w, acc)

	case "update":
		a := list.find(input.ID)
		if a == nil {
			http.NotFound(w, r)
			return
		}
		if input.Label != "" {
			label := strings.TrimSpace(input.Label)
			if !validGoogleLabel(label) {
				http.Error(w, "备注最多 64 个字符，不能含控制字符", http.StatusBadRequest)
				return
			}
			a.Label = label
		}
		if input.Email != nil {
			a.Email = strings.TrimSpace(*input.Email)
		}
		if input.Enabled != nil {
			a.Enabled = *input.Enabled
		}
		if input.ProxyNodeID != nil {
			node := strings.TrimSpace(*input.ProxyNodeID)
			if node != a.ProxyNodeID {
				if node != "" {
					store, err := managedproxy.Load(s.profile)
					if err != nil {
						requestError(w, err)
						return
					}
					if _, ok := store.Find(node); !ok || !validClaudeID(node) {
						http.Error(w, "所选代理节点不可用，请刷新列表", http.StatusBadRequest)
						return
					}
				}
				if err := s.ensureGoogleAccountIdle(r.Context(), a.ID, "切换出口"); err != nil {
					http.Error(w, err.Error(), http.StatusConflict)
					return
				}
				a.ProxyNodeID = node
			}
		}
		if err := s.saveGoogleAccountsLocked(list); err != nil {
			requestError(w, err)
			return
		}
		jsonReply(w, *a)

	case "delete":
		idx := slices.IndexFunc(list.Accounts, func(a googleAccount) bool { return a.ID == input.ID })
		if idx < 0 {
			http.NotFound(w, r)
			return
		}
		if err := s.ensureGoogleAccountIdle(r.Context(), input.ID, "移除账号"); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		list.Accounts = append(list.Accounts[:idx], list.Accounts[idx+1:]...)
		if err := s.saveGoogleAccountsLocked(list); err != nil {
			requestError(w, err)
			return
		}
		jsonReply(w, map[string]bool{"ok": true})

	default:
		http.Error(w, "未知的 Google 账号操作", http.StatusBadRequest)
	}
}

func (s *Server) ensureGoogleAccountIdle(ctx context.Context, id, verb string) error {
	for _, t := range s.terminals.snapshot() {
		if t["google_account_id"] == id && t["closed"] == false {
			return fmt.Errorf("请先关闭该 Google 账号的终端，再%s", verb)
		}
	}
	if !s.vmRunning() {
		return nil
	}
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for _, mode := range []string{"antigravity", "browser"} {
		unit := fmt.Sprintf("google-%s-%s.service", mode, id)
		state, err := s.guestQuick(c, fmt.Sprintf("systemctl show --property=ActiveState --value %s", unit))
		if err != nil {
			return fmt.Errorf("无法确认账号运行状态，暂未%s", verb)
		}
		switch strings.TrimSpace(string(state)) {
		case "inactive", "failed", "":
		default:
			label := "Google Antigravity"
			if mode == "browser" {
				label = "独立浏览器"
			}
			return fmt.Errorf("请先关闭该账号的 %s，再%s", label, verb)
		}
	}
	return nil
}

func (s *Server) googleAccountSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	mode := r.PathValue("mode")
	action := r.PathValue("action")
	if !validClaudeID(id) || (mode != "antigravity" && mode != "browser") {
		http.NotFound(w, r)
		return
	}
	acc, ok := s.findGoogleAccount(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !s.accountDesktopsSupported() {
		http.Error(w, "当前环境暂不支持 Google 独立会话", http.StatusNotImplemented)
		return
	}
	if !s.vmRunning() {
		http.Error(w, "请先启动 Cage 环境", http.StatusConflict)
		return
	}

	unit := fmt.Sprintf("google-%s-%s.service", mode, id)
	runtimeDir := fmt.Sprintf("/run/google-%s-%s", mode, id)

	switch {
	case r.Method == http.MethodGet && action == "vnc":
		s.streamDesktopSocket(w, r, runtimeDir+"/vnc.sock")
		return

	case r.Method == http.MethodGet && action == "vnc-credentials":
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		b, err := s.guestQuick(ctx, fmt.Sprintf(`set -eu
systemctl is-active --quiet %s
cat %s/password
if test -f %s/base-display-scale; then cat %s/base-display-scale
elif test -f %s/display-scale; then cat %s/display-scale
else echo; fi
if test -f %s/xsettings.pid; then echo dynamic; fi`, unit, runtimeDir, runtimeDir, runtimeDir, runtimeDir, runtimeDir, runtimeDir))
		password, rest, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
		if err != nil || len(password) != 8 {
			http.Error(w, "该 Google 会话尚未启动", http.StatusConflict)
			return
		}
		scale, dynamic, _ := strings.Cut(rest, "\n")
		density := 2.0
		if scale == "1.5" {
			density = 1.5
		}
		w.Header().Set("Cache-Control", "no-store")
		jsonReply(w, map[string]any{"password": password, "display_scale": density, "dynamic_scale": dynamic == "dynamic"})
		return

	case r.Method == http.MethodGet && action == "status":
		snap, err := s.readGoogleAccountsSnapshot(r.Context())
		if err != nil {
			http.Error(w, "读取 Google 会话状态失败", http.StatusBadGateway)
			return
		}
		rt := snap.Accounts[id]
		running := rt.AntigravityRunning
		if mode == "browser" {
			running = rt.BrowserRunning
		}
		jsonReply(w, map[string]any{
			"id":        id,
			"mode":      mode,
			"name":      acc.Label,
			"email":     acc.Email,
			"installed": mode == "browser" || snap.AntigravityInstalled,
			"running":   running,
		})
		return

	case r.Method == http.MethodPost && action == "resize":
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
		script := googleSessionResizeScript(runtimeDir, input.Width, input.Height, scale)
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		output, err := s.guestQuick(ctx, script)
		if err != nil {
			http.Error(w, "Google 会话画面尺寸未能调整", http.StatusBadGateway)
			return
		}
		reply := map[string]any{"ok": true}
		if applied, _, _ := strings.Cut(string(output), "\n"); scale != "" && strings.TrimSpace(applied) == "scale "+scale {
			reply["scale"] = input.Scale
		}
		jsonReply(w, reply)
		return

	case r.Method == http.MethodPost && (action == "start" || action == "stop"):
		var input struct {
			URL string `json:"url,omitempty"`
		}
		if r.ContentLength != 0 && !readJSON(w, r, &input) {
			return
		}
		startURL := "https://gemini.google.com"
		if strings.HasPrefix(input.URL, "https://") {
			startURL = input.URL
		}
		s.launch(w, fmt.Sprintf("google-%s-%s-%s", mode, id, action), func() error {
			return s.changeGoogleSession(s.operationContext(), acc, mode, action, startURL)
		})
		return
	}

	http.NotFound(w, r)
}

func (s *Server) changeGoogleSession(parent context.Context, acc googleAccount, mode, action, startURL string) error {
	scope := "stop"
	var key []byte
	var err error
	if action == "start" {
		s.setMessage(fmt.Sprintf("正在打开 %s（%s）…", acc.Label, map[string]string{"antigravity": "Google Antigravity", "browser": "独立浏览器"}[mode]))
		scope = "shared"
		if acc.ProxyNodeID != "" {
			if err = s.ensureAccountNetwork(parent, acc.ID, acc.ProxyNodeID); err != nil {
				return err
			}
			scope = "node-" + acc.ID
		}
		key, err = s.account.desktopKey()
		if err != nil {
			return err
		}
		defer clear(key)
	} else {
		s.setMessage(fmt.Sprintf("正在关闭 %s…", acc.Label))
	}

	ctx, cancel := context.WithTimeout(parent, 100*time.Second)
	defer cancel()
	command := fmt.Sprintf("exec /bin/bash -c %s -- %s %s %s %s",
		shellescape.Quote(linuxGoogleSession), acc.ID, mode, scope, shellescape.Quote(startURL))
	cmd := s.sshCommand(ctx, "-T", s.sshTarget(), "--", command)
	cmd.Stdin = bytes.NewReader(key)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = "Google 独立会话操作失败，请检查环境状态"
		}
		return errors.New(msg)
	}
	return nil
}

func googleSessionResizeScript(runtimeDir string, wid, hei int, scale string) string {
	return fmt.Sprintf(`set -eu
test -s %s/display
export DISPLAY=":$(cat %s/display)" XAUTHORITY=%s/Xauthority
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
		runtimeDir, runtimeDir, runtimeDir, runtimeDir, scale,
		runtimeDir, runtimeDir, runtimeDir,
		wid, hei, runtimeDir,
		runtimeDir, runtimeDir, runtimeDir, runtimeDir, runtimeDir, runtimeDir,
		wid, hei, float64((wid+32)*(hei+8)*60)/1000000, wid, wid+8, wid+16, wid+32, hei, hei+1, hei+2, hei+8)
}

// googleTerminalScript prepares an isolated interactive shell bound to a Google account's
// dedicated HOME (~/.local/share/cage/google-accounts/<id>/home) so agy / gemini CLI
// credentials and config stay completely isolated per Google account.
func (s *Server) googleTerminalScript(ctx context.Context, workspace string, acc googleAccount) (string, error) {
	inner := fmt.Sprintf(`real_home="$HOME"
export CAGE_GOOGLE_ACCOUNT_ID=%s
export CAGE_GOOGLE_ACCOUNT_LABEL=%s
export HOME="$real_home/.local/share/cage/google-accounts/%s/home"
export XDG_CONFIG_HOME="$HOME/.config"
export XDG_DATA_HOME="$HOME/.local/share"
export XDG_CACHE_HOME="$HOME/.cache"
mkdir -p "$HOME" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME"
export PATH="$HOME/.local/bin:$real_home/.local/bin:$PATH"
cd %s
printf '\033[1;34m[Google 账号隔离终端]\033[0m %%s (HOME=%%s)\n' "$CAGE_GOOGLE_ACCOUNT_LABEL" "$HOME"
printf '提示：在此终端运行 agy 或 gemini CLI 将自动使用该 Google 账号的独立配置与出口。\n\n'
exec env TERM=xterm-256color COLORTERM=truecolor CAGE_WORKBENCH=1 bash -il`,
		shellescape.Quote(acc.ID), shellescape.Quote(acc.Label), acc.ID, shellescape.Quote(workspace))

	if acc.ProxyNodeID == "" {
		return inner, nil
	}
	if err := s.ensureAccountNetwork(ctx, acc.ID, acc.ProxyNodeID); err != nil {
		return "", err
	}
	opener := "/usr/local/lib/account-routes/" + acc.ID + "/open"
	return wrapAccountNamespace("ip netns exec net-"+acc.ID+" unshare -m", "export CAGE_ACCOUNT_BROWSER="+opener+" BROWSER="+opener+"; "+inner), nil
}
