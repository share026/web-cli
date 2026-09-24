#!/usr/bin/env bash
# Full verification run. Writes evidence logs to docs/evidence/.
#   CHROME_PATH=/path/to/chromium scripts/test-all.sh
set -uo pipefail
cd "$(dirname "$0")/.."
ev=${EVIDENCE_DIR:-docs/evidence}; mkdir -p "$ev"
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
  # E2E_MODES: "emulated" (default) and/or "real" (Chromium loads extension/ itself).
  for mode in ${E2E_MODES:-emulated}; do
    step "end-to-end ($mode): Chromium + extension + nm-host + app + fzf + proxy"
    out="scripts/e2e/out-$mode"
    # emulated: headless shell; real: headed (xvfb when E2E_HEADED=1);
    # real-headless: the real extension path under Chromium's --headless
    runner=(env E2E_HEADED=0)
    hmode=$mode
    if [[ "$mode" == real && "${E2E_HEADED:-}" == 1 ]] && command -v xvfb-run >/dev/null; then
      runner=(xvfb-run -a -s "-screen 0 1280x800x24")
    elif [[ "$mode" == real ]]; then
      runner=()
    elif [[ "$mode" == real-headless ]]; then
      hmode=real
    fi
    "${runner[@]}" go run ./scripts/e2e -mode "$hmode" -out "$out" 2>&1 | tee "$ev/e2e-$mode.log"
    [[ ${PIPESTATUS[0]} -eq 0 ]] || status=1
    cp "$out/e2e-report.md" "$ev/e2e-report-$mode.md" 2>/dev/null
    cp "$out/app-transcript.log" "$ev/e2e-app-transcript-$mode.log" 2>/dev/null
    cp "$out/nm-host.log" "$ev/e2e-nm-host-$mode.log" 2>/dev/null
    cp "$out/chrome.log" "$ev/e2e-chrome-$mode.log" 2>/dev/null
    cp "$out/sw-console.log" "$ev/e2e-sw-console-$mode.log" 2>/dev/null
    cp "$out/audit/requests.http" "$ev/sample-requests-$mode.http" 2>/dev/null
    cp "$out/audit/cookies.json" "$ev/sample-cookies-$mode.json" 2>/dev/null
  done
else
  echo "CHROME_PATH not set: skipping e2e" | tee "$ev/e2e.log"
fi

echo
[[ $status -eq 0 ]] && echo "ALL CHECKS PASSED" || echo "SOME CHECKS FAILED"
exit $status
