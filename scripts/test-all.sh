#!/usr/bin/env bash
# Full verification run. Writes evidence logs to docs/evidence/.
#   CHROME_PATH=/path/to/chromium scripts/test-all.sh
set -uo pipefail
cd "$(dirname "$0")/.."
ev=docs/evidence; mkdir -p "$ev"
status=0
step() { echo; echo "######## $1"; }

step "build (go vet + go build ./... + binaries)"
{ go version; scripts/build.sh; } 2>&1 | tee "$ev/build.log" || status=1

step "unit + integration tests (race detector)"
go test -race -count=1 -v ./... 2>&1 | tee "$ev/go-test.log"
[[ ${PIPESTATUS[0]} -eq 0 ]] || status=1

step "interactive fzf on a pseudo terminal"
scripts/test-tty.sh 2>&1 | tee "$ev/tty-fzf.log"
grep -q "^--- PASS" "$ev/tty-fzf.log" || status=1

step "proxy demo with curl -x against a real HTTPS site"
scripts/proxy-demo.sh 2>&1 | tee "$ev/proxy-curl.log" || status=1

if [[ -n "${CHROME_PATH:-}" ]]; then
  step "end-to-end: Chromium + extension JS + nm-host + app + fzf + proxy"
  go run ./scripts/e2e 2>&1 | tee "$ev/e2e.log"
  [[ ${PIPESTATUS[0]} -eq 0 ]] || status=1
  cp scripts/e2e/out/e2e-report.md "$ev/e2e-report.md"
  cp scripts/e2e/out/app-transcript.log "$ev/e2e-app-transcript.log"
  cp scripts/e2e/out/audit/requests.http "$ev/sample-requests.http"
  cp scripts/e2e/out/audit/cookies.json "$ev/sample-cookies.json"
else
  echo "CHROME_PATH not set: skipping e2e" | tee "$ev/e2e.log"
fi

echo
[[ $status -eq 0 ]] && echo "ALL CHECKS PASSED" || echo "SOME CHECKS FAILED"
exit $status
