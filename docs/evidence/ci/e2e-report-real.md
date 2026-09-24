# web-cli E2E report

- mode: real (headed)
- browser: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36
- date: 2026-09-24T02:46:51Z
- result: 68/72 checks passed

| # | Phase | Check | Result | Evidence |
|---|---|---|---|---|
| 1 | Phase 3 | dynamic root CA generated | ✅ PASS | ca.pem created, SPKI BPUm8mrj94Tf0fCO5PqYNjTJYfgOdRY0074LHLlSGck= |
| 2 | Phase 1 | real Chromium loaded extension/ (MV3 service worker running) | ✅ PASS | service worker "chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/background.js" |
| 3 | Phase 1 | nm-host started by extension and connected to app | ✅ PASS | hello with origin chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/ |
| 4 | Phase 1 | extension {type:ping} answered by Go with {type:pong} | ✅ PASS | app log: ping -> pong |
| 5 | Phase 1 | background.js received pong | ✅ PASS | extension reported pong_received back over the bridge |
| 6 | Phase 1 | app -> extension ping round trip | ✅ PASS | pong from extension in 2.982ms: {"from":"extension"} |
| 7 | Phase 2 | hints Login -> fzf -> click | ✅ PASS | fzf selected hint 2 \| click hint=2 <button> "Login" action_id=5acfeaf1-4d8f-47d2-bb48-6abaccac511c |
| 8 | Phase 2 | Vimium C visibility filter | ✅ PASS | visible=["Next page" "Login" "Who am I" "Search products" "English" "Role Button" "Onclick span"] missing=[] leaked-hidden=[] |
| 9 | Phase 2 | click fired in page (fetch /api/login ran) | ✅ PASS | #status="logged in: alice" |
| 10 | Phase 4 | X-Audit-Action-Id stripped before upstream | ✅ PASS | origin saw 1 POST /api/login, action header present=false, body={"user":"alice","password":"wonderland"} |
| 11 | Phase 4 | proxy linked POST /api/login to the click action | ✅ PASS | #3 POST https://127.0.0.1:35943/api/login -> 200 (27 bytes, 1ms) action=5acfeaf1-4d8f-47d2-bb48-6abaccac511c |
| 12 | Phase 3 | rule add set-req-header | ✅ PASS | added rule #1 set-req-header url="/api/whoami" name="X-Injected" value="e2e-rule" |
| 13 | Phase 3 | rule add block | ✅ PASS | added rule #2 block url="/blocked" status=451 |
| 14 | Phase 3 | rule add set-resp-header | ✅ PASS | added rule #3 set-resp-header url="/api/whoami" name="X-Audited" value="web-cli" |
| 15 | Phase 4 | cookies dump via chrome.cookies.getAll | ✅ PASS | dumped 1 cookie(s) for https://127.0.0.1:35943/ to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/cookies.json; file has session=abc123 (HttpOnly,Secure)=true |
| 16 | Phase 4 | cookies import via chrome.cookies.set | ✅ PASS | imported 1/1 cookie(s) from /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/cookies-import.json |
| 17 | Phase 2 | hints 'who am' -> click | ✅ PASS | click hint=3 <button> "Who am I" action_id=06579ee1-003e-441b-b53a-05e622ee5ef2 |
| 18 | Phase 4 | imported cookie sent by the browser | ✅ PASS | origin received Cookie: "session=abc123; imported=from-json"; page shows "cookies: imported=from-json,session=abc123" |
| 19 | Phase 3 | request header injected by rule | ✅ PASS | origin received X-Injected: "e2e-rule" |
| 20 | Phase 3 | request blocked by rule (never reached origin) | ✅ PASS | page shows "blocked status: 451", origin saw 0 /blocked requests |
| 21 | Phase 3 | blocked + modified requests recorded with intercept notes | ✅ PASS | #4 GET https://127.0.0.1:35943/api/whoami -> 200 (74 bytes, 1ms) action=06579ee1-003e-441b-b53a-05e622ee5ef2 intercepts=2 \| #5 GET https://127.0.0.1:35943/blocked -> 451 BLOCKED action=06579ee1-003e-441b-b53a-05e622ee5ef2 intercepts=1 |
| 22 | Phase 2 | hints 'next page' -> link click navigates | ✅ PASS | click hint=1 <a> "Next page" action_id=6b222336-2fcc-417b-afd4-7fc8c7a687fa |
| 23 | Phase 2 | navigation happened | ✅ PASS | document.title="Next" |
| 24 | Phase 4 | main_frame navigation tagged, header stripped upstream | ✅ PASS | origin saw 1 GET /next without action header |
| 25 | Phase 2 | content script re-injected after navigation; focus/click on new page | ✅ PASS | collected 2 visible element(s) on https://127.0.0.1:35943/next \| click hint=2 <button> "Press me" action_id=e3b9bd0b-c853-4da3-991b-b4f3741b807a |
| 26 | Phase 2 | button on new page clicked | ✅ PASS | button text="clicked" |
| 27 | Phase 2 | stale page: 'search' has no match on new page | ✅ PASS | error: fzf: no element matches "search" |
| 28 | Phase 4 | export all captured requests to .http | ✅ PASS | wrote 6 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/requests.http |
| 29 | Phase 4 | export filtered by action ID | ✅ PASS | wrote 1 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/login-action.http |
| 30 | Phase 4 | navigation action has linked requests | ✅ PASS | wrote 1 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/nav-action.http |
| 31 | Phase 1 | status reports session and captures | ✅ PASS | sessions: 1 \| captured: 6 request(s), 4 action(s), 3 rule(s) |
| 32 | Phase 4 | .http file replays with curl | ✅ PASS | see harness.log |
| 33 | Automation | open <url> waits for the load | ✅ PASS | opened https://127.0.0.1:35943/login "Login" tab=873367710 action_id=24a0cb46-0ff4-4758-aff3-099f607d5d12 |
| 34 | Automation | type <label text> <value> (target by <label>) | ✅ PASS | typed 17 char(s) into <input:email> "Email" selector=#email action_id=d76d6978-094f-4415-87ab-547c1cfeb05b |
| 35 | Automation | type password from $VAR, value never printed | ✅ PASS | typed 13 char(s) into <input:password> "Password" selector=#password action_id=5ff08c9b-77e1-4e16-bc87-673ed7c5e002; value in app output=false |
| 36 | Automation | select <select> option by text | ✅ PASS | selected "Japan" in <select> "Country: Japan" selector=#country action_id=ca898ca3-7905-499b-be52-9cdef42fd126 |
| 37 | Automation | check a checkbox (wrapping <label>) | ✅ PASS | checked <input:checkbox> "Remember me" checked=true selector=#remember action_id=7c36e785-c841-4100-ad03-426673dfb62d |
| 38 | Automation | press Enter in a field submits the form | ✅ PASS | pressed Enter on <input:password> "Password" action_id=47aebedf-f89f-4cf5-9a59-e2e9386b6b05 |
| 39 | Automation | waitfor text=... (after navigation) | ✅ PASS | found text=Welcome alice@example.com after 2ms https://127.0.0.1:35943/dashboard?user=alice%40example.com |
| 40 | Automation | server received the filled form (POST /session) | ✅ PASS | body="email=alice%40example.com&password=E2E-s3cret-pw&country=jp&remember=on" action header upstream="" |
| 41 | Automation | form POST + redirect linked to the Enter action | ✅ PASS | #8 POST https://127.0.0.1:35943/session -> 303 (0 bytes, 0s) action=47aebedf-f89f-4cf5-9a59-e2e9386b6b05 \| #9 GET https://127.0.0.1:35943/dashboard?user=alice%40example.com -> 200 (336 bytes, 0s) action=47aebedf-f89f-4cf5-9a59-e2e9386b6b05 |
| 42 | Automation | show <id>: request headers + form body + response | ✅ PASS | #8 POST https://127.0.0.1:35943/session HTTP/1.1 \| action: 47aebedf-f89f-4cf5-9a59-e2e9386b6b05 press <input> "Enter on Password" on https://127.0.0.1:35943/login \| --- response 303 (0s) \| Location: /dashboard?user=alice%40example.com |
| 43 | Automation | body <id> <file>: HTML as served (before JavaScript) | ✅ PASS | wrote 336 byte(s) of the response body of #9 (text/html; charset=utf-8) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/dashboard-served.html |
| 44 | Automation | text: visible page text | ✅ PASS | Welcome alice@example.com \| rendered by js |
| 45 | Automation | source <file>: rendered DOM (after JavaScript) | ✅ PASS | saved rendered DOM of https://127.0.0.1:35943/dashboard?user=alice%40example.com (345 bytes) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/dashboard-rendered.html |
| 46 | Automation | eval <js> (page main world) | ✅ PASS | => "Dashboard"  (string, via main-world) |
| 47 | Automation | eval returns JSON values | ✅ PASS | => {"n":2,"path":"/dashboard","theme":"dark"}  (object, via main-world) |
| 48 | Automation | storage dump (localStorage + sessionStorage) | ✅ PASS | dumped 1 localStorage + 1 sessionStorage item(s) for https://127.0.0.1:35943 to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/storage.json \| local   theme=dark \| session tab=home |
| 49 | Automation | screenshot <file> (PNG) | ✅ PASS | saved screenshot of https://127.0.0.1:35943/dashboard?user=alice%40example.com (11775 bytes) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/dashboard.png |
| 50 | Automation | back | ✅ PASS | back https://127.0.0.1:35943/login "Login" tab=873367710 action_id=468d9a43-b5f6-4188-a003-c760a9598070 |
| 51 | Automation | forward | ✅ PASS | forward https://127.0.0.1:35943/dashboard?user=alice%40example.com "Dashboard" tab=873367710 action_id=8473ebea-e4b2-45b3-a454-87169434c200 |
| 52 | Automation | reload | ✅ PASS | reloaded https://127.0.0.1:35943/dashboard?user=alice%40example.com "Dashboard" tab=873367710 action_id=c59eb554-9474-49d0-8a9b-aaf8228c8f1e |
| 53 | Automation | newtab / tabs / closetab | ✅ PASS | opened https://127.0.0.1:35943/next "Next" tab=873367711 action_id=7897070e-ce7b-4e13-99c6-ef15739c93bc \| 2 tab(s) ; 873367710  https://127.0.0.1:35943/dashboard?user=alice%40example.com  "Dashboard" ; * 873367711  https://127.0.0.1:35943/next  "Next" \| 1 tab(s) |
| 54 | Automation | eval under a strict CSP (debugger fallback) | ✅ PASS | => 42  (number, via debugger) |
| 55 | Automation | record start | ✅ PASS | recording to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/login.webcli (starting at https://127.0.0.1:35943/login): use the browser normally and/or web-cli commands; 'record stop' ends it |
| 56 | Automation | recording of real user input | ✅ PASS | err=<nil> recording stopped: 5 step(s) written to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/login.webcli; script: # web-cli recording 2026-09-24T02:45:44Z # replay: bin/app -f login.webcli   (or 'run login.webcli' in the REPL) # password fields are recorded as $WEBCLI_PASSWORD: export it before replaying open https://127.0.0.1:35943/login # type <input:email> "Email" on https://127.0.0.1:35943/login type css=#email bob@example.com # type <input:password> "Password" on https://127.0.0.1:35943/login type css=#password $WEBCLI_PASSWORD # check <input:checkbox> "Remember me" on https://127.0.0.1:35943/login check css=#remember # click <button> "Sign in" on https://127.0.0.1:35943/login click "css=#loginform > button:nth-of-type(1)"  |
| 57 | Automation | run <recording> replays it ($WEBCLI_PASSWORD from env) | ✅ PASS | run /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/login.webcli: 5 step(s) OK in 48ms; server got "email=bob%40example.com&password=replay-pw-42&country=us&remember=on" |
| 58 | Phase 6 | open page with iframes | ✅ PASS | opened https://127.0.0.1:35943/frames "Frames" tab=873367710 action_id=9850b193-d132-4511-a18a-063a59d2d0ee |
| 59 | Phase 6 | list includes same-origin iframe, cross-origin iframe and shadow DOM controls | ❌ FAIL | same="" xo="" shadow="2  [button]  Shadow Save" |
| 60 | Phase 6 | type + click inside a same-origin iframe | ❌ FAIL | error: fzf: no element matches "Frame Name" \| error: fzf: no element matches "Frame Submit" \| server got [] |
| 61 | Phase 6 | cross-origin iframe: type, then 'press Enter' submits in the focused frame | ❌ FAIL | error: fzf: no element matches "XO Name" \| pressed Enter on <body> "Frames and shadow DOM" action_id=69b460e8-2d61-4faa-ba1d-90ccadb360af \| server got [] |
| 62 | Phase 6 | css= selector found in a cross-origin iframe | ❌ FAIL | error: extension: not found: css=#xo-btn \| requests=0 |
| 63 | Phase 6 | type + click inside an open shadow root (label resolved in the shadow tree) | ✅ PASS | typed 12 char(s) into <input:text> "Shadow input" selector=#shadow-in action_id=1b9cb6f0-6690-4da5-ad1c-b9c4a07d930b \| click <button> "Shadow Save" selector=#shadow-btn action_id=b936b159-e3d1-4848-8494-52c695f735aa \| page shows "saved: shadow-value" |
| 64 | Phase 6 | css= selector inside a shadow root | ✅ PASS | typed 7 char(s) into <input:text> "Shadow input" selector=#shadow-in action_id=77de3108-6fe1-46f3-bff9-1438d517ce0e \| value="via-css" |
| 65 | Phase 6 | upload 1.5 MB + text file via the label of a hidden <input type=file multiple> (chunked) | ✅ PASS | uploaded 2 file(s) (1500013 bytes: photo.png, notes.txt) into <input:file> "Choose files" selector=#file action_id=203143f0-9b8e-4fee-803a-9bcf911da6ce \| page change event saw "photo.png:1500000:image/png,notes.txt:13:text/plain" |
| 66 | Phase 6 | server received both files byte-identical (multipart POST, sha256) | ✅ PASS | title="Uploaded" server got [{Name:photo.png Type:image/png Size:1500000 SHA256:8efd299c629fa758a6e7b07b1701e65ef724a89629a149e2126e74eda38a3241} {Name:notes.txt Type:text/plain Size:13 SHA256:993a327368cc9a443f6d9a11d146da9e9ba2d561a8ef1e9190d119b2b1a002e0}] |
| 67 | Phase 6 | upload onto a drop zone (drag&drop events with the file) | ✅ PASS | dropped 1 file(s) (13 bytes: notes.txt) into <div> "Drop files here" selector=#drop action_id=636d4360-1281-4174-8e40-54d411afb5ef \| page saw "notes.txt:hello upload" |
| 68 | Phase 6 | WebSocket works through the MITM proxy and is listed by 'ws' | ✅ PASS | page="echo:hello-ws \| echo:{\"n\":2,\"text\":\"second message\"}" \| #26   closed    6 frame(s)  https://127.0.0.1:35943/ws |
| 69 | Phase 6 | ws <id>: frames in both directions, permessage-deflate inflated, close code | ✅ PASS | sent="02:46:49.134 → text        8B [deflate]  \"hello-ws\"" recv="02:46:49.136 ← text       13B [deflate]  \"echo:hello-ws\"" second="02:46:49.137 ← text       36B [deflate]  \"echo:{\\\"n\\\":2,\\\"text\\\":\\\"second message\\\"}\"" close="02:46:49.137 → close       5B [code=1000]  \"bye\"" |
| 70 | Phase 6 | rule throttle latency=700ms + 'timing' shows ttfb >= 700ms (DevTools-style throttling) | ✅ PASS | added rule #4 throttle url="/slow" latency=700ms kbps=2000 \| ttfb=705ms \| navigation navigate over http/1.1: redirect 0ms  dns 3ms  connect 0ms  tls 4ms  ttfb 705ms  download 129ms |
| 71 | Phase 6 | throttled request logged with the rule as intercept | ✅ PASS | #27 GET https://127.0.0.1:35943/slow -> 200 (32041 bytes, 831ms) action=91e15cd1-1860-435c-92c7-311e6965b76a intercepts=1 |
| 72 | Phase 6 | timing <file.json> saves the raw timing data | ✅ PASS | saved timing JSON (515 bytes) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/timing.json |
