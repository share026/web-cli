#!/usr/bin/env bash
# Interactive fzf on a real (pseudo) terminal: fzf draws on /dev/tty, we type
# "check" + Enter, and the element with hint 7 ("Checkout") must be selected.
set -euo pipefail
cd "$(dirname "$0")/.."
command -v script >/dev/null || { echo "util-linux 'script' is required"; exit 1; }
bin=$(mktemp)
go test -c -o "$bin" ./cmd/app
(sleep 3; printf 'check'; sleep 1.5; printf '\r'; sleep 1) |
  TERM=xterm script -qfec "WEBCLI_TTY_TEST=1 WEBCLI_TTY_WANT=7 $bin -test.run TestInteractiveFzfOnTTY -test.v" /dev/null |
  sed 's/\x1b\[[0-9;?$]*[a-zA-Z]//g' | tr -d '\r' | grep -aE "INTERACTIVE_FZF_SELECTED|fzf_test.go|^--- |^(PASS|FAIL)"
rm -f "$bin"
