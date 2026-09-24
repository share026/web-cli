// Package proxy is the local MITM audit proxy. Protocol handling (HTTP
// proxying, CONNECT, TLS interception, per-host certificate signing) is
// delegated to github.com/elazarl/goproxy; this package only adds capture,
// action-ID correlation and rule-based interception on top of it.
package proxy

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/elazarl/goproxy"

	"github.com/share026/web-cli/internal/audit"
)

// Options configures a Proxy.
type Options struct {
	CA    tls.Certificate
	Store *audit.Store
	Rules *RuleSet
	// MaxBody caps how many bytes of each body are kept in memory (default 4 MiB).
	// Traffic itself is never truncated.
	MaxBody int64
	// UpstreamRoots are the CAs trusted for upstream servers (nil = system pool).
	UpstreamRoots *x509.CertPool
	// InsecureUpstream disables upstream certificate verification (testing only).
	InsecureUpstream bool
	Logf             func(format string, args ...any)
}

// Proxy is a running MITM proxy.
type Proxy struct {
	gp    *goproxy.ProxyHttpServer
	opt   Options
	srv   *http.Server
	ln    net.Listener
	caPEM []byte
}

// New builds the goproxy server with capture and interception handlers.
func New(opt Options) (*Proxy, error) {
	if opt.Store == nil {
		return nil, errors.New("proxy: Store is required")
	}
	if opt.Rules == nil {
		opt.Rules = &RuleSet{}
	}
	if opt.MaxBody <= 0 {
		opt.MaxBody = 4 << 20
	}
	if opt.Logf == nil {
		opt.Logf = func(string, ...any) {}
	}
	if len(opt.CA.Certificate) == 0 {
		return nil, errors.New("proxy: CA is required")
	}
	if opt.CA.Leaf == nil {
		leaf, err := x509.ParseCertificate(opt.CA.Certificate[0])
		if err != nil {
			return nil, err
		}
		opt.CA.Leaf = leaf
	}

	p := &Proxy{opt: opt, caPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: opt.CA.Certificate[0]})}
	gp := goproxy.NewProxyHttpServer()
	gp.Verbose = false
	gp.CertStore = &certCache{m: map[string]*tls.Certificate{}}
	gp.Tr = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSClientConfig:       &tls.Config{RootCAs: opt.UpstreamRoots, InsecureSkipVerify: opt.InsecureUpstream}, //nolint:gosec // opt-in for tests
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	// Plain (non-proxy) requests to the listener: serve the CA for easy installation.
	gp.NonproxyHandler = http.HandlerFunc(p.serveLocal)

	tlsConf := goproxy.TLSConfigFromCA(&p.opt.CA)
	gp.OnRequest().HandleConnect(goproxy.FuncHttpsHandler(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		return &goproxy.ConnectAction{Action: goproxy.ConnectMitm, TLSConfig: tlsConf}, host
	}))
	gp.OnRequest().DoFunc(p.onRequest)
	gp.OnResponse().DoFunc(p.onResponse)
	p.gp = gp
	return p, nil
}

// Handler exposes the proxy as an http.Handler (useful for tests).
func (p *Proxy) Handler() http.Handler { return p.gp }

// CAPEM returns the CA certificate in PEM form.
func (p *Proxy) CAPEM() []byte { return p.caPEM }

// Start listens on addr ("127.0.0.1:8080"; port 0 picks a free port).
func (p *Proxy) Start(addr string) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	p.ln = ln
	p.srv = &http.Server{Handler: p.gp, ReadHeaderTimeout: 30 * time.Second}
	go func() {
		if err := p.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			p.opt.Logf("proxy: serve: %v", err)
		}
	}()
	return ln.Addr().String(), nil
}

// Close stops the listener.
func (p *Proxy) Close() error {
	if p.srv == nil {
		return nil
	}
	return p.srv.Close()
}

func (p *Proxy) serveLocal(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/ca.pem", "/ca.crt":
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Write(p.caPEM)
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "web-cli audit proxy\n\nConfigure this address as your HTTP/HTTPS proxy.\nCA certificate: /ca.pem\nSPKI (for --ignore-certificate-errors-spki-list): %s\n", SPKIHash(p.opt.CA))
	}
}

func (p *Proxy) onRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	ex := &audit.Exchange{Start: time.Now(), Method: req.Method, URL: req.URL.String(), Proto: req.Proto}

	// Context linkage: which DOM action caused this request? The header is an
	// internal signal between extension and proxy and must never leave the machine.
	if id := req.Header.Get(audit.ActionHeader); id != "" {
		ex.ActionID = id
		req.Header.Del(audit.ActionHeader)
	}

	// Capture the request body without truncating what is forwarded.
	if req.Body != nil && req.Body != http.NoBody {
		buf, err := io.ReadAll(io.LimitReader(req.Body, p.opt.MaxBody+1))
		if err != nil {
			ex.Error = "read request body: " + err.Error()
		}
		if int64(len(buf)) > p.opt.MaxBody {
			ex.Truncated = true
			req.Body = struct {
				io.Reader
				io.Closer
			}{io.MultiReader(bytes.NewReader(buf), req.Body), req.Body}
			buf = buf[:p.opt.MaxBody]
		} else {
			req.Body.Close()
			req.Body = io.NopCloser(bytes.NewReader(buf))
		}
		ex.ReqBody = append([]byte(nil), buf...)
	}

	var resp *http.Response
	for _, r := range p.opt.Rules.Matching(req) {
		switch r.Action {
		case ActBlock:
			resp = goproxy.NewResponse(req, "text/plain; charset=utf-8", r.Status,
				fmt.Sprintf("blocked by web-cli rule #%d\n", r.ID))
			ex.Blocked = true
			ex.Intercepts = append(ex.Intercepts, r.String())
		case ActSetReqHeader:
			req.Header.Set(r.Name, r.Value)
			ex.Intercepts = append(ex.Intercepts, r.String())
		case ActDelReqHeader:
			req.Header.Del(r.Name)
			ex.Intercepts = append(ex.Intercepts, r.String())
		}
		if resp != nil {
			break
		}
	}
	if resp == nil {
		if th := throttleFor(p.opt.Rules.Matching(req)); th.latency > 0 || th.kbps > 0 {
			ex.Intercepts = append(ex.Intercepts, th.note)
			// Latency is applied once per request before it goes upstream; the
			// bandwidth cap applies to the request and response bodies.
			time.Sleep(th.latency)
			if th.kbps > 0 && req.Body != nil && req.Body != http.NoBody {
				req.Body = &throttledBody{rc: req.Body, bps: th.kbps * 1000 / 8}
			}
		}
	}
	// Record the request exactly as it is forwarded (after rules, without the audit header).
	ex.ReqHeader = req.Header.Clone()
	p.opt.Store.Begin(ex)
	ctx.UserData = ex
	return req, resp
}

func (p *Proxy) onResponse(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	ex, _ := ctx.UserData.(*audit.Exchange)
	if ex == nil {
		return resp
	}
	if resp == nil {
		msg := "no response"
		if ctx.Error != nil {
			msg = ctx.Error.Error()
		}
		p.opt.Store.Finish(ex, func(e *audit.Exchange) {
			e.Error = msg
			e.Duration = time.Since(e.Start)
		})
		return nil
	}

	var notes []string
	if !ex.Blocked {
		for _, r := range p.opt.Rules.Matching(ctx.Req) {
			switch r.Action {
			case ActSetRespHeader:
				resp.Header.Set(r.Name, r.Value)
			case ActDelRespHeader:
				resp.Header.Del(r.Name)
			case ActReplaceBody:
				if resp.Body != nil {
					resp.Body.Close()
				}
				resp.Body = io.NopCloser(bytes.NewReader([]byte(r.Value)))
				resp.ContentLength = int64(len(r.Value))
				resp.Header.Set("Content-Length", strconv.Itoa(len(r.Value)))
				resp.Header.Del("Content-Encoding")
			default:
				continue
			}
			notes = append(notes, r.String())
		}
	}
	status, header := resp.StatusCode, resp.Header.Clone()
	p.opt.Store.Update(ex, func(e *audit.Exchange) {
		e.Status = status
		e.RespHeader = header
		e.Intercepts = append(e.Intercepts, notes...)
	})

	// WebSocket upgrade: goproxy needs resp.Body to stay an io.ReadWriter, so
	// tap it instead of wrapping it in captureBody. The exchange is finished
	// when the connection ends.
	if resp.StatusCode == http.StatusSwitchingProtocols {
		if rw, ok := resp.Body.(io.ReadWriteCloser); ok {
			resp.Body = newWSTap(rw, resp.Header,
				func(f audit.WSFrame) { p.opt.Store.AddWSFrame(ex, f) },
				func(err error) {
					p.opt.Store.Finish(ex, func(e *audit.Exchange) {
						e.Duration = time.Since(e.Start)
						if err != nil && e.Error == "" {
							e.Error = "websocket: " + err.Error()
						}
					})
				})
			return resp
		}
	}

	if resp.Body == nil || resp.Body == http.NoBody {
		p.opt.Store.Finish(ex, func(e *audit.Exchange) { e.Duration = time.Since(e.Start) })
		return resp
	}
	if th := throttleFor(p.opt.Rules.Matching(ctx.Req)); th.kbps > 0 {
		resp.Body = &throttledBody{rc: resp.Body, bps: th.kbps * 1000 / 8}
	}
	// Tee the body as it streams to the client; finish the record at EOF/Close.
	resp.Body = &captureBody{rc: resp.Body, max: p.opt.MaxBody, done: func(b []byte, truncated bool, err error) {
		p.opt.Store.Finish(ex, func(e *audit.Exchange) {
			e.RespBody = b
			e.Truncated = e.Truncated || truncated
			e.Duration = time.Since(e.Start)
			if err != nil && e.Error == "" {
				e.Error = "response body: " + err.Error()
			}
		})
	}}
	return resp
}

type captureBody struct {
	rc        io.ReadCloser
	buf       bytes.Buffer
	max       int64
	truncated bool
	once      sync.Once
	done      func([]byte, bool, error)
}

func (c *captureBody) Read(b []byte) (int, error) {
	n, err := c.rc.Read(b)
	if n > 0 {
		room := c.max - int64(c.buf.Len())
		switch {
		case room >= int64(n):
			c.buf.Write(b[:n])
		case room > 0:
			c.buf.Write(b[:room])
			c.truncated = true
		default:
			c.truncated = true
		}
	}
	if err != nil {
		var ferr error
		if !errors.Is(err, io.EOF) {
			ferr = err
		}
		c.finish(ferr)
	}
	return n, err
}

func (c *captureBody) Close() error {
	err := c.rc.Close()
	c.finish(nil)
	return err
}

func (c *captureBody) finish(err error) {
	c.once.Do(func() { c.done(append([]byte(nil), c.buf.Bytes()...), c.truncated, err) })
}
