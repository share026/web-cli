# web-cli E2E report

- mode: real (headless: --headless)
- browser: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36
- date: 2026-09-24T02:48:05Z
- result: 68/72 checks passed

| # | Phase | Check | Result | Evidence |
|---|---|---|---|---|
| 1 | Phase 3 | dynamic root CA generated | ✅ PASS | ca.pem created, SPKI C4AP+h3Y5pkuply3tlG64Qk79IvxmG8Ujnojnes3xvU= |
| 2 | Phase 1 | real Chromium loaded extension/ (MV3 service worker running) | ✅ PASS | service worker "chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/background.js" |
| 3 | Phase 1 | nm-host started by extension and connected to app | ✅ PASS | hello with origin chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/ |
| 4 | Phase 1 | extension {type:ping} answered by Go with {type:pong} | ✅ PASS | app log: ping -> pong |
| 5 | Phase 1 | background.js received pong | ✅ PASS | extension reported pong_received back over the bridge |
| 6 | Phase 1 | app -> extension ping round trip | ✅ PASS | pong from extension in 880µs: {"from":"extension"} |
| 7 | Phase 2 | hints Login -> fzf -> click | ✅ PASS | fzf selected hint 2 \| click hint=2 <button> "Login" action_id=f15b3744-36dd-40e5-954f-66e0fb2a2863 |
| 8 | Phase 2 | Vimium C visibility filter | ✅ PASS | visible=["Next page" "Login" "Who am I" "Search products" "English" "Role Button" "Onclick span"] missing=[] leaked-hidden=[] |
| 9 | Phase 2 | click fired in page (fetch /api/login ran) | ✅ PASS | #status="logged in: alice" |
| 10 | Phase 4 | X-Audit-Action-Id stripped before upstream | ✅ PASS | origin saw 1 POST /api/login, action header present=false, body={"user":"alice","password":"wonderland"} |
| 11 | Phase 4 | proxy linked POST /api/login to the click action | ✅ PASS | #3 POST https://127.0.0.1:43311/api/login -> 200 (27 bytes, 0s) action=f15b3744-36dd-40e5-954f-66e0fb2a2863 |
| 12 | Phase 3 | rule add set-req-header | ✅ PASS | added rule #1 set-req-header url="/api/whoami" name="X-Injected" value="e2e-rule" |
| 13 | Phase 3 | rule add block | ✅ PASS | added rule #2 block url="/blocked" status=451 |
| 14 | Phase 3 | rule add set-resp-header | ✅ PASS | added rule #3 set-resp-header url="/api/whoami" name="X-Audited" value="web-cli" |
| 15 | Phase 4 | cookies dump via chrome.cookies.getAll | ✅ PASS | dumped 1 cookie(s) for https://127.0.0.1:43311/ to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/cookies.json; file has session=abc123 (HttpOnly,Secure)=true |
| 16 | Phase 4 | cookies import via chrome.cookies.set | ✅ PASS | imported 1/1 cookie(s) from /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/cookies-import.json |
| 17 | Phase 2 | hints 'who am' -> click | ✅ PASS | click hint=3 <button> "Who am I" action_id=7bd2b2c0-12b6-4057-9232-8f513bcea0d5 |
| 18 | Phase 4 | imported cookie sent by the browser | ✅ PASS | origin received Cookie: "session=abc123; imported=from-json"; page shows "cookies: imported=from-json,session=abc123" |
| 19 | Phase 3 | request header injected by rule | ✅ PASS | origin received X-Injected: "e2e-rule" |
| 20 | Phase 3 | request blocked by rule (never reached origin) | ✅ PASS | page shows "blocked status: 451", origin saw 0 /blocked requests |
| 21 | Phase 3 | blocked + modified requests recorded with intercept notes | ✅ PASS | #4 GET https://127.0.0.1:43311/api/whoami -> 200 (74 bytes, 0s) action=7bd2b2c0-12b6-4057-9232-8f513bcea0d5 intercepts=2 \| #5 GET https://127.0.0.1:43311/blocked -> 451 BLOCKED action=7bd2b2c0-12b6-4057-9232-8f513bcea0d5 intercepts=1 |
| 22 | Phase 2 | hints 'next page' -> link click navigates | ✅ PASS | click hint=1 <a> "Next page" action_id=020682b3-98e8-4ba7-8911-599f30df454c |
| 23 | Phase 2 | navigation happened | ✅ PASS | document.title="Next" |
| 24 | Phase 4 | main_frame navigation tagged, header stripped upstream | ✅ PASS | origin saw 1 GET /next without action header |
| 25 | Phase 2 | content script re-injected after navigation; focus/click on new page | ✅ PASS | collected 2 visible element(s) on https://127.0.0.1:43311/next \| click hint=2 <button> "Press me" action_id=3cc29983-aef5-4236-9174-bd53a80c1432 |
| 26 | Phase 2 | button on new page clicked | ✅ PASS | button text="clicked" |
| 27 | Phase 2 | stale page: 'search' has no match on new page | ✅ PASS | error: fzf: no element matches "search" |
| 28 | Phase 4 | export all captured requests to .http | ✅ PASS | wrote 6 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/requests.http |
| 29 | Phase 4 | export filtered by action ID | ✅ PASS | wrote 1 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/login-action.http |
| 30 | Phase 4 | navigation action has linked requests | ✅ PASS | wrote 1 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/nav-action.http |
| 31 | Phase 1 | status reports session and captures | ✅ PASS | sessions: 1 \| captured: 6 request(s), 4 action(s), 3 rule(s) |
| 32 | Phase 4 | .http file replays with curl | ✅ PASS | see harness.log |
| 33 | Automation | open <url> waits for the load | ✅ PASS | opened https://127.0.0.1:43311/login "Login" tab=2027235643 action_id=15d34c2b-5fe1-4ec0-a032-dcfa192d51f2 |
| 34 | Automation | type <label text> <value> (target by <label>) | ✅ PASS | typed 17 char(s) into <input:email> "Email" selector=#email action_id=def702f8-617c-479c-a487-6bb7d9fb95b0 |
| 35 | Automation | type password from $VAR, value never printed | ✅ PASS | typed 13 char(s) into <input:password> "Password" selector=#password action_id=e72f3637-a096-4b07-aa06-4b05b6103794; value in app output=false |
| 36 | Automation | select <select> option by text | ✅ PASS | selected "Japan" in <select> "Country: Japan" selector=#country action_id=a90bcc1b-d7b0-4104-8f35-440e08e6d6be |
| 37 | Automation | check a checkbox (wrapping <label>) | ✅ PASS | checked <input:checkbox> "Remember me" checked=true selector=#remember action_id=5d46045e-d57d-4a2d-a3fe-694f21702572 |
| 38 | Automation | press Enter in a field submits the form | ✅ PASS | pressed Enter on <input:password> "Password" action_id=bdd431c4-8eb5-4752-850d-486aa0735782 |
| 39 | Automation | waitfor text=... (after navigation) | ✅ PASS | found text=Welcome alice@example.com after 2ms https://127.0.0.1:43311/dashboard?user=alice%40example.com |
| 40 | Automation | server received the filled form (POST /session) | ✅ PASS | body="email=alice%40example.com&password=E2E-s3cret-pw&country=jp&remember=on" action header upstream="" |
| 41 | Automation | form POST + redirect linked to the Enter action | ✅ PASS | #8 POST https://127.0.0.1:43311/session -> 303 (0 bytes, 0s) action=bdd431c4-8eb5-4752-850d-486aa0735782 \| #9 GET https://127.0.0.1:43311/dashboard?user=alice%40example.com -> 200 (336 bytes, 0s) action=bdd431c4-8eb5-4752-850d-486aa0735782 |
| 42 | Automation | show <id>: request headers + form body + response | ✅ PASS | #8 POST https://127.0.0.1:43311/session HTTP/1.1 \| action: bdd431c4-8eb5-4752-850d-486aa0735782 press <input> "Enter on Password" on https://127.0.0.1:43311/login \| --- response 303 (0s) \| Location: /dashboard?user=alice%40example.com |
| 43 | Automation | body <id> <file>: HTML as served (before JavaScript) | ✅ PASS | wrote 336 byte(s) of the response body of #9 (text/html; charset=utf-8) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/dashboard-served.html |
| 44 | Automation | text: visible page text | ✅ PASS | Welcome alice@example.com \| rendered by js |
| 45 | Automation | source <file>: rendered DOM (after JavaScript) | ✅ PASS | saved rendered DOM of https://127.0.0.1:43311/dashboard?user=alice%40example.com (345 bytes) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/dashboard-rendered.html |
| 46 | Automation | eval <js> (page main world) | ✅ PASS | => "Dashboard"  (string, via main-world) |
| 47 | Automation | eval returns JSON values | ✅ PASS | => {"n":2,"path":"/dashboard","theme":"dark"}  (object, via main-world) |
| 48 | Automation | storage dump (localStorage + sessionStorage) | ✅ PASS | dumped 1 localStorage + 1 sessionStorage item(s) for https://127.0.0.1:43311 to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/storage.json \| local   theme=dark \| session tab=home |
| 49 | Automation | screenshot <file> (PNG) | ✅ PASS | saved screenshot of https://127.0.0.1:43311/dashboard?user=alice%40example.com (10465 bytes) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/dashboard.png |
| 50 | Automation | back | ✅ PASS | back https://127.0.0.1:43311/login "Login" tab=2027235643 action_id=5f18d7e4-dfb1-4894-9cad-52c95cd0323c |
| 51 | Automation | forward | ✅ PASS | forward https://127.0.0.1:43311/dashboard?user=alice%40example.com "Dashboard" tab=2027235643 action_id=eeaaa270-6d85-4928-b2d7-4422b16d8de3 |
| 52 | Automation | reload | ✅ PASS | reloaded https://127.0.0.1:43311/dashboard?user=alice%40example.com "Dashboard" tab=2027235643 action_id=e6120d86-80d4-41ca-9941-21a5fb5b7798 |
| 53 | Automation | newtab / tabs / closetab | ✅ PASS | opened https://127.0.0.1:43311/next "Next" tab=2027235644 action_id=0453b26a-68c2-45b1-82a9-974dd1a354f3 \| 2 tab(s) ; 2027235643  https://127.0.0.1:43311/dashboard?user=alice%40example.com  "Dashboard" ; * 2027235644  https://127.0.0.1:43311/next  "Next" \| 1 tab(s) |
| 54 | Automation | eval under a strict CSP (debugger fallback) | ✅ PASS | => 42  (number, via debugger) |
| 55 | Automation | record start | ✅ PASS | recording to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/login.webcli (starting at https://127.0.0.1:43311/login): use the browser normally and/or web-cli commands; 'record stop' ends it |
| 56 | Automation | recording of real user input | ✅ PASS | err=<nil> recording stopped: 5 step(s) written to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/login.webcli; script: # web-cli recording 2026-09-24T02:46:58Z # replay: bin/app -f login.webcli   (or 'run login.webcli' in the REPL) # password fields are recorded as $WEBCLI_PASSWORD: export it before replaying open https://127.0.0.1:43311/login # type <input:email> "Email" on https://127.0.0.1:43311/login type css=#email bob@example.com # type <input:password> "Password" on https://127.0.0.1:43311/login type css=#password $WEBCLI_PASSWORD # check <input:checkbox> "Remember me" on https://127.0.0.1:43311/login check css=#remember # click <button> "Sign in" on https://127.0.0.1:43311/login click "css=#loginform > button:nth-of-type(1)"  |
| 57 | Automation | run <recording> replays it ($WEBCLI_PASSWORD from env) | ✅ PASS | run /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/login.webcli: 5 step(s) OK in 39ms; server got "email=bob%40example.com&password=replay-pw-42&country=us&remember=on" |
| 58 | Phase 6 | open page with iframes | ✅ PASS | opened https://127.0.0.1:43311/frames "Frames" tab=2027235643 action_id=cd789515-d14b-452e-abca-9352376dec00 |
| 59 | Phase 6 | list includes same-origin iframe, cross-origin iframe and shadow DOM controls | ❌ FAIL | same="" xo="" shadow="2  [button]  Shadow Save" |
| 60 | Phase 6 | type + click inside a same-origin iframe | ❌ FAIL | error: fzf: no element matches "Frame Name" \| error: fzf: no element matches "Frame Submit" \| server got [] |
| 61 | Phase 6 | cross-origin iframe: type, then 'press Enter' submits in the focused frame | ❌ FAIL | error: fzf: no element matches "XO Name" \| pressed Enter on <body> "Frames and shadow DOM" action_id=f29ae8a0-684e-4687-b17b-59bc67df2e6c \| server got [] |
| 62 | Phase 6 | css= selector found in a cross-origin iframe | ❌ FAIL | error: extension: not found: css=#xo-btn \| requests=0 |
| 63 | Phase 6 | type + click inside an open shadow root (label resolved in the shadow tree) | ✅ PASS | typed 12 char(s) into <input:text> "Shadow input" selector=#shadow-in action_id=bd77c709-d536-4f4b-a7a4-dbff005ecc8c \| click <button> "Shadow Save" selector=#shadow-btn action_id=c859d60d-910d-4617-b5cd-7eb33ddc6717 \| page shows "saved: shadow-value" |
| 64 | Phase 6 | css= selector inside a shadow root | ✅ PASS | typed 7 char(s) into <input:text> "Shadow input" selector=#shadow-in action_id=62480d1a-1f85-459b-a6c6-0cb24d86fe2b \| value="via-css" |
| 65 | Phase 6 | upload 1.5 MB + text file via the label of a hidden <input type=file multiple> (chunked) | ✅ PASS | uploaded 2 file(s) (1500013 bytes: photo.png, notes.txt) into <input:file> "Choose files" selector=#file action_id=e9d6d1f2-c156-4f92-a005-441028253153 \| page change event saw "photo.png:1500000:image/png,notes.txt:13:text/plain" |
| 66 | Phase 6 | server received both files byte-identical (multipart POST, sha256) | ✅ PASS | title="Uploaded" server got [{Name:photo.png Type:image/png Size:1500000 SHA256:e5e7b285772da8c2ef251f62d31b539a3cd759e9a1f1fbc78db8acab10d8ea32} {Name:notes.txt Type:text/plain Size:13 SHA256:993a327368cc9a443f6d9a11d146da9e9ba2d561a8ef1e9190d119b2b1a002e0}] |
| 67 | Phase 6 | upload onto a drop zone (drag&drop events with the file) | ✅ PASS | dropped 1 file(s) (13 bytes: notes.txt) into <div> "Drop files here" selector=#drop action_id=be265fa2-df67-4bee-88b0-34d711468a1c \| page saw "notes.txt:hello upload" |
| 68 | Phase 6 | WebSocket works through the MITM proxy and is listed by 'ws' | ✅ PASS | page="echo:hello-ws \| echo:{\"n\":2,\"text\":\"second message\"}" \| #26   closed    6 frame(s)  https://127.0.0.1:43311/ws |
| 69 | Phase 6 | ws <id>: frames in both directions, permessage-deflate inflated, close code | ✅ PASS | sent="02:48:03.336 → text        8B [deflate]  \"hello-ws\"" recv="02:48:03.337 ← text       13B [deflate]  \"echo:hello-ws\"" second="02:48:03.338 ← text       36B [deflate]  \"echo:{\\\"n\\\":2,\\\"text\\\":\\\"second message\\\"}\"" close="02:48:03.338 → close       5B [code=1000]  \"bye\"" |
| 70 | Phase 6 | rule throttle latency=700ms + 'timing' shows ttfb >= 700ms (DevTools-style throttling) | ✅ PASS | added rule #4 throttle url="/slow" latency=700ms kbps=2000 \| ttfb=705ms \| navigation navigate over http/1.1: redirect 0ms  dns 3ms  connect 0ms  tls 4ms  ttfb 705ms  download 208ms |
| 71 | Phase 6 | throttled request logged with the rule as intercept | ✅ PASS | #27 GET https://127.0.0.1:43311/slow -> 200 (32041 bytes, 830ms) action=f3cd16cb-b191-4d0a-a951-d4bce1788503 intercepts=1 |
| 72 | Phase 6 | timing <file.json> saves the raw timing data | ✅ PASS | saved timing JSON (515 bytes) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real-headless/audit/timing.json |
