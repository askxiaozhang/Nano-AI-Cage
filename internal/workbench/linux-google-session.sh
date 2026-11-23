#!/bin/bash
# Launches an isolated per-Google-account Antigravity Desktop or Chromium Browser session.
# Runs as the workspace user inside the Linux guest.
set -euo pipefail
umask 077

id=$1
mode=$2
scope=$3
start_url=${4:-https://gemini.google.com}

[[ $id =~ ^[0-9a-f]{24}$ ]]
[[ $mode == antigravity || $mode == browser ]]

unit="google-${mode}-${id}"
base="$HOME/.local/share/cage/google-accounts/$id"
runtime="/run/$unit"
mkdir -p "$base"
exec 9>"$base/${mode}-launch.lock"
flock -w 55 9

if [[ $scope == stop ]]; then
  if systemctl is-active --quiet "$unit.service"; then
    sudo systemctl stop "$unit.service" 2>/dev/null || ! systemctl is-active --quiet "$unit.service"
  fi
  exit 0
fi

if systemctl is-active --quiet "$unit.service"; then
  exit 0
fi

if [[ $mode == antigravity ]]; then
  test -r /etc/claude-cage/ai-desktops/antigravity.json || { echo '请先在 AI Desktops 中安装 Google Antigravity'; exit 1; }
else
  command -v chromium >/dev/null 2>&1 || command -v chromium-browser >/dev/null 2>&1 || command -v google-chrome >/dev/null 2>&1 || { echo '未找到 Chromium 浏览器'; exit 1; }
fi

sudo systemctl reset-failed "$unit.service" 2>/dev/null || :

props=()
if [[ $scope == node-* ]]; then
  route=${scope#node-}
  [[ $route =~ ^[0-9a-f]{24}$ ]]
  namespace="net-$route"
  test -e "/run/netns/$namespace" || { echo '专属网络尚未就绪'; exit 1; }
  props+=("--property=NetworkNamespacePath=/run/netns/$namespace")
  [[ -s /etc/netns/$namespace/resolv.conf ]] || { echo '专属网络尚未就绪'; exit 1; }
  props+=(
    "--property=BindReadOnlyPaths=/etc/netns/$namespace/resolv.conf:/etc/resolv.conf"
    "--property=InaccessiblePaths=-/run/systemd/resolve/io.systemd.Resolve -/run/dbus/system_bus_socket -/run/nscd/socket"
  )
elif [[ $scope != shared ]]; then
  exit 2
fi

real_home="$HOME"
program=$(mktemp "$base/${mode}-session.XXXXXX")
trap 'rm -f -- "$program"' EXIT

cat >"$program" <<'SESSION'
set -euo pipefail
umask 077
base=$1
mode=$2
start_url=$3
real_home=$4

export HOME="$base/home"
export XDG_RUNTIME_DIR="$RUNTIME_DIRECTORY"
export XDG_CONFIG_HOME="$HOME/.config"
export XDG_DATA_HOME="$HOME/.local/share"
export XDG_CACHE_HOME="$HOME/.cache"
export XDG_CURRENT_DESKTOP=Openbox
unset RUNTIME_DIRECTORY INVOCATION_ID JOURNAL_STREAM SYSTEMD_EXEC_PID
export PATH="$HOME/.local/bin:$real_home/.local/bin:/usr/local/bin:/usr/bin:/bin"

mkdir -p "$HOME" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME/applications" "$XDG_CACHE_HOME" \
  "$base/antigravity-data" "$base/chromium-data"

chromium_bin=""
for cand in chromium chromium-browser google-chrome; do
  if command -v "$cand" >/dev/null 2>&1; then
    chromium_bin="$(command -v "$cand")"
    break
  fi
done

# Route OAuth / external links from Antigravity into this same Google account's Chromium profile
# inside the same network namespace and X display so Google login completes seamlessly.
mkdir -p "$XDG_RUNTIME_DIR/bin"
cat > "$XDG_RUNTIME_DIR/bin/google-open" <<EOF
#!/bin/sh
if [ -n "$chromium_bin" ]; then
  exec "$chromium_bin" --no-sandbox --password-store=gnome-libsecret --user-data-dir="$base/chromium-data" --no-first-run --no-default-browser-check "\$@"
fi
exec /usr/local/bin/cage-open "\$@"
EOF
chmod 0700 "$XDG_RUNTIME_DIR/bin/google-open"
cp "$XDG_RUNTIME_DIR/bin/google-open" "$XDG_RUNTIME_DIR/bin/xdg-open"
export PATH="$XDG_RUNTIME_DIR/bin:$PATH"
export BROWSER="$XDG_RUNTIME_DIR/bin/google-open"

export XAUTHORITY="$XDG_RUNTIME_DIR/Xauthority"
touch "$XAUTHORITY"
cookie=$(mcookie)
xauth -f "$XAUTHORITY" add :0 . "$cookie"
od -An -N4 -tx1 /dev/urandom | tr -d ' \n' > "$XDG_RUNTIME_DIR/password"
tigervncpasswd -f < "$XDG_RUNTIME_DIR/password" > "$XDG_RUNTIME_DIR/vnc-password"

Xtigervnc -displayfd 3 3>"$XDG_RUNTIME_DIR/display" -geometry 1600x1000 -depth 24 -rfbport -1 \
  -rfbunixpath "$XDG_RUNTIME_DIR/vnc.sock" -rfbunixmode 0600 -auth "$XAUTHORITY" -nolisten tcp \
  -SecurityTypes VncAuth -PasswordFile "$XDG_RUNTIME_DIR/vnc-password" -AlwaysShared &
display_pid=$!
trap 'kill "$display_pid" 2>/dev/null || true' EXIT

for _ in {1..100}; do
  test -s "$XDG_RUNTIME_DIR/display" && break
  sleep .05
done
export DISPLAY=":$(cat "$XDG_RUNTIME_DIR/display")"
xauth -f "$XAUTHORITY" add "$DISPLAY" . "$cookie"
dbus-update-activation-environment DISPLAY XAUTHORITY HOME XDG_RUNTIME_DIR XDG_CONFIG_HOME XDG_DATA_HOME XDG_CURRENT_DESKTOP >/dev/null 2>&1 || true

cat > "$XDG_RUNTIME_DIR/windows.xml" <<'XML'
<openbox_config xmlns="http://openbox.org/3.4/rc"><focus><focusNew>yes</focusNew><followMouse>no</followMouse></focus><desktops><number>1</number></desktops><applications><application type="normal"><decor>no</decor><maximized>yes</maximized></application></applications></openbox_config>
XML
openbox --config-file "$XDG_RUNTIME_DIR/windows.xml" >/dev/null 2>&1 &
if [ -x /usr/local/bin/cage-display-governor ]; then
  /usr/local/bin/cage-display-governor &
fi

display_scale=2
if [[ -r /etc/claude-cage/display-scale && $(< /etc/claude-cage/display-scale) == 1.5 ]]; then
  display_scale=1.5
fi
printf '%s\n' "$display_scale" > "$XDG_RUNTIME_DIR/display-scale"
printf '%s\n' "$display_scale" > "$XDG_RUNTIME_DIR/base-display-scale"
scale_args=(--force-device-scale-factor="$display_scale")

if command -v xsettingsd >/dev/null 2>&1; then
  dpi=$(awk -v scale="$display_scale" 'BEGIN {printf "%.0f", scale * 96 * 1024}')
  printf 'Gtk/DecorationLayout ":"\n' > "$XDG_RUNTIME_DIR/xsettings.fixed"
  if [ "$display_scale" = 1.5 ]; then
    printf 'Gtk/EnableAnimations 0\n' >> "$XDG_RUNTIME_DIR/xsettings.fixed"
  fi
  {
    printf 'Xft/DPI %s\nGdk/WindowScalingFactor 1\nGdk/UnscaledDPI %s\n' "$dpi" "$dpi"
    cat "$XDG_RUNTIME_DIR/xsettings.fixed"
  } > "$XDG_RUNTIME_DIR/xsettings.conf"
  xsettingsd --config="$XDG_RUNTIME_DIR/xsettings.conf" &
  printf '%s\n' "$!" > "$XDG_RUNTIME_DIR/xsettings.pid"
  /usr/bin/python3 - <<'PYSETTINGS' || true
import time
from Xlib import display
d = display.Display()
sel = d.intern_atom('_XSETTINGS_S0')
for _ in range(60):
    if d.get_selection_owner(sel):
        break
    time.sleep(.05)
d.close()
PYSETTINGS
  scale_args=()
fi

if command -v gnome-keyring-daemon >/dev/null 2>&1; then
  mkfifo -m 0600 "$XDG_RUNTIME_DIR/unlock"
  gnome-keyring-daemon --unlock --components=secrets < "$XDG_RUNTIME_DIR/unlock" >/dev/null 2>&1 || true
  rm -f "$XDG_RUNTIME_DIR/unlock"
fi

notify=${NOTIFY_SOCKET:-}
unset NOTIFY_SOCKET
cd "$HOME"

if [[ $mode == antigravity ]]; then
  exec_cmd="$(/usr/bin/python3 - /etc/claude-cage/ai-desktops/antigravity.json <<'PYMETA'
import json, sys
print(json.load(open(sys.argv[1], encoding="utf-8")).get("exec", ""))
PYMETA
)"
  case "$exec_cmd" in
    /opt/cage-ai-desktops/antigravity/bin/antigravity|/opt/cage-ai-desktops/antigravity/bin/antigravity-ide)
      gui="/opt/cage-ai-desktops/antigravity/${exec_cmd##*/}"
      if [[ -x "$gui" ]]; then exec_cmd="$gui"; fi
      ;;
  esac
  if [[ -z "$exec_cmd" ]]; then
    echo "未找到 Google Antigravity 可执行文件" >&2
    exit 1
  fi
  launch_args=(--no-sandbox --password-store=gnome-libsecret "--user-data-dir=$base/antigravity-data")
  if [ "${#scale_args[@]}" -gt 0 ]; then
    launch_args+=("${scale_args[@]}")
  fi
  if [ -x /usr/local/bin/cage-input-session ]; then
    /usr/local/bin/cage-input-session "$exec_cmd" "${launch_args[@]}" &
  else
    "$exec_cmd" "${launch_args[@]}" &
  fi
else
  launch_args=(--no-sandbox --password-store=gnome-libsecret "--user-data-dir=$base/chromium-data" --no-first-run --no-default-browser-check --start-maximized)
  if [ "${#scale_args[@]}" -gt 0 ]; then
    launch_args+=("${scale_args[@]}")
  fi
  if [ -x /usr/local/bin/cage-input-session ]; then
    /usr/local/bin/cage-input-session "$chromium_bin" "${launch_args[@]}" "$start_url" &
  else
    "$chromium_bin" "${launch_args[@]}" "$start_url" &
  fi
fi

app_pid=$!
printf '%s\n' "$app_pid" > "$XDG_RUNTIME_DIR/app-pid"

for _ in {1..250}; do
  if ! kill -0 "$app_pid" 2>/dev/null; then
    break
  fi
  if xprop -root _NET_CLIENT_LIST 2>/dev/null | grep -q '0x[1-9a-f]'; then
    if [ -n "$notify" ]; then
      NOTIFY_SOCKET="$notify" systemd-notify --ready
    fi
    break
  fi
  sleep .1
done

if [ -n "$notify" ] && [ -S "$XDG_RUNTIME_DIR/vnc.sock" ]; then
  NOTIFY_SOCKET="$notify" systemd-notify --ready || true
fi

wait "$app_pid" || true
while xprop -root _NET_CLIENT_LIST 2>/dev/null | grep -q '0x[1-9a-f]'; do
  sleep 2
done
SESSION

user=$(id -un)
sudo systemd-run --quiet --no-block --collect --unit="$unit" --service-type=notify \
  --property="User=$user" --property="RuntimeDirectory=$unit" --property=RuntimeDirectoryMode=0700 \
  --property=NotifyAccess=all --property=TimeoutStartSec=50 --property=TimeoutStopSec=10 \
  --property="ExecStop=/bin/bash /usr/local/lib/desktop-finish.sh $runtime $unit.service" \
  --property=KillMode=control-group --property=PrivateTmp=yes "${props[@]}" \
  /usr/bin/dbus-run-session -- /bin/bash "$program" "$base" "$mode" "$start_url" "$real_home"

ok=
trap 'if [[ -z "$ok" ]]; then sudo systemctl stop "$unit.service" >/dev/null 2>&1 || :; fi; rm -f -- "$program"' EXIT

for _ in {1..160}; do
  if [ -p "$runtime/unlock" ]; then
    timeout 8 sh -c 'cat > "$1/unlock"' sh "$runtime" || true
    break
  fi
  if systemctl is-active --quiet "$unit.service"; then
    ok=1
    exit 0
  fi
  if systemctl is-failed --quiet "$unit.service"; then
    echo "Google 独立图形会话启动失败" >&2
    exit 1
  fi
  sleep .1
done

for _ in {1..260}; do
  if systemctl is-active --quiet "$unit.service"; then
    ok=1
    exit 0
  fi
  if systemctl is-failed --quiet "$unit.service"; then
    echo "Google 独立图形会话未能就绪" >&2
    exit 1
  fi
  sleep .1
done
exit 1
