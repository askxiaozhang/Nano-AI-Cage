#!/usr/bin/env bash
set -euo pipefail
app_id="${1:?app id required}"
display_num="${2:?display number required}"

if [ "${CAGE_AI_DESKTOP_SESSION:-}" != 1 ]; then
  export CAGE_AI_DESKTOP_SESSION=1 CAGE_NOTIFY_SOCKET="${NOTIFY_SOCKET:-}"
  unset NOTIFY_SOCKET
  exec dbus-run-session -- "$0" "$app_id" "$display_num"
fi

runtime_dir="/run/cage-ai-desktop-${app_id}"
export DISPLAY=":${display_num}"
export XAUTHORITY="${runtime_dir}/Xauthority"
export XDG_RUNTIME_DIR="${runtime_dir}"
export XDG_CURRENT_DESKTOP=Openbox
export PATH="$HOME/.local/bin:/usr/local/bin:/usr/bin:/bin:$PATH"
export BROWSER=/usr/local/bin/cage-open

dbus-update-activation-environment DISPLAY XAUTHORITY XDG_RUNTIME_DIR XDG_CURRENT_DESKTOP >/dev/null 2>&1 || true
umask 077
touch "$XAUTHORITY"
xauth -f "$XAUTHORITY" add "$DISPLAY" . "$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')"
printf '%s' "${DBUS_SESSION_BUS_ADDRESS:-}" > "${runtime_dir}/bus-address"

password="$HOME/.config/claude-cage/ai-desktop-${app_id}-password"
if [ ! -s "$password" ]; then
  mkdir -p "$(dirname "$password")"
  od -An -N4 -tx1 /dev/urandom | tr -d ' \n' > "$password"
fi
tigervncpasswd -f < "$password" > "${runtime_dir}/vnc-password"

Xtigervnc "$DISPLAY" -geometry 1600x1000 -depth 24 -rfbport -1 -rfbunixpath "${runtime_dir}/vnc.sock" -rfbunixmode 0600 -nolisten tcp -auth "$XAUTHORITY" -SecurityTypes VncAuth -PasswordFile "${runtime_dir}/vnc-password" -AlwaysShared &
display_pid=$!
trap 'kill "$display_pid" 2>/dev/null || true' EXIT

for _ in $(seq 1 100); do
  if xdpyinfo -display "$DISPLAY" >/dev/null 2>&1; then break; fi
  kill -0 "$display_pid"
  sleep .1
done
xdpyinfo -display "$DISPLAY" >/dev/null

openbox --config-file /etc/claude-cage/desktop-openbox.xml &
if [ -x /usr/local/bin/cage-display-governor ]; then
  /usr/local/bin/cage-display-governor &
fi

# Unlock gnome-keyring when available so Electron/AI apps keep login sessions.
if command -v gnome-keyring-daemon >/dev/null 2>&1; then
  mkfifo -m 0600 "${runtime_dir}/unlock"
  gnome-keyring-daemon --unlock --components=secrets < "${runtime_dir}/unlock" >/dev/null 2>&1 || true
  rm -f "${runtime_dir}/unlock"
fi

display_scale=2
if [[ -r /etc/claude-cage/display-scale && $(< /etc/claude-cage/display-scale) == 1.5 ]]; then
  display_scale=1.5
fi
printf '%s\n' "$display_scale" > "${runtime_dir}/display-scale"
printf '%s\n' "$display_scale" > "${runtime_dir}/base-display-scale"
scale_args=(--force-device-scale-factor="$display_scale")

if command -v xsettingsd >/dev/null 2>&1; then
  dpi=$(awk -v scale="$display_scale" 'BEGIN {printf "%.0f", scale * 96 * 1024}')
  printf 'Gtk/DecorationLayout ":"\n' > "${runtime_dir}/xsettings.fixed"
  if [ "$display_scale" = 1.5 ]; then
    printf 'Gtk/EnableAnimations 0\n' >> "${runtime_dir}/xsettings.fixed"
  fi
  {
    printf 'Xft/DPI %s\nGdk/WindowScalingFactor 1\nGdk/UnscaledDPI %s\n' "$dpi" "$dpi"
    cat "${runtime_dir}/xsettings.fixed"
  } > "${runtime_dir}/xsettings.conf"
  xsettingsd --config="${runtime_dir}/xsettings.conf" &
  printf '%s\n' "$!" > "${runtime_dir}/xsettings.pid"
  /usr/bin/python3 - <<'CAGE_AI_DESKTOP_DPI_READY' || true
import time
from Xlib import display
connection = display.Display()
selection = connection.intern_atom('_XSETTINGS_S0')
for _ in range(60):
    if connection.get_selection_owner(selection):
        break
    time.sleep(.05)
connection.close()
CAGE_AI_DESKTOP_DPI_READY
  scale_args=()
fi

meta_file="/etc/claude-cage/ai-desktops/${app_id}.json"
if [ ! -r "$meta_file" ]; then
  echo "应用元数据缺失：${meta_file}" >&2
  exit 1
fi

readarray -t launch_info < <(/usr/bin/python3 - "$meta_file" <<'CAGE_AI_META'
import json, shlex, sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
print(data.get("exec", ""))
print("1" if data.get("electron", True) else "0")
CAGE_AI_META
)
exec_cmd="${launch_info[0]:-}"
is_electron="${launch_info[1]:-1}"
if [ -z "$exec_cmd" ]; then
  echo "未能找到应用启动命令，请重新安装或修复安装。" >&2
  exit 1
fi

extra_args=()
if [ "$is_electron" = "1" ]; then
  extra_args+=(--no-sandbox --password-store=gnome-libsecret)
  if [ "${#scale_args[@]}" -gt 0 ]; then
    extra_args+=("${scale_args[@]}")
  fi
fi

# shellcheck disable=SC2206
cmd_parts=($exec_cmd)
if [ -x /usr/local/bin/cage-input-session ]; then
  /usr/local/bin/cage-input-session "${cmd_parts[@]}" "${extra_args[@]}" &
else
  "${cmd_parts[@]}" "${extra_args[@]}" &
fi
app_pid=$!
printf '%s\n' "$app_pid" > "${runtime_dir}/app-pid"

for _ in $(seq 1 250); do
  if ! kill -0 "$app_pid" 2>/dev/null; then
    break
  fi
  if xprop -root _NET_CLIENT_LIST 2>/dev/null | grep -q '0x[1-9a-f]'; then
    if [ -n "${CAGE_NOTIFY_SOCKET:-}" ]; then
      NOTIFY_SOCKET="$CAGE_NOTIFY_SOCKET" systemd-notify --ready
    fi
    break
  fi
  sleep .1
done

# Fallback readiness notify for apps that fork their main process or open windows lazily.
if [ -n "${CAGE_NOTIFY_SOCKET:-}" ] && [ -S "${runtime_dir}/vnc.sock" ]; then
  NOTIFY_SOCKET="$CAGE_NOTIFY_SOCKET" systemd-notify --ready || true
fi

wait "$app_pid" || true
# Keep Xvfb/VNC session alive while child GUI windows remain open (e.g. forked Electron apps).
while xprop -root _NET_CLIENT_LIST 2>/dev/null | grep -q '0x[1-9a-f]'; do
  sleep 2
done
