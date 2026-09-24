# web-cli E2E report

- mode: real (headless: --headless)
- browser: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36
- date: 2026-09-24T01:38:30Z
- result: 54/57 checks passed

| # | Phase | Check | Result | Evidence |
|---|---|---|---|---|
| 1 | Phase 3 | dynamic root CA generated | ✅ PASS | ca.pem created, SPKI Pmm3aga2sXO+D6hb5dUeDZ5IvaHuo8Q+cch54dPAICE= |
| 2 | Phase 1 | real Chromium loaded extension/ (MV3 service worker running) | ✅ PASS | service worker "chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/background.js" |
| 3 | Phase 1 | nm-host started by extension and connected to app | ✅ PASS | hello with origin chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/ |
| 4 | Phase 1 | extension {type:ping} answered by Go with {type:pong} | ✅ PASS | app log: ping -> pong |
| 5 | Phase 1 | background.js received pong | ✅ PASS | extension reported pong_received back over the bridge |
| 6 | Phase 1 | app -> extension ping round trip | ✅ PASS | pong from extension in 2.593ms: {"from":"extension"} |
| 7 | Phase 2 | hints Login -> fzf -> click | ✅ PASS | fzf selected hint 2 \| click hint=2 <button> "Login" action_id=9a0b7276-7c83-4a63-91b0-e148ce0948f0 |
| 8 | Phase 2 | Vimium C visibility filter | ✅ PASS | visible=["Next page" "Login" "Who am I" "Search products" "English" "Role Button" "Onclick span"] missing=[] leaked-hidden=[] |
| 9 | Phase 2 | click fired in page (fetch /api/login ran) | ✅ PASS | #status="logged in: alice" |
| 10 | Phase 4 | X-Audit-Action-Id stripped before upstream | ✅ PASS | origin saw 1 POST /api/login, action header present=false, body={"user":"alice","password":"wonderland"} |
| 11 | Phase 4 | proxy linked POST /api/login to the click action | ✅ PASS | #3 POST https://127.0.0.1:46521/api/login -> 200 (27 bytes, 0s) action=9a0b7276-7c83-4a63-91b0-e148ce0948f0 |
| 12 | Phase 3 | rule add set-req-header | ✅ PASS | added rule #1 set-req-header url="/api/whoami" name="X-Injected" value="e2e-rule" |
| 13 | Phase 3 | rule add block | ✅ PASS | added rule #2 block url="/blocked" status=451 |
| 14 | Phase 3 | rule add set-resp-header | ✅ PASS | added rule #3 set-resp-header url="/api/whoami" name="X-Audited" value="web-cli" |
| 15 | Phase 4 | cookies dump via chrome.cookies.getAll | ✅ PASS | dumped 1 cookie(s) for https://127.0.0.1:46521/ to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/cookies.json; file has session=abc123 (HttpOnly,Secure)=true |
| 16 | Phase 4 | cookies import via chrome.cookies.set | ✅ PASS | imported 1/1 cookie(s) from /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/cookies-import.json |
| 17 | Phase 2 | hints 'who am' -> click | ✅ PASS | click hint=3 <button> "Who am I" action_id=53a51c33-2507-482d-beb8-e9cfe0f176fe |
| 18 | Phase 4 | imported cookie sent by the browser | ✅ PASS | origin received Cookie: "session=abc123; imported=from-json"; page shows "cookies: imported=from-json,session=abc123" |
| 19 | Phase 3 | request header injected by rule | ✅ PASS | origin received X-Injected: "e2e-rule" |
| 20 | Phase 3 | request blocked by rule (never reached origin) | ✅ PASS | page shows "blocked status: 451", origin saw 0 /blocked requests |
| 21 | Phase 3 | blocked + modified requests recorded with intercept notes | ✅ PASS | #4 GET https://127.0.0.1:46521/api/whoami -> 200 (74 bytes, 0s) action=53a51c33-2507-482d-beb8-e9cfe0f176fe intercepts=2 \| #5 GET https://127.0.0.1:46521/blocked -> 451 BLOCKED action=53a51c33-2507-482d-beb8-e9cfe0f176fe intercepts=1 |
| 22 | Phase 2 | hints 'next page' -> link click navigates | ✅ PASS | click hint=1 <a> "Next page" action_id=c639b468-8f03-47f2-b247-d2657e0eb514 |
| 23 | Phase 2 | navigation happened | ✅ PASS | document.title="Next" |
| 24 | Phase 4 | main_frame navigation tagged, header stripped upstream | ✅ PASS | origin saw 1 GET /next without action header |
| 25 | Phase 2 | content script re-injected after navigation; focus/click on new page | ✅ PASS | collected 2 visible element(s) on https://127.0.0.1:46521/next \| click hint=2 <button> "Press me" action_id=1106fcbc-98be-4440-8725-44eba2bb2c79 |
| 26 | Phase 2 | button on new page clicked | ✅ PASS | button text="clicked" |
| 27 | Phase 2 | stale page: 'search' has no match on new page | ✅ PASS | error: fzf: no element matches "search" |
| 28 | Phase 4 | export all captured requests to .http | ✅ PASS | wrote 6 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/requests.http |
| 29 | Phase 4 | export filtered by action ID | ✅ PASS | wrote 1 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/login-action.http |
| 30 | Phase 4 | navigation action has linked requests | ✅ PASS | wrote 1 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/nav-action.http |
| 31 | Phase 1 | status reports session and captures | ✅ PASS | sessions: 1 \| captured: 6 request(s), 4 action(s), 3 rule(s) |
| 32 | Phase 4 | .http file replays with curl | ✅ PASS | see harness.log |
| 33 | Automation | open <url> waits for the load | ✅ PASS | opened https://127.0.0.1:46521/login "Login" tab=1104616433 action_id=53ba240a-7f3d-4252-b04b-e3d4011f8eb2 |
| 34 | Automation | type <label text> <value> (target by <label>) | ✅ PASS | typed 17 char(s) into <input:email> "Email" selector=#email action_id=844f532b-4907-481c-87ac-1edce4835d2b |
| 35 | Automation | type password from $VAR, value never printed | ✅ PASS | typed 13 char(s) into <input:password> "Password" selector=#password action_id=316b9f61-d25e-4f2b-a9a3-175170580438; value in app output=false |
| 36 | Automation | select <select> option by text | ✅ PASS | selected "Japan" in <select> "Country: Japan" selector=#country action_id=caa0b93e-2c5d-42a5-881d-6db56f4af803 |
| 37 | Automation | check a checkbox (wrapping <label>) | ✅ PASS | checked <input:checkbox> "Remember me" checked=true selector=#remember action_id=0accd0c1-46f8-4677-955f-12d243240eb7 |
| 38 | Automation | press Enter in a field submits the form | ✅ PASS | pressed Enter on <input:password> "Password" action_id=09782ca4-be28-4c64-911b-8de42baf34a6 |
| 39 | Automation | waitfor text=... (after navigation) | ❌ FAIL | error: time: invalid duration "alice@example.com" |
| 40 | Automation | server received the filled form (POST /session) | ✅ PASS | body="email=alice%40example.com&password=E2E-s3cret-pw&country=jp&remember=on" action header upstream="" |
| 41 | Automation | form POST + redirect linked to the Enter action | ✅ PASS | #8 POST https://127.0.0.1:46521/session -> 303 (0 bytes, 0s) action=09782ca4-be28-4c64-911b-8de42baf34a6 \| #9 GET https://127.0.0.1:46521/dashboard?user=alice%40example.com -> 200 (336 bytes, 0s) action=09782ca4-be28-4c64-911b-8de42baf34a6 |
| 42 | Automation | show <id>: request headers + form body + response | ✅ PASS | #8 POST https://127.0.0.1:46521/session HTTP/1.1 \| action: 09782ca4-be28-4c64-911b-8de42baf34a6 press <input> "Enter on Password" on https://127.0.0.1:46521/login \| --- response 303 (0s) \| Location: /dashboard?user=alice%40example.com |
| 43 | Automation | body <id> <file>: HTML as served (before JavaScript) | ✅ PASS | wrote 336 byte(s) of the response body of #9 (text/html; charset=utf-8) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/dashboard-served.html |
| 44 | Automation | text: visible page text | ✅ PASS | Welcome alice@example.com \| rendered by js |
| 45 | Automation | source <file>: rendered DOM (after JavaScript) | ✅ PASS | saved rendered DOM of https://127.0.0.1:46521/dashboard?user=alice%40example.com (345 bytes) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/dashboard-rendered.html |
| 46 | Automation | eval <js> (page main world) | ✅ PASS | => "Dashboard"  (string, via main-world) |
| 47 | Automation | eval returns JSON values | ✅ PASS | => {"n":2,"path":"/dashboard","theme":"dark"}  (object, via main-world) |
| 48 | Automation | storage dump (localStorage + sessionStorage) | ✅ PASS | dumped 1 localStorage + 1 sessionStorage item(s) for https://127.0.0.1:46521 to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/storage.json \| local   theme=dark \| session tab=home |
| 49 | Automation | screenshot <file> (PNG) | ✅ PASS | saved screenshot of https://127.0.0.1:46521/dashboard?user=alice%40example.com (10465 bytes) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/dashboard.png |
| 50 | Automation | back | ❌ FAIL | back about:blank "about:blank" tab=1104616433 action_id=1292cea9-34b3-4142-9576-4a8988b5e553 |
| 51 | Automation | forward | ✅ PASS | forward https://127.0.0.1:46521/dashboard?user=alice%40example.com "Dashboard" tab=1104616433 action_id=5bc287f1-3f68-4d3a-b9f1-19abc4a5c0b5 |
| 52 | Automation | reload | ✅ PASS | reloaded https://127.0.0.1:46521/dashboard?user=alice%40example.com "Dashboard" tab=1104616433 action_id=330760b1-0231-4088-86ea-87bdea5b4777 |
| 53 | Automation | newtab / tabs / closetab | ❌ FAIL | opened https://127.0.0.1:46521/next "Next" tab=1104616434 action_id=c05dd8b7-d4f0-457e-a4a6-2a75a45d7929 \| 2 tab(s) ; 1104616433  https://127.0.0.1:46521/dashboard?user=alice%40example.com  "Dashboard" ; * 1104616434  https://127.0.0.1:46521/next  "Next" \| 1 tab(s) |
| 54 | Automation | eval under a strict CSP (debugger fallback) | ✅ PASS | => 42  (number, via debugger) |
| 55 | Automation | record start | ✅ PASS | recording to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/login.webcli (starting at https://127.0.0.1:46521/login): use the browser normally and/or web-cli commands; 'record stop' ends it |
| 56 | Automation | recording of real user input | ✅ PASS | err=<nil> recording stopped: 5 step(s) written to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/login.webcli; script: # web-cli recording 2026-09-24T01:38:28Z # replay: bin/app -f login.webcli   (or 'run login.webcli' in the REPL) # password fields are recorded as $WEBCLI_PASSWORD: export it before replaying open https://127.0.0.1:46521/login # type <input:email> "Email" on https://127.0.0.1:46521/login type css=#email bob@example.com # type <input:password> "Password" on https://127.0.0.1:46521/login type css=#password $WEBCLI_PASSWORD # check <input:checkbox> "Remember me" on https://127.0.0.1:46521/login check css=#remember # click <button> "Sign in" on https://127.0.0.1:46521/login click "css=#loginform > button:nth-of-type(1)"  |
| 57 | Automation | run <recording> replays it ($WEBCLI_PASSWORD from env) | ✅ PASS | run /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/login.webcli: 5 step(s) OK in 45ms; server got "email=bob%40example.com&password=replay-pw-42&country=us&remember=on" |
