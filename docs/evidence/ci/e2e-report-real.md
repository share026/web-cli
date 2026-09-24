# web-cli E2E report

- mode: real (headed)
- browser: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36
- date: 2026-09-24T02:51:10Z
- result: 72/72 checks passed

| # | Phase | Check | Result | Evidence |
|---|---|---|---|---|
| 1 | Phase 3 | dynamic root CA generated | ✅ PASS | ca.pem created, SPKI mP0wSkn5xWSWm7vZCDxryJHCuutONCpgvV/RPu+h+/E= |
| 2 | Phase 1 | real Chromium loaded extension/ (MV3 service worker running) | ✅ PASS | service worker "chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/background.js" |
| 3 | Phase 1 | nm-host started by extension and connected to app | ✅ PASS | hello with origin chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/ |
| 4 | Phase 1 | extension {type:ping} answered by Go with {type:pong} | ✅ PASS | app log: ping -> pong |
| 5 | Phase 1 | background.js received pong | ✅ PASS | extension reported pong_received back over the bridge |
| 6 | Phase 1 | app -> extension ping round trip | ✅ PASS | pong from extension in 1.766ms: {"from":"extension"} |
| 7 | Phase 2 | hints Login -> fzf -> click | ✅ PASS | fzf selected hint 2 \| click hint=2 <button> "Login" action_id=5e601fd8-f096-4665-9320-3ec1d47ac4ff |
| 8 | Phase 2 | Vimium C visibility filter | ✅ PASS | visible=["Next page" "Login" "Who am I" "Search products" "English" "Role Button" "Onclick span"] missing=[] leaked-hidden=[] |
| 9 | Phase 2 | click fired in page (fetch /api/login ran) | ✅ PASS | #status="logged in: alice" |
| 10 | Phase 4 | X-Audit-Action-Id stripped before upstream | ✅ PASS | origin saw 1 POST /api/login, action header present=false, body={"user":"alice","password":"wonderland"} |
| 11 | Phase 4 | proxy linked POST /api/login to the click action | ✅ PASS | #3 POST https://127.0.0.1:35597/api/login -> 200 (27 bytes, 0s) action=5e601fd8-f096-4665-9320-3ec1d47ac4ff |
| 12 | Phase 3 | rule add set-req-header | ✅ PASS | added rule #1 set-req-header url="/api/whoami" name="X-Injected" value="e2e-rule" |
| 13 | Phase 3 | rule add block | ✅ PASS | added rule #2 block url="/blocked" status=451 |
| 14 | Phase 3 | rule add set-resp-header | ✅ PASS | added rule #3 set-resp-header url="/api/whoami" name="X-Audited" value="web-cli" |
| 15 | Phase 4 | cookies dump via chrome.cookies.getAll | ✅ PASS | dumped 1 cookie(s) for https://127.0.0.1:35597/ to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/cookies.json; file has session=abc123 (HttpOnly,Secure)=true |
| 16 | Phase 4 | cookies import via chrome.cookies.set | ✅ PASS | imported 1/1 cookie(s) from /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/cookies-import.json |
| 17 | Phase 2 | hints 'who am' -> click | ✅ PASS | click hint=3 <button> "Who am I" action_id=a6f9f275-6ae8-4751-a18b-7ad2c2764dc6 |
| 18 | Phase 4 | imported cookie sent by the browser | ✅ PASS | origin received Cookie: "session=abc123; imported=from-json"; page shows "cookies: imported=from-json,session=abc123" |
| 19 | Phase 3 | request header injected by rule | ✅ PASS | origin received X-Injected: "e2e-rule" |
| 20 | Phase 3 | request blocked by rule (never reached origin) | ✅ PASS | page shows "blocked status: 451", origin saw 0 /blocked requests |
| 21 | Phase 3 | blocked + modified requests recorded with intercept notes | ✅ PASS | #4 GET https://127.0.0.1:35597/api/whoami -> 200 (74 bytes, 0s) action=a6f9f275-6ae8-4751-a18b-7ad2c2764dc6 intercepts=2 \| #5 GET https://127.0.0.1:35597/blocked -> 451 BLOCKED action=a6f9f275-6ae8-4751-a18b-7ad2c2764dc6 intercepts=1 |
| 22 | Phase 2 | hints 'next page' -> link click navigates | ✅ PASS | click hint=1 <a> "Next page" action_id=c410aaea-b6ea-4a3e-9d60-36c7af12b94e |
| 23 | Phase 2 | navigation happened | ✅ PASS | document.title="Next" |
| 24 | Phase 4 | main_frame navigation tagged, header stripped upstream | ✅ PASS | origin saw 1 GET /next without action header |
| 25 | Phase 2 | content script re-injected after navigation; focus/click on new page | ✅ PASS | collected 2 visible element(s) on https://127.0.0.1:35597/next \| click hint=2 <button> "Press me" action_id=635506de-53de-4c1f-8894-97482a71d857 |
| 26 | Phase 2 | button on new page clicked | ✅ PASS | button text="clicked" |
| 27 | Phase 2 | stale page: 'search' has no match on new page | ✅ PASS | error: fzf: no element matches "search" |
| 28 | Phase 4 | export all captured requests to .http | ✅ PASS | wrote 6 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/requests.http |
| 29 | Phase 4 | export filtered by action ID | ✅ PASS | wrote 1 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/login-action.http |
| 30 | Phase 4 | navigation action has linked requests | ✅ PASS | wrote 1 request(s) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/nav-action.http |
| 31 | Phase 1 | status reports session and captures | ✅ PASS | sessions: 1 \| captured: 6 request(s), 4 action(s), 3 rule(s) |
| 32 | Phase 4 | .http file replays with curl | ✅ PASS | see harness.log |
| 33 | Automation | open <url> waits for the load | ✅ PASS | opened https://127.0.0.1:35597/login "Login" tab=878221972 action_id=0030607e-e57a-4341-a4ec-fcaf24e3999d |
| 34 | Automation | type <label text> <value> (target by <label>) | ✅ PASS | typed 17 char(s) into <input:email> "Email" selector=#email action_id=0a19a040-d97d-41b0-ad23-5b47c626aaa4 |
| 35 | Automation | type password from $VAR, value never printed | ✅ PASS | typed 13 char(s) into <input:password> "Password" selector=#password action_id=87eceeea-9294-47cc-a8d5-1adf85a419aa; value in app output=false |
| 36 | Automation | select <select> option by text | ✅ PASS | selected "Japan" in <select> "Country: Japan" selector=#country action_id=49a47c09-b7e3-4300-8206-7a5066abbd17 |
| 37 | Automation | check a checkbox (wrapping <label>) | ✅ PASS | checked <input:checkbox> "Remember me" checked=true selector=#remember action_id=ce49a87d-0db2-4373-969b-88c7b59fc090 |
| 38 | Automation | press Enter in a field submits the form | ✅ PASS | pressed Enter on <input:password> "Password" action_id=ae4f858e-d58d-49e6-ad28-c389ce30355c |
| 39 | Automation | waitfor text=... (after navigation) | ✅ PASS | found text=Welcome alice@example.com after 3ms https://127.0.0.1:35597/dashboard?user=alice%40example.com |
| 40 | Automation | server received the filled form (POST /session) | ✅ PASS | body="email=alice%40example.com&password=E2E-s3cret-pw&country=jp&remember=on" action header upstream="" |
| 41 | Automation | form POST + redirect linked to the Enter action | ✅ PASS | #8 POST https://127.0.0.1:35597/session -> 303 (0 bytes, 1ms) action=ae4f858e-d58d-49e6-ad28-c389ce30355c \| #9 GET https://127.0.0.1:35597/dashboard?user=alice%40example.com -> 200 (336 bytes, 0s) action=ae4f858e-d58d-49e6-ad28-c389ce30355c |
| 42 | Automation | show <id>: request headers + form body + response | ✅ PASS | #8 POST https://127.0.0.1:35597/session HTTP/1.1 \| action: ae4f858e-d58d-49e6-ad28-c389ce30355c press <input> "Enter on Password" on https://127.0.0.1:35597/login \| --- response 303 (1ms) \| Location: /dashboard?user=alice%40example.com |
| 43 | Automation | body <id> <file>: HTML as served (before JavaScript) | ✅ PASS | wrote 336 byte(s) of the response body of #9 (text/html; charset=utf-8) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/dashboard-served.html |
| 44 | Automation | text: visible page text | ✅ PASS | Welcome alice@example.com \| rendered by js |
| 45 | Automation | source <file>: rendered DOM (after JavaScript) | ✅ PASS | saved rendered DOM of https://127.0.0.1:35597/dashboard?user=alice%40example.com (345 bytes) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/dashboard-rendered.html |
| 46 | Automation | eval <js> (page main world) | ✅ PASS | => "Dashboard"  (string, via main-world) |
| 47 | Automation | eval returns JSON values | ✅ PASS | => {"n":2,"path":"/dashboard","theme":"dark"}  (object, via main-world) |
| 48 | Automation | storage dump (localStorage + sessionStorage) | ✅ PASS | dumped 1 localStorage + 1 sessionStorage item(s) for https://127.0.0.1:35597 to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/storage.json \| local   theme=dark \| session tab=home |
| 49 | Automation | screenshot <file> (PNG) | ✅ PASS | saved screenshot of https://127.0.0.1:35597/dashboard?user=alice%40example.com (11775 bytes) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/dashboard.png |
| 50 | Automation | back | ✅ PASS | back https://127.0.0.1:35597/login "Login" tab=878221972 action_id=7c30053a-c52e-4d25-8d0a-fe9ea626462b |
| 51 | Automation | forward | ✅ PASS | forward https://127.0.0.1:35597/dashboard?user=alice%40example.com "Dashboard" tab=878221972 action_id=b4811db1-f704-449c-8d39-e47e9fa96c36 |
| 52 | Automation | reload | ✅ PASS | reloaded https://127.0.0.1:35597/dashboard?user=alice%40example.com "Dashboard" tab=878221972 action_id=18683384-6c98-4ad5-89cc-3ccd93483d82 |
| 53 | Automation | newtab / tabs / closetab | ✅ PASS | opened https://127.0.0.1:35597/next "Next" tab=878221973 action_id=ee6d80fd-70a7-4e05-848f-051dac981694 \| 2 tab(s) ; 878221972  https://127.0.0.1:35597/dashboard?user=alice%40example.com  "Dashboard" ; * 878221973  https://127.0.0.1:35597/next  "Next" \| 1 tab(s) |
| 54 | Automation | eval under a strict CSP (debugger fallback) | ✅ PASS | => 42  (number, via debugger) |
| 55 | Automation | record start | ✅ PASS | recording to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/login.webcli (starting at https://127.0.0.1:35597/login): use the browser normally and/or web-cli commands; 'record stop' ends it |
| 56 | Automation | recording of real user input | ✅ PASS | err=<nil> recording stopped: 5 step(s) written to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/login.webcli; script: # web-cli recording 2026-09-24T02:51:03Z # replay: bin/app -f login.webcli   (or 'run login.webcli' in the REPL) # password fields are recorded as $WEBCLI_PASSWORD: export it before replaying open https://127.0.0.1:35597/login # type <input:email> "Email" on https://127.0.0.1:35597/login type css=#email bob@example.com # type <input:password> "Password" on https://127.0.0.1:35597/login type css=#password $WEBCLI_PASSWORD # check <input:checkbox> "Remember me" on https://127.0.0.1:35597/login check css=#remember # click <button> "Sign in" on https://127.0.0.1:35597/login click "css=#loginform > button:nth-of-type(1)"  |
| 57 | Automation | run <recording> replays it ($WEBCLI_PASSWORD from env) | ✅ PASS | run /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/login.webcli: 5 step(s) OK in 60ms; server got "email=bob%40example.com&password=replay-pw-42&country=us&remember=on" |
| 58 | Phase 6 | open page with iframes | ✅ PASS | opened https://127.0.0.1:35597/frames "Frames" tab=878221972 action_id=975bc6ce-ba6a-4fa3-b0b4-f16a7df37e33 |
| 59 | Phase 6 | list includes same-origin iframe, cross-origin iframe and shadow DOM controls | ✅ PASS | same="4  [button]  Frame Submit    (frame https://127.0.0.1:35597/frame-inner)" xo="6  [button]  XO Submit    (frame https://127.0.0.1:39897/xo-inner)" shadow="2  [button]  Shadow Save" |
| 60 | Phase 6 | type + click inside a same-origin iframe | ✅ PASS | typed 3 char(s) into <input:text> "Frame Name" in frame 3 (https://127.0.0.1:35597/frame-inner) selector=#name action_id=c1c9661d-f154-4ff3-9d93-6695999a406c \| click <button> "Frame Submit" in frame 3 (https://127.0.0.1:35597/frame-inner) selector=#same-btn action_id=861c459a-3628-4a2e-b92d-807d7e1ff0ff \| server got ["same:Bob"] |
| 61 | Phase 6 | cross-origin iframe: type, then 'press Enter' submits in the focused frame | ✅ PASS | typed 5 char(s) into <input:text> "XO Name" in frame 4 (https://127.0.0.1:39897/xo-inner) selector=#name action_id=cbf47344-807d-4e86-9a5c-4a3d6bdc6f5b \| pressed Enter on <input:text> "XO Name" in frame 4 (https://127.0.0.1:39897/xo-inner) action_id=65eff755-d23a-41f9-8484-3dfce4bdf871 \| server got ["same:Bob" "xo:Carol"] |
| 62 | Phase 6 | css= selector found in a cross-origin iframe | ✅ PASS | click <button> "XO Submit" in frame 4 (https://127.0.0.1:39897/xo-inner) selector=#xo-btn action_id=17ddd4ea-8116-40ea-acf3-35beac79687e \| requests=3 |
| 63 | Phase 6 | type + click inside an open shadow root (label resolved in the shadow tree) | ✅ PASS | typed 12 char(s) into <input:text> "Shadow input" selector=#shadow-in action_id=c5b9ccfd-0dc8-4baa-a11a-df06b21ac939 \| click <button> "Shadow Save" selector=#shadow-btn action_id=c7e003dd-6ca7-45d1-a6d5-f408a8891fd9 \| page shows "saved: shadow-value" |
| 64 | Phase 6 | css= selector inside a shadow root | ✅ PASS | typed 7 char(s) into <input:text> "Shadow input" selector=#shadow-in action_id=0c8eca0d-8228-4823-ab46-b6f62a5938ea \| value="via-css" |
| 65 | Phase 6 | upload 1.5 MB + text file via the label of a hidden <input type=file multiple> (chunked) | ✅ PASS | uploaded 2 file(s) (1500013 bytes: photo.png, notes.txt) into <input:file> "Choose files" selector=#file action_id=636a66a2-1178-463c-a66e-77f9b536825f \| page change event saw "photo.png:1500000:image/png,notes.txt:13:text/plain" |
| 66 | Phase 6 | server received both files byte-identical (multipart POST, sha256) | ✅ PASS | title="Uploaded" server got [{Name:photo.png Type:image/png Size:1500000 SHA256:0805d0fcefe75b72c6852a71335418dc9aa84a3a51cbb1067340e5d3c06d605f} {Name:notes.txt Type:text/plain Size:13 SHA256:993a327368cc9a443f6d9a11d146da9e9ba2d561a8ef1e9190d119b2b1a002e0}] |
| 67 | Phase 6 | upload onto a drop zone (drag&drop events with the file) | ✅ PASS | dropped 1 file(s) (13 bytes: notes.txt) into <div> "Drop files here" selector=#drop action_id=1208230e-4dd7-4ad1-8c4c-2d58f5026bc1 \| page saw "notes.txt:hello upload" |
| 68 | Phase 6 | WebSocket works through the MITM proxy and is listed by 'ws' | ✅ PASS | page="echo:hello-ws \| echo:{\"n\":2,\"text\":\"second message\"}" \| #29   closed    6 frame(s)  https://127.0.0.1:35597/ws |
| 69 | Phase 6 | ws <id>: frames in both directions, permessage-deflate inflated, close code | ✅ PASS | sent="02:51:07.830 → text        8B [deflate]  \"hello-ws\"" recv="02:51:07.831 ← text       13B [deflate]  \"echo:hello-ws\"" second="02:51:07.833 ← text       36B [deflate]  \"echo:{\\\"n\\\":2,\\\"text\\\":\\\"second message\\\"}\"" close="02:51:07.833 → close       5B [code=1000]  \"bye\"" |
| 70 | Phase 6 | rule throttle latency=700ms + 'timing' shows ttfb >= 700ms (DevTools-style throttling) | ✅ PASS | added rule #4 throttle url="/slow" latency=700ms kbps=2000 \| ttfb=705ms \| navigation navigate over http/1.1: redirect 0ms  dns 3ms  connect 0ms  tls 4ms  ttfb 705ms  download 129ms |
| 71 | Phase 6 | throttled request logged with the rule as intercept | ✅ PASS | #30 GET https://127.0.0.1:35597/slow -> 200 (32041 bytes, 830ms) action=27733d79-2a2c-412f-a9a0-ac719be29f69 intercepts=1 |
| 72 | Phase 6 | timing <file.json> saves the raw timing data | ✅ PASS | saved timing JSON (513 bytes) to /home/runner/work/web-cli/web-cli/scripts/e2e/out-real/audit/timing.json |
