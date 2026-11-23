package cage

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
)

//go:embed ai-desktop-session.sh
var aiDesktopSession string

//go:embed ai-desktop-start.sh
var aiDesktopStart string

var validAIDesktopIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

func ValidAIDesktopID(id string) bool {
	return validAIDesktopIDPattern.MatchString(id)
}

// AIDesktopSpec describes an AI Desktop application that can be downloaded,
// installed, and run inside the Cage Linux guest via Xtigervnc + Openbox.
type AIDesktopSpec struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Homepage        string   `json:"homepage,omitempty"`
	Category        string   `json:"category,omitempty"`
	InstallType     string   `json:"install_type"` // "deb", "github-deb", "appimage", "apt"
	DownloadURL     string   `json:"download_url,omitempty"`
	ARM64URL        string   `json:"arm64_url,omitempty"`
	GitHubRepo      string   `json:"github_repo,omitempty"`
	AssetAMD64      string   `json:"asset_amd64,omitempty"`
	AssetARM64      string   `json:"asset_arm64,omitempty"`
	PackageName     string   `json:"package_name,omitempty"`
	ExecCommand     string   `json:"exec_command,omitempty"`
	Electron        bool     `json:"electron"`
	SupportedArches []string `json:"supported_arches"`
	DisplayNumber   int      `json:"display_number"`
	Custom          bool     `json:"custom,omitempty"`
}

// BuiltinAIDesktops returns the curated catalog of AI Desktop applications
// that ship official Linux packages compatible with Cage's Debian 13 guest.
func BuiltinAIDesktops() []AIDesktopSpec {
	return []AIDesktopSpec{
		{
			ID:              "antigravity",
			Name:            "Google Antigravity",
			Description:     "Google 官方 Agentic 桌面应用，内置 Gemini 与多 Agent 编排工作区。",
			Homepage:        "https://antigravity.google/download?os=linux",
			Category:        "coding",
			InstallType:     "antigravity-tarball",
			ExecCommand:     "/opt/cage-ai-desktops/antigravity/antigravity",
			Electron:        true,
			SupportedArches: []string{"amd64", "arm64"},
			DisplayNumber:   87,
		},
		{
			ID:              "chatgpt",
			Name:            "ChatGPT Desktop",
			Description:     "OpenAI 官方桌面应用，集成 ChatGPT 与 Codex，支持本地项目与文件协作。",
			Homepage:        "https://learn.chatgpt.com/docs/linux/linux-app",
			Category:        "chat",
			InstallType:     "deb",
			DownloadURL:     "https://persistent.oaistatic.com/codex-app-prod/linux/deb/latest/chatgpt_amd64.deb",
			ARM64URL:        "https://persistent.oaistatic.com/codex-app-prod/linux/deb/latest/chatgpt_arm64.deb",
			PackageName:     "chatgpt",
			ExecCommand:     "chatgpt",
			Electron:        true,
			SupportedArches: []string{"amd64", "arm64"},
			DisplayNumber:   81,
		},
		{
			ID:              "cherry-studio",
			Name:            "Cherry Studio",
			Description:     "开源多模型 AI 桌面客户端，支持 Claude / OpenAI / Gemini / DeepSeek / Ollama 与 MCP。",
			Homepage:        "https://github.com/CherryHQ/cherry-studio/releases",
			Category:        "multi-model",
			InstallType:     "github-deb",
			GitHubRepo:      "CherryHQ/cherry-studio",
			AssetAMD64:      "linux-x64.deb",
			AssetARM64:      "linux-arm64.deb",
			PackageName:     "cherry-studio",
			Electron:        true,
			SupportedArches: []string{"amd64", "arm64"},
			DisplayNumber:   82,
		},
		{
			ID:              "chatbox",
			Name:            "Chatbox",
			Description:     "轻量跨平台 AI 桌面客户端，支持自定义 API Key 与多模型对话。",
			Homepage:        "https://github.com/chatboxai/chatbox/releases",
			Category:        "multi-model",
			InstallType:     "github-deb",
			GitHubRepo:      "chatboxai/chatbox",
			AssetAMD64:      "amd64.deb",
			AssetARM64:      "arm64.deb",
			PackageName:     "xyz.chatboxapp.app",
			Electron:        true,
			SupportedArches: []string{"amd64", "arm64"},
			DisplayNumber:   83,
		},
		{
			ID:              "opencode",
			Name:            "OpenCode Desktop",
			Description:     "开源 AI 编程 Agent 桌面应用，支持多 Agent 并行与多模型提供商。",
			Homepage:        "https://github.com/anomalyco/opencode/releases",
			Category:        "coding",
			InstallType:     "github-deb",
			GitHubRepo:      "anomalyco/opencode",
			AssetAMD64:      "opencode-desktop-linux-amd64.deb",
			AssetARM64:      "opencode-desktop-linux-arm64.deb",
			PackageName:     "opencode-desktop",
			Electron:        true,
			SupportedArches: []string{"amd64", "arm64"},
			DisplayNumber:   84,
		},
		{
			ID:              "cursor",
			Name:            "Cursor",
			Description:     "AI 原生代码编辑器，内置智能代码补全、Chat 与 Agent 工作流。",
			Homepage:        "https://cursor.com/download",
			Category:        "coding",
			InstallType:     "apt-cursor",
			PackageName:     "cursor",
			ExecCommand:     "cursor",
			Electron:        true,
			SupportedArches: []string{"amd64", "arm64"},
			DisplayNumber:   85,
		},
		{
			ID:              "devin-desktop",
			Name:            "Devin Desktop (Windsurf)",
			Description:     "Cognition AI 编程桌面 IDE（原 Windsurf），支持本地 Agent 与代码库上下文。",
			Homepage:        "https://docs.devin.ai/desktop/getting-started",
			Category:        "coding",
			InstallType:     "apt-devin",
			PackageName:     "devin-desktop",
			ExecCommand:     "devin-desktop",
			Electron:        true,
			SupportedArches: []string{"amd64"},
			DisplayNumber:   86,
		},
	}
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// AIDesktopProvisionScript installs the shared AI Desktop session runner and
// per-app systemd unit inside the Linux guest.
func AIDesktopProvisionScript(app AIDesktopSpec) string {
	unitName := fmt.Sprintf("cage-ai-desktop-%s.service", app.ID)
	runtimeDir := fmt.Sprintf("cage-ai-desktop-%s", app.ID)
	unitBody := fmt.Sprintf(`[Unit]
Description=%s in Cage
[Service]
Type=notify
NotifyAccess=all
User=cage
WorkingDirectory=/home/cage
Environment=HOME=/home/cage
EnvironmentFile=/etc/claude-cage/environment
RuntimeDirectory=%s
RuntimeDirectoryMode=0700
ExecStart=/usr/local/bin/cage-log-run /usr/local/bin/cage-ai-desktop-session %s %d
ExecStop=/bin/bash /usr/local/lib/desktop-finish.sh /run/%s %s
LogNamespace=cage
KillMode=control-group
TimeoutStartSec=60
TimeoutStopSec=20
`, app.Name, runtimeDir, app.ID, app.DisplayNumber, runtimeDir, unitName)

	return "set -euo pipefail\n" +
		sharedClipboardScript() + nativeInputScript() + displayGovernorScript() +
		"sudo install -d -m 0755 /etc/claude-cage /etc/claude-cage/ai-desktops\n" +
		"sudo tee /usr/local/bin/cage-ai-desktop-session >/dev/null <<'CAGE_AI_DESKTOP_SESSION'\n" + aiDesktopSession + "\nCAGE_AI_DESKTOP_SESSION\n" +
		"sudo tee /usr/local/bin/cage-ai-desktop-start >/dev/null <<'CAGE_AI_DESKTOP_START'\n" + aiDesktopStart + "\nCAGE_AI_DESKTOP_START\n" +
		"sudo chmod 0755 /usr/local/bin/cage-ai-desktop-session /usr/local/bin/cage-ai-desktop-start\n" +
		guestOpenScript() + `
sudo tee /etc/claude-cage/desktop-openbox.xml >/dev/null <<'CAGE_AI_WINDOWS'
<?xml version="1.0" encoding="UTF-8"?>
<openbox_config xmlns="http://openbox.org/3.4/rc">
<focus><focusNew>yes</focusNew><followMouse>no</followMouse></focus>
<desktops><number>1</number></desktops>
<applications><application type="normal"><decor>no</decor><maximized>yes</maximized></application></applications>
</openbox_config>
CAGE_AI_WINDOWS
sudo tee /etc/systemd/system/` + unitName + ` >/dev/null <<'CAGE_AI_UNIT'
` + unitBody + `CAGE_AI_UNIT
sudo systemctl daemon-reload
`
}

// AIDesktopInstallScript generates the guest installation script for an AI Desktop app.
func AIDesktopInstallScript(app AIDesktopSpec) string {
	electronFlag := "false"
	if app.Electron {
		electronFlag = "true"
	}

	prelude := ToolInstallPrelude() + fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
APP_ID=%s
APP_NAME=%s
EXEC_OVERRIDE=%s
ELECTRON_FLAG=%s
mkdir -p "$HOME/.config/claude-cage"
exec 9>"$HOME/.config/claude-cage/ai-desktop-${APP_ID}-install.lock"
flock -n 9 || { echo "${APP_NAME} 安装任务已在运行中。" >&2; exit 1; }
if systemctl is-active --quiet "cage-ai-desktop-${APP_ID}.service" 2>/dev/null; then
  sudo systemctl stop "cage-ai-desktop-${APP_ID}.service" || true
fi
ARCH="$(dpkg --print-architecture)"
case "$ARCH" in arm64|amd64) ;; *) echo "不支持的系统架构: $ARCH" >&2; exit 1 ;; esac

printf '正在准备图形运行依赖…\n'
sudo env DEBIAN_FRONTEND=noninteractive APT_CONFIG="${CAGE_APT_CONFIG:-/dev/null}" apt-get -o DPkg::Lock::Timeout=60 update </dev/null
sudo env DEBIAN_FRONTEND=noninteractive APT_CONFIG="${CAGE_APT_CONFIG:-/dev/null}" apt-get -o DPkg::Lock::Timeout=60 install -y --no-install-recommends \
  ca-certificates curl gnupg dbus-x11 xauth x11-utils x11-xserver-utils tigervnc-standalone-server tigervnc-tools openbox gnome-keyring gcr socat libglib2.0-bin ibus ibus-gtk3 gir1.2-ibus-1.0 gir1.2-gtk-3.0 python3-gi python3-pil python3-xlib xsettingsd libnss3 libatk1.0-0 libatk-bridge2.0-0 libcups2 libdrm2 libxkbcommon0 libxcomposite1 libxdamage1 libxfixes3 libxrandr2 libgbm1 libasound2t64 </dev/null || \
sudo env DEBIAN_FRONTEND=noninteractive APT_CONFIG="${CAGE_APT_CONFIG:-/dev/null}" apt-get -o DPkg::Lock::Timeout=60 install -y --no-install-recommends \
  ca-certificates curl gnupg dbus-x11 xauth x11-utils x11-xserver-utils tigervnc-standalone-server tigervnc-tools openbox gnome-keyring gcr socat libglib2.0-bin ibus ibus-gtk3 gir1.2-ibus-1.0 gir1.2-gtk-3.0 python3-gi python3-pil python3-xlib xsettingsd libnss3 libatk1.0-0 libatk-bridge2.0-0 libcups2 libdrm2 libxkbcommon0 libxcomposite1 libxdamage1 libxfixes3 libxrandr2 libgbm1 </dev/null

temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT
sudo install -d -m 0755 /etc/claude-cage/ai-desktops

detect_deb_exec() {
  local pkg="$1"
  if [ -n "$EXEC_OVERRIDE" ]; then
    printf '%%s\n' "$EXEC_OVERRIDE"
    return 0
  fi
  /usr/bin/python3 - "$pkg" <<'CAGE_FIND_EXEC'
import re, subprocess, sys
pkg = sys.argv[1]
files = subprocess.run(["dpkg", "-L", pkg], capture_output=True, text=True).stdout.splitlines()
for f in files:
    if f.endswith(".desktop") and "/applications/" in f:
        try:
            for line in open(f, encoding="utf-8", errors="ignore"):
                if line.startswith("Exec="):
                    cmd = line.split("=", 1)[1].strip()
                    cmd = re.sub(r"\s+%%[a-zA-Z]", "", cmd).strip()
                    if cmd:
                        print(cmd)
                        sys.exit(0)
        except OSError:
            pass
for f in files:
    if f.startswith("/usr/bin/") and len(f) > len("/usr/bin/"):
        print(f)
        sys.exit(0)
print(pkg)
CAGE_FIND_EXEC
}

write_meta() {
  local kind="$1" pkg="$2" exec_cmd="$3" version="$4" app_dir="$5"
  sudo /usr/bin/python3 - "/etc/claude-cage/ai-desktops/${APP_ID}.json" "$APP_ID" "$kind" "$pkg" "$exec_cmd" "$version" "$app_dir" "$ELECTRON_FLAG" <<'CAGE_WRITE_META'
import json, os, sys
path, app_id, kind, pkg, exec_cmd, version, app_dir, electron = sys.argv[1:]
data = {
    "id": app_id,
    "kind": kind,
    "package": pkg,
    "exec": exec_cmd,
    "version": version,
    "app_dir": app_dir,
    "electron": electron == "true",
}
with open(path, "w", encoding="utf-8") as fh:
    json.dump(data, fh, ensure_ascii=False, indent=2)
os.chmod(path, 0o644)
CAGE_WRITE_META
}
`, shellSingleQuote(app.ID), shellSingleQuote(app.Name), shellSingleQuote(app.ExecCommand), electronFlag)

	var body string
	switch app.InstallType {
	case "deb":
		body = fmt.Sprintf(`
URL_AMD64=%s
URL_ARM64=%s
if [ "$ARCH" = "arm64" ]; then
  DOWNLOAD_URL="${URL_ARM64:-$URL_AMD64}"
else
  DOWNLOAD_URL="$URL_AMD64"
fi
if [ -z "$DOWNLOAD_URL" ]; then
  echo "${APP_NAME} 暂未提供 ${ARCH} 架构的安装包。" >&2
  exit 1
fi
printf '正在下载 %%s (.deb)…\n' "$APP_NAME"
cage_curl_download "$temporary/app.deb" --proto '=https' --proto-redir '=https' -fSL --connect-timeout 20 "$DOWNLOAD_URL" -o "$temporary/app.deb"
PKG_NAME="$(dpkg-deb -f "$temporary/app.deb" Package)"
PKG_VER="$(dpkg-deb -f "$temporary/app.deb" Version)"
printf '正在安装 %%s (%%s)…\n' "$APP_NAME" "$PKG_VER"
sudo env DEBIAN_FRONTEND=noninteractive APT_CONFIG="${CAGE_APT_CONFIG:-/dev/null}" apt-get -o DPkg::Lock::Timeout=60 install -y "$temporary/app.deb" </dev/null
EXEC_CMD="$(detect_deb_exec "$PKG_NAME")"
write_meta "deb" "$PKG_NAME" "$EXEC_CMD" "$PKG_VER" ""
printf '%%s 已安装。\n' "$APP_NAME"
`, shellSingleQuote(app.DownloadURL), shellSingleQuote(app.ARM64URL))

	case "github-deb":
		body = fmt.Sprintf(`
REPO=%s
ASSET_AMD64=%s
ASSET_ARM64=%s
if [ "$ARCH" = "arm64" ]; then
  SUFFIX="$ASSET_ARM64"
else
  SUFFIX="$ASSET_AMD64"
fi
printf '正在从 GitHub 获取 %%s 最新版本信息…\n' "$APP_NAME"
cage_curl_download "$temporary/release.json" --proto '=https' --proto-redir '=https' -fSL --connect-timeout 20 -H "Accept: application/vnd.github+json" "https://api.github.com/repos/${REPO}/releases/latest" -o "$temporary/release.json"
DOWNLOAD_URL="$(/usr/bin/python3 - "$temporary/release.json" "$SUFFIX" "$ARCH" <<'CAGE_GH_ASSET'
import json, sys
rel = json.load(open(sys.argv[1], encoding="utf-8"))
suffix = sys.argv[2].lower()
arch = sys.argv[3].lower()
assets = rel.get("assets", [])
for a in assets:
    name = a.get("name", "").lower()
    if suffix and name.endswith(suffix):
        print(a["browser_download_url"])
        sys.exit(0)
# Fallback matching .deb by arch
arch_tokens = ["arm64", "aarch64"] if arch == "arm64" else ["amd64", "x64", "x86_64"]
for a in assets:
    name = a.get("name", "").lower()
    if name.endswith(".deb") and any(t in name for t in arch_tokens):
        print(a["browser_download_url"])
        sys.exit(0)
sys.exit(1)
CAGE_GH_ASSET
)" || { echo "未在 GitHub Releases 找到适用于 ${ARCH} 的 .deb 安装包。" >&2; exit 1; }
printf '正在下载 %%s…\n' "$APP_NAME"
cage_curl_download "$temporary/app.deb" --proto '=https' --proto-redir '=https' -fSL --connect-timeout 20 "$DOWNLOAD_URL" -o "$temporary/app.deb"
PKG_NAME="$(dpkg-deb -f "$temporary/app.deb" Package)"
PKG_VER="$(dpkg-deb -f "$temporary/app.deb" Version)"
printf '正在安装 %%s (%%s)…\n' "$APP_NAME" "$PKG_VER"
sudo env DEBIAN_FRONTEND=noninteractive APT_CONFIG="${CAGE_APT_CONFIG:-/dev/null}" apt-get -o DPkg::Lock::Timeout=60 install -y "$temporary/app.deb" </dev/null
EXEC_CMD="$(detect_deb_exec "$PKG_NAME")"
write_meta "deb" "$PKG_NAME" "$EXEC_CMD" "$PKG_VER" ""
printf '%%s 已安装。\n' "$APP_NAME"
`, shellSingleQuote(app.GitHubRepo), shellSingleQuote(app.AssetAMD64), shellSingleQuote(app.AssetARM64))

	case "appimage":
		body = fmt.Sprintf(`
URL_AMD64=%s
URL_ARM64=%s
if [ "$ARCH" = "arm64" ]; then
  DOWNLOAD_URL="${URL_ARM64:-$URL_AMD64}"
else
  DOWNLOAD_URL="$URL_AMD64"
fi
if [ -z "$DOWNLOAD_URL" ]; then
  echo "${APP_NAME} 暂未提供 ${ARCH} 架构的 AppImage。" >&2
  exit 1
fi
printf '正在下载 %%s (AppImage)…\n' "$APP_NAME"
cage_curl_download "$temporary/app.AppImage" --proto '=https' --proto-redir '=https' -fSL --connect-timeout 20 "$DOWNLOAD_URL" -o "$temporary/app.AppImage"
chmod +x "$temporary/app.AppImage"
APP_DIR="/opt/cage-ai-desktops/${APP_ID}"
printf '正在解压安装 %%s…\n' "$APP_NAME"
(cd "$temporary" && ./app.AppImage --appimage-extract >/dev/null)
sudo rm -rf "$APP_DIR"
sudo install -d -m 0755 /opt/cage-ai-desktops
sudo mv "$temporary/squashfs-root" "$APP_DIR"
sudo chmod -R a+rX "$APP_DIR"
EXEC_CMD="${EXEC_OVERRIDE:-${APP_DIR}/AppRun}"
write_meta "appimage" "" "$EXEC_CMD" "AppImage" "$APP_DIR"
printf '%%s 已安装。\n' "$APP_NAME"
`, shellSingleQuote(app.DownloadURL), shellSingleQuote(app.ARM64URL))

	case "apt-cursor":
		body = `
if [ "$ARCH" = "amd64" ]; then
  printf '正在配置 Cursor 官方 APT 软件源…\n'
  sudo install -d -m 0755 /etc/apt/keyrings
  cage_curl_download "$temporary/anysphere.asc" --proto '=https' --proto-redir '=https' -fSL --connect-timeout 20 https://downloads.cursor.com/keys/anysphere.asc -o "$temporary/anysphere.asc"
  gpg --batch --dearmor < "$temporary/anysphere.asc" | sudo tee /etc/apt/keyrings/cursor.gpg >/dev/null
  sudo chmod 0644 /etc/apt/keyrings/cursor.gpg
  printf '%s\n' 'deb [arch=amd64 signed-by=/etc/apt/keyrings/cursor.gpg] https://downloads.cursor.com/aptrepo stable main' | sudo tee /etc/apt/sources.list.d/cursor.list >/dev/null
  sudo env DEBIAN_FRONTEND=noninteractive APT_CONFIG="${CAGE_APT_CONFIG:-/dev/null}" apt-get -o DPkg::Lock::Timeout=60 update </dev/null
  printf '正在下载并安装 Cursor…\n'
  sudo env DEBIAN_FRONTEND=noninteractive APT_CONFIG="${CAGE_APT_CONFIG:-/dev/null}" apt-get -o DPkg::Lock::Timeout=60 install -y cursor </dev/null
else
  printf '正在下载 Cursor ARM64 安装包…\n'
  cage_curl_download "$temporary/cursor.deb" --proto '=https' --proto-redir '=https' -fSL --connect-timeout 20 'https://api2.cursor.sh/updates/download/golden/linux-arm64-deb/cursor/latest' -o "$temporary/cursor.deb"
  sudo env DEBIAN_FRONTEND=noninteractive APT_CONFIG="${CAGE_APT_CONFIG:-/dev/null}" apt-get -o DPkg::Lock::Timeout=60 install -y "$temporary/cursor.deb" </dev/null
fi
PKG_VER="$(dpkg-query -W -f='${Version}' cursor 2>/dev/null || echo '')"
EXEC_CMD="$(detect_deb_exec cursor)"
write_meta "deb" "cursor" "$EXEC_CMD" "$PKG_VER" ""
printf 'Cursor 已安装。\n'
`

	case "antigravity-tarball":
		body = `
resolve_antigravity_url() {
  cage_curl_download "$temporary/download.html" --proto '=https' --proto-redir '=https' --compressed -fSL --connect-timeout 20 'https://antigravity.google/download' -o "$temporary/download.html" >&2 || return $?
  /usr/bin/python3 - "$temporary/download.html" "$ARCH" <<'CAGE_AG_URL'
import re, sys, urllib.request
html = open(sys.argv[1], encoding="utf-8", errors="ignore").read()
arch = sys.argv[2]
arch_pat = r'(?:linux-arm|linux-arm64|linux-aarch64)' if arch == 'arm64' else r'(?:linux-x64|linux-amd64|linux-x86_64)'
# Prefer Antigravity IDE (edgedl.me.gvt1.com), fall back to Antigravity App (storage.googleapis.com).
patterns = [
    re.compile(r'https://edgedl\.me\.gvt1\.com/[^"\'\s<>]+/' + arch_pat + r'/Antigravity(?:%20IDE)?\.tar\.gz'),
    re.compile(r'https://storage\.googleapis\.com/antigravity-public/antigravity-hub/[^"\'\s<>]+/' + arch_pat + r'/Antigravity\.tar\.gz'),
]
def find_in(text):
    for pat in patterns:
        m = pat.search(text)
        if m:
            return m.group(0)
    return ''
found = find_in(html)
if not found:
    scripts = re.findall(r'src=["\']([^"\']*?(?:main|chunk|app)[^"\']*?\.js)["\']', html)
    for s in scripts[:12]:
        js_url = s if s.startswith('http') else 'https://antigravity.google/' + s.lstrip('/')
        try:
            with urllib.request.urlopen(js_url, timeout=20) as resp:
                js = resp.read().decode('utf-8', errors='ignore')
            found = find_in(js)
            if found:
                break
        except Exception:
            continue
if found:
    print(found)
    sys.exit(0)
sys.exit(1)
CAGE_AG_URL
}
printf '正在从 antigravity.google 解析最新官方 Linux 下载地址…\n'
DOWNLOAD_URL="$(resolve_antigravity_url)" || {
  status=$?
  if (( status >= 128 )); then exit "$status"; fi
  # Official IDE release links from https://antigravity.google/download.
  platform=linux-x64
  if [ "$ARCH" = arm64 ]; then platform=linux-arm; fi
  DOWNLOAD_URL="https://edgedl.me.gvt1.com/edgedl/release2/j0qc3/antigravity/stable/2.8.0-6733682917113856/${platform}/Antigravity%20IDE.tar.gz"
  printf '未能读取最新版本链接，改用 Google 官方 Antigravity IDE 2.8.0 安装包。\n' >&2
}
printf '正在下载 Google Antigravity (%s)…\n' "$ARCH"
cage_curl_download "$temporary/antigravity.tar.gz" --proto '=https' --proto-redir '=https' -fSL --connect-timeout 20 "$DOWNLOAD_URL" -o "$temporary/antigravity.tar.gz"
mkdir -p "$temporary/extracted"
printf '正在解压安装 Google Antigravity…\n'
tar -xzf "$temporary/antigravity.tar.gz" -C "$temporary/extracted"
APP_DIR="/opt/cage-ai-desktops/antigravity"
sudo rm -rf "$APP_DIR"
sudo install -d -m 0755 /opt/cage-ai-desktops
TOP_COUNT="$(find "$temporary/extracted" -mindepth 1 -maxdepth 1 | wc -l)"
if [ "$TOP_COUNT" -eq 1 ] && [ -d "$(find "$temporary/extracted" -mindepth 1 -maxdepth 1)" ]; then
  sudo mv "$(find "$temporary/extracted" -mindepth 1 -maxdepth 1)" "$APP_DIR"
else
  sudo mv "$temporary/extracted" "$APP_DIR"
fi
sudo chmod -R a+rX "$APP_DIR"
if [ -f "$APP_DIR/chrome-sandbox" ] && [ ! -L "$APP_DIR/chrome-sandbox" ]; then
  sudo chown root:root "$APP_DIR/chrome-sandbox"
  sudo chmod 4755 "$APP_DIR/chrome-sandbox"
fi
EXEC_CMD="$(find "$APP_DIR" -maxdepth 2 -type f \( -name 'antigravity' -o -name 'antigravity-ide' \) -perm -u+x | head -n 1)"
if [ -z "$EXEC_CMD" ]; then
  EXEC_CMD="${APP_DIR}/antigravity"
fi
APP_VER="$(/usr/bin/python3 - "$APP_DIR" "$DOWNLOAD_URL" <<'CAGE_AG_VER'
import json, pathlib, re, sys
for p in [pathlib.Path(sys.argv[1]) / 'resources/app/package.json', pathlib.Path(sys.argv[1]) / 'package.json']:
    if p.is_file():
        try:
            v = json.loads(p.read_text(encoding='utf-8')).get('version', '')
            if v:
                print(v)
                sys.exit(0)
        except Exception:
            pass
m = re.search(r'/([0-9]+\.[0-9]+\.[0-9]+)(?:-[0-9]+)?/', sys.argv[2])
if m:
    print(m.group(1))
CAGE_AG_VER
)"
write_meta "tarball" "" "$EXEC_CMD" "${APP_VER:-2.x}" "$APP_DIR"
printf 'Google Antigravity 已安装。\n'
`

	case "apt-devin":
		body = `
if [ "$ARCH" != "amd64" ]; then
  echo 'Devin Desktop (Windsurf) 官方 APT 源目前仅提供 amd64 架构版本。' >&2
  exit 1
fi
printf '正在配置 Devin Desktop (Windsurf) 官方 APT 软件源…\n'
sudo install -d -m 0755 /etc/apt/keyrings
cage_curl_download "$temporary/windsurf.gpg" --proto '=https' --proto-redir '=https' -fSL --connect-timeout 20 https://windsurf-stable.codeiumdata.com/wVxQEIWkwPUEAGf3/windsurf.gpg -o "$temporary/windsurf.gpg"
gpg --batch --dearmor < "$temporary/windsurf.gpg" | sudo tee /etc/apt/keyrings/windsurf-stable.gpg >/dev/null
sudo chmod 0644 /etc/apt/keyrings/windsurf-stable.gpg
printf '%s\n' 'deb [arch=amd64 signed-by=/etc/apt/keyrings/windsurf-stable.gpg] https://windsurf-stable.codeiumdata.com/wVxQEIWkwPUEAGf3/apt stable main' | sudo tee /etc/apt/sources.list.d/windsurf.list >/dev/null
sudo env DEBIAN_FRONTEND=noninteractive APT_CONFIG="${CAGE_APT_CONFIG:-/dev/null}" apt-get -o DPkg::Lock::Timeout=60 update </dev/null
printf '正在下载并安装 Devin Desktop…\n'
sudo env DEBIAN_FRONTEND=noninteractive APT_CONFIG="${CAGE_APT_CONFIG:-/dev/null}" apt-get -o DPkg::Lock::Timeout=60 install -y devin-desktop </dev/null
PKG_VER="$(dpkg-query -W -f='${Version}' devin-desktop 2>/dev/null || echo '')"
EXEC_CMD="$(detect_deb_exec devin-desktop)"
write_meta "deb" "devin-desktop" "$EXEC_CMD" "$PKG_VER" ""
printf 'Devin Desktop 已安装。\n'
`
	}

	return prelude + body + "\n" + AIDesktopProvisionScript(app)
}

// AIDesktopUninstallScript stops and removes an installed AI Desktop app.
func AIDesktopUninstallScript(app AIDesktopSpec) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
APP_ID=%s
UNIT="cage-ai-desktop-${APP_ID}.service"
META="/etc/claude-cage/ai-desktops/${APP_ID}.json"
sudo systemctl stop "$UNIT" 2>/dev/null || true
if [ -r "$META" ]; then
  readarray -t info < <(/usr/bin/python3 - "$META" <<'CAGE_AI_UNINSTALL'
import json, sys
d = json.load(open(sys.argv[1], encoding="utf-8"))
print(d.get("kind", ""))
print(d.get("package", ""))
print(d.get("app_dir", ""))
CAGE_AI_UNINSTALL
)
  kind="${info[0]:-}"
  pkg="${info[1]:-}"
  app_dir="${info[2]:-}"
  if [ "$kind" = "deb" ] && [ -n "$pkg" ]; then
    sudo env DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=60 remove -y "$pkg" </dev/null || true
  fi
  if [ -n "$app_dir" ] && [[ "$app_dir" == /opt/cage-ai-desktops/* ]]; then
    sudo rm -rf "$app_dir"
  fi
  sudo rm -f "$META"
fi
sudo rm -f "/etc/systemd/system/${UNIT}"
sudo systemctl daemon-reload || true
printf '应用已卸载。\n'
`, shellSingleQuote(app.ID))
}
