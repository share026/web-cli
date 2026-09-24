# web-cli E2E report

- mode: real (headed)
- browser: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36
- date: 2026-09-24T01:38:01Z
- result: 54/57 checks passed

| # | Phase | Check | Result | Evidence |
|---|---|---|---|---|
| 1 | Phase 3 | dynamic root CA generated | ✅ PASS | ca.pem created, SPKI 43qJtjeBeUYWAUrnxIf5TPwfdPo+j3mJ3lsle9X50c4= |
| 2 | Phase 1 | real Chromium loaded extension/ (MV3 service worker running) | ✅ PASS | service worker "chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/background.js" |
| 3 | Phase 1 | nm-host started by extension and connected to app | ✅ PASS | hello with origin chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/ |
| 4 | Phase 1 | extension {type:ping} answered by Go with {type:pong} | ✅ PASS | app log: ping -> pong |
| 5 | Phase 1 | background.js received pong | ✅ PASS | extension reported pong_received back over the bridge |
| 6 | Phase 1 | app -> extension ping round trip | ✅ PASS | pong from extension in 1.309ms: {"from":"extension"} |
| 7 | Phase 2 | hints Login -> fzf -> click | ✅ PASS | fzf selected hint 2 \| click hint=2 <button> "Login" action_id=c6ef411b-8631-4edf-8206-18c04329e6ec |
| 8 | Phase 2 | Vimium C visibility filter | ✅ PASS | visible=["Next page" "Login" "Who am I" "Search products" "English" "Role Button" "Onclick span"] missing=[] leaked-hidden=[] |
| 9 | Phase 2 | click fired in page (fetch /api/login ran) | ✅ PASS | #status="logged in: alice" |
| 10 | Phase 4 | X-Audit-Action-Id stripped before upstream | ✅ PASS | origin saw 1 POST /api/login, action header present=false, body={"user":"alice","password":"wonderland"} |
| 11 | Phase 4 | proxy linked POST /api/login to the click action | ✅ PASS | #3 POST https://127.0.0.1:44021/api/login -> 200 (27 bytes, 0s) action=c6ef411b-8631-4edf-8206-18c04329e6ec |
| 12 | Phase 3 | rule add set-req-header | ✅ PASS | added rule #1 set-req-header url="/api/whoami" name="X-Injected" value="e2e-rule" |
| 13 | Phase 3 | rule add block | ✅ PASS | added rule #2 block url="/blocked" status=451 |
| 14 | Phase 3 | rule add set-resp-header | ✅ PASS | added rule #3 set-resp-header url="/api/whoami" name="X-Audited" value="web-cli" |
| 15 | Phase 4 | cookies dump via chrome.cookies.getAll | ✅ PASS | dumped 1 cookie(s) for https://127.0.0.1:44021/ to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/cookies.json; file has session=abc123 (HttpOnly,Secure)=true |
| 16 | Phase 4 | cookies import via chrome.cookies.set | ✅ PASS | imported 1/1 cookie(s) from /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/cookies-import.json |
| 17 | Phase 2 | hints 'who am' -> click | ✅ PASS | click hint=3 <button> "Who am I" action_id=ffdaaf87-6fb5-4368-b8e1-7c4992836293 |
| 18 | Phase 4 | imported cookie sent by the browser | ✅ PASS | origin received Cookie: "session=abc123; imported=from-json"; page shows "cookies: imported=from-json,session=abc123" |
| 19 | Phase 3 | request header injected by rule | ✅ PASS | origin received X-Injected: "e2e-rule" |
| 20 | Phase 3 | request blocked by rule (never reached origin) | ✅ PASS | page shows "blocked status: 451", origin saw 0 /blocked requests |
| 21 | Phase 3 | blocked + modified requests recorded with intercept notes | ✅ PASS | #4 GET https://127.0.0.1:44021/api/whoami -> 200 (74 bytes, 0s) action=ffdaaf87-6fb5-4368-b8e1-7c4992836293 intercepts=2 \| #5 GET https://127.0.0.1:44021/blocked -> 451 BLOCKED action=ffdaaf87-6fb5-4368-b8e1-7c4992836293 intercepts=1 |
| 22 | Phase 2 | hints 'next page' -> link click navigates | ✅ PASS | click hint=1 <a> "Next page" action_id=a91d471c-6328-46ab-9f6f-25f360d99ed5 |
| 23 | Phase 2 | navigation happened | ✅ PASS | document.title="Next" |
| 24 | Phase 4 | main_frame navigation tagged, header stripped upstream | ✅ PASS | origin saw 1 GET /next without action header |
| 25 | Phase 2 | content script re-injected after navigation; focus/click on new page | ✅ PASS | collected 2 visible element(s) on https://127.0.0.1:44021/next \| click hint=2 <button> "Press me" action_id=d3bdc5c7-b8d2-4091-8b06-81b1cdd1ed4c |
| 26 | Phase 2 | button on new page clicked | ✅ PASS | button text="clicked" |
| 27 | Phase 2 | stale page: 'search' has no match on new page | ✅ PASS | error: fzf: no element matches "search" |
| 28 | Phase 4 | export all captured requests to .http | ✅ PASS | wrote 6 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/requests.http |
| 29 | Phase 4 | export filtered by action ID | ✅ PASS | wrote 1 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/login-action.http |
| 30 | Phase 4 | navigation action has linked requests | ✅ PASS | wrote 1 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/nav-action.http |
| 31 | Phase 1 | status reports session and captures | ✅ PASS | sessions: 1 \| captured: 6 request(s), 4 action(s), 3 rule(s) |
| 32 | Phase 4 | .http file replays with curl | ✅ PASS | see harness.log |
| 33 | Automation | open <url> waits for the load | ✅ PASS | opened https://127.0.0.1:44021/login "Login" tab=55605265 action_id=9570760d-c868-4214-abe0-d1cbfeccc45e |
| 34 | Automation | type <label text> <value> (target by <label>) | ✅ PASS | typed 17 char(s) into <input:email> "Email" selector=#email action_id=a8d99fa1-23a0-4180-a1cf-74415af13a3d |
| 35 | Automation | type password from $VAR, value never printed | ✅ PASS | typed 13 char(s) into <input:password> "Password" selector=#password action_id=92caf90a-611d-4c7c-815d-291b7e7ecc1a; value in app output=false |
| 36 | Automation | select <select> option by text | ✅ PASS | selected "Japan" in <select> "Country: Japan" selector=#country action_id=ca436f46-1537-4769-a494-490ecfc9bf31 |
| 37 | Automation | check a checkbox (wrapping <label>) | ✅ PASS | checked <input:checkbox> "Remember me" checked=true selector=#remember action_id=a07d6dee-164e-4e39-a267-35c3133584d3 |
| 38 | Automation | press Enter in a field submits the form | ✅ PASS | pressed Enter on <input:password> "Password" action_id=a89a4956-42a2-4a16-98cc-84ce476cc9a6 |
| 39 | Automation | waitfor text=... (after navigation) | ❌ FAIL | error: time: invalid duration "alice@example.com" |
| 40 | Automation | server received the filled form (POST /session) | ✅ PASS | body="email=alice%40example.com&password=E2E-s3cret-pw&country=jp&remember=on" action header upstream="" |
| 41 | Automation | form POST + redirect linked to the Enter action | ✅ PASS | #8 POST https://127.0.0.1:44021/session -> 303 (0 bytes, 0s) action=a89a4956-42a2-4a16-98cc-84ce476cc9a6 \| #9 GET https://127.0.0.1:44021/dashboard?user=alice%40example.com -> 200 (336 bytes, 0s) action=a89a4956-42a2-4a16-98cc-84ce476cc9a6 |
| 42 | Automation | show <id>: request headers + form body + response | ✅ PASS | #8 POST https://127.0.0.1:44021/session HTTP/1.1 \| action: a89a4956-42a2-4a16-98cc-84ce476cc9a6 press <input> "Enter on Password" on https://127.0.0.1:44021/login \| --- response 303 (0s) \| Location: /dashboard?user=alice%40example.com |
| 43 | Automation | body <id> <file>: HTML as served (before JavaScript) | ✅ PASS | wrote 336 byte(s) of the response body of #9 (text/html; charset=utf-8) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/dashboard-served.html |
| 44 | Automation | text: visible page text | ✅ PASS | Welcome alice@example.com \| rendered by js |
| 45 | Automation | source <file>: rendered DOM (after JavaScript) | ✅ PASS | saved rendered DOM of https://127.0.0.1:44021/dashboard?user=alice%40example.com (345 bytes) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/dashboard-rendered.html |
| 46 | Automation | eval <js> (page main world) | ✅ PASS | => "Dashboard"  (string, via main-world) |
| 47 | Automation | eval returns JSON values | ✅ PASS | => {"n":2,"path":"/dashboard","theme":"dark"}  (object, via main-world) |
| 48 | Automation | storage dump (localStorage + sessionStorage) | ✅ PASS | dumped 1 localStorage + 1 sessionStorage item(s) for https://127.0.0.1:44021 to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/storage.json \| local   theme=dark \| session tab=home |
| 49 | Automation | screenshot <file> (PNG) | ✅ PASS | saved screenshot of https://127.0.0.1:44021/dashboard?user=alice%40example.com (11775 bytes) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/dashboard.png |
| 50 | Automation | back | ❌ FAIL | back about:blank "about:blank" tab=55605265 action_id=ef53c495-993e-46d9-a2e0-7d49416e9636 |
| 51 | Automation | forward | ✅ PASS | forward https://127.0.0.1:44021/dashboard?user=alice%40example.com "Dashboard" tab=55605265 action_id=4116a4eb-d698-4d68-aac1-5b2a6ad4d668 |
| 52 | Automation | reload | ✅ PASS | reloaded https://127.0.0.1:44021/dashboard?user=alice%40example.com "Dashboard" tab=55605265 action_id=9f75fef1-d0a0-4903-a2dd-89ebd8ea2735 |
| 53 | Automation | newtab / tabs / closetab | ❌ FAIL | opened https://127.0.0.1:44021/next "Next" tab=55605266 action_id=e2721257-15af-49d7-8525-8242c2ef159e \| 2 tab(s) ; 55605265  https://127.0.0.1:44021/dashboard?user=alice%40example.com  "Dashboard" ; * 55605266  https://127.0.0.1:44021/next  "Next" \| 1 tab(s) |
| 54 | Automation | eval under a strict CSP (debugger fallback) | ✅ PASS | => 42  (number, via debugger) |
| 55 | Automation | record start | ✅ PASS | recording to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/login.webcli (starting at https://127.0.0.1:44021/login): use the browser normally and/or web-cli commands; 'record stop' ends it |
| 56 | Automation | recording of real user input | ✅ PASS | err=<nil> recording stopped: 5 step(s) written to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/login.webcli; script: # web-cli recording 2026-09-24T01:37:59Z # replay: bin/app -f login.webcli   (or 'run login.webcli' in the REPL) # password fields are recorded as $WEBCLI_PASSWORD: export it before replaying open https://127.0.0.1:44021/login # type <input:email> "Email" on https://127.0.0.1:44021/login type css=#email bob@example.com # type <input:password> "Password" on https://127.0.0.1:44021/login type css=#password $WEBCLI_PASSWORD # check <input:checkbox> "Remember me" on https://127.0.0.1:44021/login check css=#remember # click <button> "Sign in" on https://127.0.0.1:44021/login click "css=#loginform > button:nth-of-type(1)"  |
| 57 | Automation | run <recording> replays it ($WEBCLI_PASSWORD from env) | ✅ PASS | run /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/login.webcli: 5 step(s) OK in 56ms; server got "email=bob%40example.com&password=replay-pw-42&country=us&remember=on" |
