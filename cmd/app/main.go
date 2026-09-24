// Command app is the resident web-cli process. It runs in a real terminal and
//
//   - listens on the Unix socket that nm-host bridges to the Chrome extension,
//   - runs the local MITM audit proxy (goproxy),
//   - lets the user pick visible page elements with the system fzf and clicks them,
//   - exports captured traffic as .http files (kulala.nvim / REST Client),
//   - dumps and imports cookies through the extension.
package main

import (
	"bufio"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/share026/web-cli/internal/audit"
	"github.com/share026/web-cli/internal/ipc"
	"github.com/share026/web-cli/internal/proxy"
)

type config struct {
	socket           string
	proxyAddr        string
	caDir            string
	outDir           string
	rulesFile        string
	fzf              string
	upstreamCA       string
	insecureUpstream bool
	commands         string
	quiet            bool
}

func defaultCADir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "web-cli")
	}
	return ".web-cli"
}

func main() {
	var cfg config
	defSock := ipc.DefaultSocketPath
	if p := os.Getenv("WEBCLI_SOCKET"); p != "" {
		defSock = p
	}
	flag.StringVar(&cfg.socket, "socket", defSock, "Unix socket shared with nm-host (env WEBCLI_SOCKET)")
	flag.StringVar(&cfg.proxyAddr, "proxy", "127.0.0.1:8080", "MITM proxy listen address (empty disables the proxy)")
	flag.StringVar(&cfg.caDir, "ca-dir", defaultCADir(), "directory holding the generated root CA (ca.pem / ca-key.pem)")
	flag.StringVar(&cfg.outDir, "out", "audit-out", "directory for audit.jsonl, .http exports and cookie dumps")
	flag.StringVar(&cfg.rulesFile, "rules", "", "JSON file with interception rules to load at startup")
	flag.StringVar(&cfg.fzf, "fzf", "fzf", "fzf binary")
	flag.StringVar(&cfg.upstreamCA, "upstream-ca", "", "extra PEM CA bundle trusted for upstream TLS (in addition to system roots)")
	flag.BoolVar(&cfg.insecureUpstream, "insecure-upstream", false, "do not verify upstream TLS certificates (testing only)")
	flag.StringVar(&cfg.commands, "c", "", "run ';'-separated commands non-interactively, then exit")
	flag.BoolVar(&cfg.quiet, "q", false, "do not print a line per proxied request")
	flag.Parse()

	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	logger := log.New(os.Stderr, "", log.Ltime|log.Lmicroseconds)
	if err := os.MkdirAll(cfg.outDir, 0o700); err != nil {
		return err
	}
	jsonl, err := audit.OpenJSONL(filepath.Join(cfg.outDir, "audit.jsonl"))
	if err != nil {
		return err
	}
	defer jsonl.Close()
	store := audit.NewStore(jsonl)
	rules := &proxy.RuleSet{}
	if cfg.rulesFile != "" {
		n, err := rules.LoadFile(cfg.rulesFile)
		if err != nil {
			return err
		}
		logger.Printf("[proxy] loaded %d rule(s) from %s", n, cfg.rulesFile)
	}

	hub, err := ipc.Listen(cfg.socket)
	if err != nil {
		return err
	}
	defer hub.Close()
	hub.Logf = func(f string, a ...any) { logger.Printf("["+"ipc] "+strings.TrimPrefix(f, "ipc: "), a...) }
	hub.OnEvent = func(s *ipc.Session, m ipc.Message) {
		if m.Type == ipc.TypeLog {
			logger.Printf("[ext#%d] %s", s.ID, string(m.Payload))
			return
		}
		logger.Printf("[ipc] unsolicited %s from session #%d: %s", m.Type, s.ID, string(m.Payload))
	}
	go func() {
		if err := hub.Serve(); err != nil {
			logger.Printf("[ipc] serve: %v", err)
		}
	}()
	logger.Printf("[ipc] listening on %s", cfg.socket)

	a := &App{cfg: cfg, hub: hub, store: store, rules: rules, log: logger}

	if cfg.proxyAddr != "" {
		ca, created, err := proxy.LoadOrCreateCA(cfg.caDir)
		if err != nil {
			return err
		}
		if created {
			logger.Printf("[proxy] generated new root CA in %s", cfg.caDir)
		}
		var roots *x509.CertPool
		if cfg.upstreamCA != "" {
			roots, err = x509.SystemCertPool()
			if err != nil || roots == nil {
				roots = x509.NewCertPool()
			}
			pemData, err := os.ReadFile(cfg.upstreamCA)
			if err != nil {
				return err
			}
			if !roots.AppendCertsFromPEM(pemData) {
				return fmt.Errorf("no certificates found in %s", cfg.upstreamCA)
			}
		}
		p, err := proxy.New(proxy.Options{
			CA: ca, Store: store, Rules: rules,
			UpstreamRoots: roots, InsecureUpstream: cfg.insecureUpstream,
			Logf: logger.Printf,
		})
		if err != nil {
			return err
		}
		addr, err := p.Start(cfg.proxyAddr)
		if err != nil {
			return err
		}
		defer p.Close()
		a.proxyAddr, a.ca = addr, ca
		logger.Printf("[proxy] listening on %s (CA %s, SPKI %s)", addr, filepath.Join(cfg.caDir, proxy.CACertFile), proxy.SPKIHash(ca))
		if !cfg.quiet {
			store.OnRecord = func(ex audit.Exchange) { logger.Printf("[proxy] %s", summary(ex)) }
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		logger.Printf("shutting down")
		hub.Close()
		os.Exit(0)
	}()

	if cfg.commands != "" {
		for _, c := range strings.Split(cfg.commands, ";") {
			if strings.TrimSpace(c) == "" {
				continue
			}
			fmt.Printf("> %s\n", strings.TrimSpace(c))
			if quit, err := a.exec(c); err != nil {
				return fmt.Errorf("%s: %w", strings.TrimSpace(c), err)
			} else if quit {
				break
			}
		}
		return nil
	}
	return a.repl(os.Stdin, os.Stdout)
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// repl reads one command per line. Commands run synchronously on this
// goroutine so that nothing competes with fzf for the terminal.
func (a *App) repl(in *os.File, out io.Writer) error {
	interactive := isTerminal(in)
	if interactive {
		fmt.Fprintln(out, "web-cli ready. Type 'help' for commands.")
	}
	sc := bufio.NewScanner(in)
	for {
		if interactive {
			fmt.Fprint(out, "web-cli> ")
		}
		if !sc.Scan() {
			return sc.Err()
		}
		quit, err := a.exec(sc.Text())
		if err != nil {
			fmt.Fprintln(out, "error:", err)
		}
		if quit {
			return nil
		}
	}
}
