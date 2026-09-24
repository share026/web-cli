package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/share026/web-cli/internal/audit"
)

// upstream echoes what it received so tests can prove what left the proxy.
func upstream(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ws" {
			wsEcho(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream", "yes")
		json.NewEncoder(w).Encode(map[string]any{
			"method":     r.Method,
			"path":       r.URL.Path,
			"headers":    r.Header,
			"body":       string(body),
			"tls":        r.TLS != nil,
			"action_hdr": r.Header.Get(audit.ActionHeader),
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

type fixture struct {
	proxy  *Proxy
	addr   string
	store  *audit.Store
	rules  *RuleSet
	caPath string
	client *http.Client
	up     *httptest.Server
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	ca, created, err := LoadOrCreateCA(dir)
	if err != nil || !created {
		t.Fatalf("CA: created=%v err=%v", created, err)
	}
	up := upstream(t)
	upRoots := x509.NewCertPool()
	upRoots.AddCert(up.Certificate())

	store := audit.NewStore(nil)
	rules := &RuleSet{}
	p, err := New(Options{CA: ca, Store: store, Rules: rules, UpstreamRoots: upRoots, Logf: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	addr, err := p.Start("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })

	// The client trusts ONLY our generated CA: a successful TLS handshake proves
	// the proxy issued a valid per-host certificate on the fly.
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(p.CAPEM())
	pu, _ := url.Parse("http://" + addr)
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(pu), TLSClientConfig: &tls.Config{RootCAs: roots}},
	}
	return &fixture{proxy: p, addr: addr, store: store, rules: rules, caPath: filepath.Join(dir, CACertFile), client: client, up: up}
}

func (f *fixture) waitExchanges(t *testing.T, n int) []audit.Exchange {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		exs := f.store.Exchanges(func(e *audit.Exchange) bool { return e.Duration > 0 || e.Error != "" })
		if len(exs) >= n || time.Now().After(deadline) {
			return exs
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHTTPSCaptureAndActionHeaderStripped(t *testing.T) {
	f := newFixture(t)
	req, _ := http.NewRequest("POST", f.up.URL+"/api/login?x=1", strings.NewReader(`{"user":"alice"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(audit.ActionHeader, "11111111-2222-3333-4444-555555555555")
	resp, err := f.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var echo map[string]any
	json.NewDecoder(resp.Body).Decode(&echo)
	resp.Body.Close()

	if echo["action_hdr"] != "" {
		t.Fatalf("X-Audit-Action-Id leaked upstream: %v", echo["action_hdr"])
	}
	if echo["tls"] != true || echo["body"] != `{"user":"alice"}` {
		t.Fatalf("upstream saw %v", echo)
	}
	if resp.TLS == nil || resp.TLS.PeerCertificates[0].Issuer.CommonName == "" ||
		!strings.Contains(resp.TLS.PeerCertificates[0].Issuer.CommonName, "web-cli") {
		t.Fatalf("client cert not issued by web-cli CA: %+v", resp.TLS)
	}

	exs := f.waitExchanges(t, 1)
	if len(exs) != 1 {
		t.Fatalf("captured %d exchanges", len(exs))
	}
	ex := exs[0]
	if ex.ActionID != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("action id not linked: %q", ex.ActionID)
	}
	if ex.Method != "POST" || !strings.HasPrefix(ex.URL, "https://127.0.0.1:") || !strings.HasSuffix(ex.URL, "/api/login?x=1") {
		t.Fatalf("url/method: %s %s", ex.Method, ex.URL)
	}
	if string(ex.ReqBody) != `{"user":"alice"}` || ex.ReqHeader.Get(audit.ActionHeader) != "" {
		t.Fatalf("request capture: body=%q hdr=%v", ex.ReqBody, ex.ReqHeader)
	}
	if ex.Status != 200 || ex.RespHeader.Get("X-Upstream") != "yes" || !strings.Contains(string(ex.RespBody), `"path":"/api/login"`) {
		t.Fatalf("response capture: %d %v %s", ex.Status, ex.RespHeader, ex.RespBody)
	}
	t.Logf("captured: %s %s action=%s status=%d resp=%s", ex.Method, ex.URL, ex.ActionID, ex.Status, ex.RespBody)
}

func TestInterceptRules(t *testing.T) {
	f := newFixture(t)
	mustAdd := func(spec string) {
		r, err := ParseRule(spec)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.rules.Add(r); err != nil {
			t.Fatal(err)
		}
	}
	mustAdd(`set-req-header url=/modify name=X-Injected value="by proxy"`)
	mustAdd(`del-req-header url=/modify name=X-Secret`)
	mustAdd(`set-resp-header url=/modify name=X-Audited value=yes`)
	mustAdd(`block url=/blocked status=451`)
	mustAdd(`replace-body url=/replace value="{\"replaced\":true}"`)

	// header modification
	req, _ := http.NewRequest("GET", f.up.URL+"/modify", nil)
	req.Header.Set("X-Secret", "s3cr3t")
	resp, err := f.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var echo struct {
		Headers http.Header `json:"headers"`
	}
	json.NewDecoder(resp.Body).Decode(&echo)
	resp.Body.Close()
	if echo.Headers.Get("X-Injected") != "by proxy" || echo.Headers.Get("X-Secret") != "" {
		t.Fatalf("upstream headers: %v", echo.Headers)
	}
	if resp.Header.Get("X-Audited") != "yes" {
		t.Fatalf("response header not injected: %v", resp.Header)
	}

	// block
	resp, err = f.client.Get(f.up.URL + "/blocked")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 451 || !strings.Contains(string(b), "blocked by web-cli rule") {
		t.Fatalf("block: %d %s", resp.StatusCode, b)
	}

	// body replacement
	resp, err = f.client.Get(f.up.URL + "/replace")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != `{"replaced":true}` {
		t.Fatalf("replace-body: %s", b)
	}

	exs := f.waitExchanges(t, 3)
	if len(exs) != 3 || len(exs[0].Intercepts) != 3 || !exs[1].Blocked || len(exs[2].Intercepts) != 1 {
		t.Fatalf("intercept records: %+v", exs)
	}
	for _, ex := range exs {
		t.Logf("#%d %s %s -> %d blocked=%v intercepts=%v", ex.ID, ex.Method, ex.URL, ex.Status, ex.Blocked, ex.Intercepts)
	}
}

func TestCAIsReusedAndServed(t *testing.T) {
	dir := t.TempDir()
	ca1, created1, err := LoadOrCreateCA(dir)
	if err != nil || !created1 {
		t.Fatal(err)
	}
	ca2, created2, err := LoadOrCreateCA(dir)
	if err != nil || created2 {
		t.Fatalf("second load created=%v err=%v", created2, err)
	}
	if SPKIHash(ca1) != SPKIHash(ca2) || !ca1.Leaf.IsCA {
		t.Fatal("CA not persisted")
	}
	if st, _ := os.Stat(filepath.Join(dir, CAKeyFile)); st.Mode().Perm() != 0o600 {
		t.Fatalf("key perms %v", st.Mode().Perm())
	}
	p, _ := New(Options{CA: ca1, Store: audit.NewStore(nil)})
	rec := httptest.NewRecorder()
	p.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ca.pem", nil))
	blk, _ := pem.Decode(rec.Body.Bytes())
	if blk == nil || blk.Type != "CERTIFICATE" {
		t.Fatalf("GET /ca.pem: %s", rec.Body.String())
	}
}

// TestCurlThroughProxy drives the real curl binary through the proxy
// (curl -x ... --cacert ca.pem), as a user would.
func TestCurlThroughProxy(t *testing.T) {
	curl, err := exec.LookPath("curl")
	if err != nil {
		t.Skip("curl not installed")
	}
	f := newFixture(t)
	r, _ := ParseRule(`set-req-header host=127.0.0.1 name=X-Curl-Modified value=1`)
	f.rules.Add(r)
	r, _ = ParseRule(`set-resp-header host=127.0.0.1 name=X-Audited value=web-cli`)
	f.rules.Add(r)

	out, err := exec.Command(curl, "-sS", "-i", "-x", "http://"+f.addr, "--cacert", f.caPath,
		"-H", audit.ActionHeader+": curl-action-1", "-H", "Content-Type: application/json",
		"-d", `{"via":"curl"}`, f.up.URL+"/curl-test").CombinedOutput()
	if err != nil {
		t.Fatalf("curl: %v\n%s", err, out)
	}
	s := string(out)
	t.Logf("curl output:\n%s", s)
	if !strings.Contains(s, "HTTP/1.1 200") || !strings.Contains(s, "X-Audited: web-cli") {
		t.Fatalf("unexpected curl response")
	}
	if !strings.Contains(s, `"X-Curl-Modified":["1"]`) || !strings.Contains(s, `"action_hdr":""`) {
		t.Fatalf("upstream did not see modified headers / saw action header")
	}
	exs := f.waitExchanges(t, 1)
	if len(exs) != 1 || exs[0].ActionID != "curl-action-1" || string(exs[0].ReqBody) != `{"via":"curl"}` {
		t.Fatalf("capture: %+v", exs)
	}
}
