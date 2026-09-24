#!/usr/bin/env bash
# Phase 3 evidence with the real curl CLI against a real HTTPS site:
#   curl -x <app proxy> --cacert <generated CA> https://...
# shows (1) the certificate is issued on the fly by the web-cli CA,
# (2) request/response capture, (3) header injection by rules,
# (4) X-Audit-Action-Id linkage + stripping, (5) blocking, (6) .http export.
set -euo pipefail
cd "$(dirname "$0")/.."
TARGET=${1:-https://github.com/robots.txt}
[[ -x bin/app ]] || scripts/build.sh >/dev/null
tmp=$(mktemp -d)
port=$(( 20000 + RANDOM % 20000 ))
fifo="$tmp/cmd"; mkfifo "$fifo"
bin/app -socket "$tmp/ipc.sock" -proxy "127.0.0.1:$port" -ca-dir "$tmp/ca" -out "$tmp/audit" <"$fifo" >"$tmp/app.log" 2>&1 &
app=$!
exec 3>"$fifo"
trap 'echo quit >&3 2>/dev/null || true; wait $app 2>/dev/null || true; rm -rf "$tmp"' EXIT
for _ in $(seq 50); do grep -q "proxy\] listening" "$tmp/app.log" && break; sleep 0.1; done

host=$(echo "$TARGET" | sed -E 's#https?://([^/:]+).*#\1#')
echo "rule add set-req-header host=^${host//./\\.}\$ name=X-Web-Cli-Test value=injected" >&3
echo "rule add set-resp-header host=^${host//./\\.}\$ name=X-Audited value=web-cli" >&3
echo "rule add block url=/web-cli-blocked status=451" >&3
sleep 0.3

echo "== 1. HTTPS through the MITM proxy (certificate chain as seen by curl)"
curl -sS -v -o /dev/null -x "http://127.0.0.1:$port" --cacert "$tmp/ca/ca.pem" \
  -H "X-Audit-Action-Id: demo-action-0001" "$TARGET" 2>&1 |
  grep -E "subject:|issuer:|SSL certificate verify|^< HTTP|^< x-audited|^< X-Audited" || true
echo
echo "== 2. blocked by rule"
curl -sS -o "$tmp/blocked.txt" -w 'HTTP status: %{http_code}\n' -x "http://127.0.0.1:$port" --cacert "$tmp/ca/ca.pem" "https://${host}/web-cli-blocked"
echo "body: $(cat "$tmp/blocked.txt")"
echo
echo "== 3. CA download from the proxy itself"
curl -sS "http://127.0.0.1:$port/ca.pem" | head -1
sleep 0.5
echo "log 10" >&3
echo "export $tmp/audit/demo.http" >&3
sleep 0.5
echo
echo "== 4. app log"
sed -E 's/^[0-9:.]+ //' "$tmp/app.log"
echo
echo "== 5. exported .http"
cat "$tmp/audit/demo.http"
echo
echo "== 6. X-Audit-Action-Id never forwarded (captured request headers of the linked exchange)"
grep '"action_id":"demo-action-0001"' "$tmp/audit/audit.jsonl" | head -1 |
  python3 -c 'import json,sys; r=json.loads(sys.stdin.read()); print("action_id:", r["action_id"]); print("forwarded headers:", sorted(r["request_headers"].keys()))'
