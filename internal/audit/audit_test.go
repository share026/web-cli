package audit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteHTTPFile(t *testing.T) {
	st := NewStore(nil)
	st.AddAction(Action{ID: "act-1", Kind: "click", Hint: 3, Tag: "button", Text: "Login", PageURL: "https://app.test/"})
	ex := st.Begin(&Exchange{
		Start: time.Now(), ActionID: "act-1", Method: "POST", URL: "https://app.test/api/login",
		ReqHeader: http.Header{
			"Content-Type":    {"application/json"},
			"Cookie":          {"sid=1"},
			"Connection":      {"keep-alive"},
			"Accept-Encoding": {"gzip"},
			"Content-Length":  {"16"},
		},
		ReqBody: []byte(`{"user":"alice"}`),
	})
	st.Finish(ex, func(e *Exchange) {
		e.Status = 200
		e.RespHeader = http.Header{"Content-Type": {"application/json"}}
		e.RespBody = []byte(`{"ok":true}`)
	})
	ex2 := st.Begin(&Exchange{Method: "GET", URL: "https://ads.test/x", ReqHeader: http.Header{}})
	st.Finish(ex2, func(e *Exchange) { e.Status = 451; e.Blocked = true })

	var buf bytes.Buffer
	if err := WriteHTTPFile(&buf, st.Exchanges(nil), HTTPFileOptions{Actions: st.Action}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	t.Logf("\n%s", got)
	for _, want := range []string{
		"### #1 POST https://app.test/api/login\n# @name req_1\n",
		`# action: act-1 click hint=3 <button> "Login" page=https://app.test/`,
		"# response: 200 OK, application/json, 11 bytes\n",
		"POST https://app.test/api/login HTTP/1.1\nContent-Type: application/json\nCookie: sid=1\n\n{\"user\":\"alice\"}\n",
		"### #2 GET https://ads.test/x\n",
		"# response: 451 (blocked by proxy rule)\n",
		"GET https://ads.test/x HTTP/1.1\n\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{"Connection:", "Accept-Encoding:", "Content-Length:"} {
		if strings.Contains(got, bad) {
			t.Errorf("hop-by-hop/computed header %q must be omitted", bad)
		}
	}
}

func TestJSONLAndDecompress(t *testing.T) {
	var log bytes.Buffer
	st := NewStore(&log)
	ex := st.Begin(&Exchange{Method: "GET", URL: "https://x.test/", ReqHeader: http.Header{}})
	st.Finish(ex, func(e *Exchange) { e.Status = 200; e.RespBody = []byte("héllo"); e.RespHeader = http.Header{} })
	st.AddAction(Action{ID: "a1", Kind: "click", Hint: 1})
	lines := bytes.Split(bytes.TrimSpace(log.Bytes()), []byte("\n"))
	var rec, act map[string]any
	if err := json.Unmarshal(lines[0], &rec); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(lines[1], &act); err != nil || act["record"] != "action" || act["kind"] != "click" {
		t.Fatalf("action record: %s", lines[1])
	}
	if rec["record"] != "exchange" || rec["response_body"].(map[string]any)["data"] != "héllo" {
		t.Fatalf("jsonl: %s", log.String())
	}
}

func TestCookieRoundTrip(t *testing.T) {
	exp := 1893456000.0
	d := CookieDump{URL: "https://app.test/", Cookies: []Cookie{
		{Name: "sid", Value: "abc", Domain: "app.test", HostOnly: true, Path: "/", Secure: true, HTTPOnly: true, SameSite: "lax", Session: true},
		{Name: "pref", Value: "dark", Domain: ".app.test", Path: "/app", ExpirationDate: &exp},
	}}
	path := filepath.Join(t.TempDir(), "cookies.json")
	if err := SaveCookies(path, d); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("perm %v", st.Mode().Perm())
	}
	got, err := LoadCookies(path)
	if err != nil || len(got.Cookies) != 2 {
		t.Fatal(err)
	}
	s1, _ := got.Cookies[0].ToSetDetails()
	if s1.URL != "https://app.test/" || s1.Domain != "" || s1.ExpirationDate != nil || !s1.HTTPOnly {
		t.Fatalf("host-only session cookie: %+v", s1)
	}
	s2, _ := got.Cookies[1].ToSetDetails()
	if s2.URL != "http://app.test/app" || s2.Domain != ".app.test" || *s2.ExpirationDate != exp {
		t.Fatalf("domain cookie: %+v", s2)
	}
	// bare array format
	os.WriteFile(path, []byte(`[{"name":"a","value":"1","domain":"x.test","path":"/"}]`), 0o600)
	if got, err := LoadCookies(path); err != nil || got.Cookies[0].Name != "a" {
		t.Fatalf("array format: %v %v", got, err)
	}
}
