# web-cli E2E report

- mode: real
- browser: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36
- date: 2026-09-24T00:57:25Z
- result: 7/32 checks passed

| # | Phase | Check | Result | Evidence |
|---|---|---|---|---|
| 1 | Phase 3 | dynamic root CA generated | ✅ PASS | ca.pem created, SPKI Iz2drKry5hiqfGTsm/LBirEh95QCt8jjTbLex6EL00M= |
| 2 | Phase 1 | real Chromium loaded extension/ (MV3 service worker running) | ✅ PASS | service worker "chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/background.js" |
| 3 | Phase 1 | nm-host started by extension and connected to app | ❌ FAIL | hello with origin chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/ |
| 4 | Phase 1 | extension {type:ping} answered by Go with {type:pong} | ❌ FAIL | app log: ping -> pong |
| 5 | Phase 1 | background.js received pong | ❌ FAIL | extension reported pong_received back over the bridge |
| 6 | Phase 1 | app -> extension ping round trip | ❌ FAIL | error: no browser session connected: context deadline exceeded |
| 7 | Phase 2 | hints Login -> fzf -> click | ❌ FAIL |  |
| 8 | Phase 2 | Vimium C visibility filter | ❌ FAIL | visible=[] missing=["English" "Login" "Next page" "Onclick span" "Role Button" "Search products" "Who am I"] leaked-hidden=[] |
| 9 | Phase 2 | click fired in page (fetch /api/login ran) | ❌ FAIL | #status="idle" |
| 10 | Phase 4 | X-Audit-Action-Id stripped before upstream | ❌ FAIL | origin saw 0 POST /api/login, action header present=false, body= |
| 11 | Phase 4 | proxy linked POST /api/login to the click action | ❌ FAIL |  |
| 12 | Phase 3 | rule add set-req-header | ✅ PASS | added rule #1 set-req-header url="/api/whoami" name="X-Injected" value="e2e-rule" |
| 13 | Phase 3 | rule add block | ✅ PASS | added rule #2 block url="/blocked" status=451 |
| 14 | Phase 3 | rule add set-resp-header | ✅ PASS | added rule #3 set-resp-header url="/api/whoami" name="X-Audited" value="web-cli" |
| 15 | Phase 4 | cookies dump via chrome.cookies.getAll | ❌ FAIL | error: no browser session connected: context deadline exceeded; file has session=abc123 (HttpOnly,Secure)=false |
| 16 | Phase 4 | cookies import via chrome.cookies.set | ❌ FAIL | error: no browser session connected: context deadline exceeded |
| 17 | Phase 2 | hints 'who am' -> click | ❌ FAIL |  |
| 18 | Phase 4 | imported cookie sent by the browser | ❌ FAIL | origin received Cookie: ""; page shows "idle" |
| 19 | Phase 3 | request header injected by rule | ❌ FAIL | origin received X-Injected: "" |
| 20 | Phase 3 | request blocked by rule (never reached origin) | ❌ FAIL | page shows "", origin saw 0 /blocked requests |
| 21 | Phase 3 | blocked + modified requests recorded with intercept notes | ❌ FAIL |  |
| 22 | Phase 2 | hints 'next page' -> link click navigates | ❌ FAIL |  |
| 23 | Phase 2 | navigation happened | ❌ FAIL | document.title="web-cli E2E dummy" |
| 24 | Phase 4 | main_frame navigation tagged, header stripped upstream | ❌ FAIL | origin saw 0 GET /next without action header |
| 25 | Phase 2 | content script re-injected after navigation; focus/click on new page | ❌ FAIL |  |
| 26 | Phase 2 | button on new page clicked | ❌ FAIL | button text="Login" |
| 27 | Phase 2 | stale page: 'search' has no match on new page | ❌ FAIL | error: no browser session connected: context deadline exceeded |
| 28 | Phase 4 | export all captured requests to .http | ❌ FAIL | wrote 2 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/requests.http |
| 29 | Phase 4 | export filtered by action ID | ❌ FAIL | wrote 2 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/login-action.http |
| 30 | Phase 4 | navigation action has linked requests | ✅ PASS | wrote 2 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/nav-action.http |
| 31 | Phase 1 | status reports session and captures | ✅ PASS | sessions: 0 \| captured: 2 request(s), 0 action(s), 3 rule(s) |
| 32 | Phase 4 | .http file replays with curl | ❌ FAIL | see harness.log |
