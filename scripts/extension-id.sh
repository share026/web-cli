#!/usr/bin/env bash
# Print the Chrome extension ID derived from the "key" in extension/manifest.json:
# first 32 hex chars of sha256(DER public key), mapped 0-9a-f -> a-p.
set -euo pipefail
cd "$(dirname "$0")/.."
key=$(sed -n 's/.*"key": *"\([^"]*\)".*/\1/p' extension/manifest.json)
printf '%s' "$key" | base64 -d | sha256sum | head -c32 | tr 0-9a-f a-p
echo
