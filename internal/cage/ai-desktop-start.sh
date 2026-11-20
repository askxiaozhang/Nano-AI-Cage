#!/usr/bin/env bash
set -euo pipefail
app_id="${1:?app id required}"
unit="cage-ai-desktop-${app_id}.service"
runtime_dir="/run/cage-ai-desktop-${app_id}"

if systemctl is-active --quiet "$unit"; then
  exit 0
fi
if systemctl is-failed --quiet "$unit"; then
  sudo systemctl reset-failed "$unit"
fi
sudo systemctl start --no-block "$unit"

# If gnome-keyring is installed, wait briefly for the unlock FIFO and feed stdin key.
for _ in $(seq 1 100); do
  if [ -p "${runtime_dir}/unlock" ]; then
    timeout 10 sh -c "cat > '${runtime_dir}/unlock'" || true
    break
  fi
  if systemctl is-active --quiet "$unit"; then
    exit 0
  fi
  if systemctl is-failed --quiet "$unit"; then
    echo "图形会话未能启动。" >&2
    exit 1
  fi
  sleep .1
done

for _ in $(seq 1 300); do
  if systemctl is-active --quiet "$unit"; then
    exit 0
  fi
  if systemctl is-failed --quiet "$unit"; then
    echo "AI Desktop 应用未能启动，请查看操作详情。" >&2
    exit 1
  fi
  sleep .1
done
echo "AI Desktop 启动超时。" >&2
exit 1
