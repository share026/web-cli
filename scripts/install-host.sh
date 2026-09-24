#!/usr/bin/env bash
# Register nm-host as the Native Messaging host "com.share026.webcli".
#
#   scripts/install-host.sh                 # all detected browsers (user level)
#   scripts/install-host.sh <user-data-dir> # a specific --user-data-dir profile root
#   scripts/install-host.sh --uninstall
#
# The extension ID is derived from the manifest "key", so the unpacked
# extension (chrome://extensions -> Load unpacked -> ./extension) always has
# the ID listed in allowed_origins.
set -euo pipefail
cd "$(dirname "$0")/.."
NAME=com.share026.webcli
ROOT=$(pwd)

if [[ "${1:-}" == "--uninstall" ]]; then
  find "$HOME/.config" "$HOME/Library/Application Support" -path "*NativeMessagingHosts/$NAME.json" -print -delete 2>/dev/null || true
  exit 0
fi

[[ -x bin/nm-host ]] || scripts/build.sh
EXT_ID=$(scripts/extension-id.sh)

manifest() {
  cat <<JSON
{
  "name": "$NAME",
  "description": "web-cli native messaging bridge (Go)",
  "path": "$ROOT/bin/nm-host",
  "type": "stdio",
  "allowed_origins": ["chrome-extension://$EXT_ID/"]
}
JSON
}

if [[ $# -ge 1 ]]; then
  dirs=("$1/NativeMessagingHosts")
elif [[ "$(uname)" == "Darwin" ]]; then
  base="$HOME/Library/Application Support"
  dirs=("$base/Google/Chrome/NativeMessagingHosts" "$base/Chromium/NativeMessagingHosts" "$base/BraveSoftware/Brave-Browser/NativeMessagingHosts" "$base/Microsoft Edge/NativeMessagingHosts")
else
  base="$HOME/.config"
  dirs=("$base/google-chrome/NativeMessagingHosts" "$base/chromium/NativeMessagingHosts" "$base/BraveSoftware/Brave-Browser/NativeMessagingHosts" "$base/microsoft-edge/NativeMessagingHosts")
fi

for d in "${dirs[@]}"; do
  mkdir -p "$d"
  manifest > "$d/$NAME.json"
  echo "installed $d/$NAME.json"
done
echo "extension id: $EXT_ID"
