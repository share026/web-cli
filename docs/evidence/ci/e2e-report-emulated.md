# web-cli E2E report

- mode: emulated (headless: --headless)
- browser: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36
- date: 2026-09-24T02:50:48Z
- result: 31/31 checks passed

| # | Phase | Check | Result | Evidence |
|---|---|---|---|---|
| 1 | Phase 3 | dynamic root CA generated | ✅ PASS | ca.pem created, SPKI 7wg99kbXXpZIidsXQcU4q5X5VKaVP/ZKokTFoWOY+5Q= |
| 2 | Phase 1 | nm-host started by extension and connected to app | ✅ PASS | hello with origin chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/ |
| 3 | Phase 1 | extension {type:ping} answered by Go with {type:pong} | ✅ PASS | app log: ping -> pong |
| 4 | Phase 1 | background.js received pong | ✅ PASS | extension reported pong_received back over the bridge |
| 5 | Phase 1 | app -> extension ping round trip | ✅ PASS | 2026/09/24 02:50:45 [002] WARN: Cannot read response from mitm'd server dial tcp: lookup web-cli-extension.invalid on 127.0.0.53:53: no such host |
| 6 | Phase 2 | hints Login -> fzf -> click | ✅ PASS | fzf selected hint 2 \| click hint=2 <button> "Login" action_id=d627364c-22f7-441e-a261-ab9998177529 |
| 7 | Phase 2 | Vimium C visibility filter | ✅ PASS | visible=["Next page" "Login" "Who am I" "Search products" "English" "Role Button" "Onclick span"] missing=[] leaked-hidden=[] |
| 8 | Phase 2 | click fired in page (fetch /api/login ran) | ✅ PASS | #status="logged in: alice" |
| 9 | Phase 4 | X-Audit-Action-Id stripped before upstream | ✅ PASS | origin saw 1 POST /api/login, action header present=false, body={"user":"alice","password":"wonderland"} |
| 10 | Phase 4 | proxy linked POST /api/login to the click action | ✅ PASS | #4 POST https://127.0.0.1:36833/api/login -> 200 (27 bytes, 1ms) action=d627364c-22f7-441e-a261-ab9998177529 |
| 11 | Phase 3 | rule add set-req-header | ✅ PASS | added rule #1 set-req-header url="/api/whoami" name="X-Injected" value="e2e-rule" |
| 12 | Phase 3 | rule add block | ✅ PASS | added rule #2 block url="/blocked" status=451 |
| 13 | Phase 3 | rule add set-resp-header | ✅ PASS | added rule #3 set-resp-header url="/api/whoami" name="X-Audited" value="web-cli" |
| 14 | Phase 4 | cookies dump via chrome.cookies.getAll | ✅ PASS | dumped 1 cookie(s) for https://127.0.0.1:36833/ to /home/runner/work/web-cli/web-cli/scripts/e2e/out-emulated/audit/cookies.json; file has session=abc123 (HttpOnly,Secure)=true |
| 15 | Phase 4 | cookies import via chrome.cookies.set | ✅ PASS | imported 1/1 cookie(s) from /home/runner/work/web-cli/web-cli/scripts/e2e/out-emulated/audit/cookies-import.json |
| 16 | Phase 2 | hints 'who am' -> click | ✅ PASS | click hint=3 <button> "Who am I" action_id=83b5ab54-b984-47c3-b7d7-c7381a1d74b7 |
| 17 | Phase 4 | imported cookie sent by the browser | ✅ PASS | origin received Cookie: "session=abc123; imported=from-json"; page shows "cookies: imported=from-json,session=abc123" |
| 18 | Phase 3 | request header injected by rule | ✅ PASS | origin received X-Injected: "e2e-rule" |
| 19 | Phase 3 | request blocked by rule (never reached origin) | ✅ PASS | page shows "blocked status: 451", origin saw 0 /blocked requests |
| 20 | Phase 3 | blocked + modified requests recorded with intercept notes | ✅ PASS | #5 GET https://127.0.0.1:36833/api/whoami -> 200 (74 bytes, 1ms) action=83b5ab54-b984-47c3-b7d7-c7381a1d74b7 intercepts=2 \| #6 GET https://127.0.0.1:36833/blocked -> 451 BLOCKED action=83b5ab54-b984-47c3-b7d7-c7381a1d74b7 intercepts=1 |
| 21 | Phase 2 | hints 'next page' -> link click navigates | ✅ PASS | click hint=1 <a> "Next page" action_id=87091760-5b05-4a08-b42a-2836f43b0be2 |
| 22 | Phase 2 | navigation happened | ✅ PASS | document.title="Next" |
| 23 | Phase 4 | main_frame navigation tagged, header stripped upstream | ✅ PASS | origin saw 1 GET /next without action header |
| 24 | Phase 2 | content script re-injected after navigation; focus/click on new page | ✅ PASS | collected 2 visible element(s) on https://127.0.0.1:36833/next \| click hint=2 <button> "Press me" action_id=f3137d2b-82cc-4027-bae9-49cc00f44414 |
| 25 | Phase 2 | button on new page clicked | ✅ PASS | button text="clicked" |
| 26 | Phase 2 | stale page: 'search' has no match on new page | ✅ PASS | error: fzf: no element matches "search" |
| 27 | Phase 4 | export all captured requests to .http | ✅ PASS | wrote 7 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-emulated/audit/requests.http |
| 28 | Phase 4 | export filtered by action ID | ✅ PASS | wrote 1 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-emulated/audit/login-action.http |
| 29 | Phase 4 | navigation action has linked requests | ✅ PASS | wrote 1 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-emulated/audit/nav-action.http |
| 30 | Phase 1 | status reports session and captures | ✅ PASS | sessions: 1 \| captured: 7 request(s), 4 action(s), 3 rule(s) |
| 31 | Phase 4 | .http file replays with curl | ✅ PASS | see harness.log |
