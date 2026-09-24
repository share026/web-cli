package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Rule actions.
const (
	ActBlock         = "block"           // answer locally with Status (default 403); never reaches upstream
	ActSetReqHeader  = "set-req-header"  // set request header Name=Value before forwarding
	ActDelReqHeader  = "del-req-header"  // remove request header Name before forwarding
	ActSetRespHeader = "set-resp-header" // set response header Name=Value before returning to client
	ActDelRespHeader = "del-resp-header" // remove response header Name
	ActReplaceBody   = "replace-body"    // replace the response body with Value
	ActThrottle      = "throttle"        // add Latency before forwarding and cap bodies at Kbps
)

// Rule is an interception rule. All non-empty matchers must match.
type Rule struct {
	ID     int    `json:"id"`
	Action string `json:"action"`
	Host   string `json:"host,omitempty"`   // regexp matched against the request host (without port)
	URL    string `json:"url,omitempty"`    // regexp matched against the full URL
	Method string `json:"method,omitempty"` // exact, case-insensitive
	Name   string `json:"name,omitempty"`   // header name for header actions
	Value  string `json:"value,omitempty"`  // header value / replacement body
	Status int    `json:"status,omitempty"` // for block

	// throttle: extra round-trip latency and bandwidth cap (kilobits/s) — the
	// proxy-side equivalent of DevTools network throttling.
	LatencyMS int `json:"latency_ms,omitempty"`
	Kbps      int `json:"kbps,omitempty"`

	hostRe, urlRe *regexp.Regexp
}

func (r *Rule) compile() error {
	switch r.Action {
	case ActBlock, ActReplaceBody:
	case ActThrottle:
		if r.LatencyMS <= 0 && r.Kbps <= 0 {
			return fmt.Errorf("rule throttle requires latency= and/or kbps=")
		}
	case ActSetReqHeader, ActSetRespHeader, ActDelReqHeader, ActDelRespHeader:
		if r.Name == "" {
			return fmt.Errorf("rule %s requires name=", r.Action)
		}
	default:
		return fmt.Errorf("unknown rule action %q", r.Action)
	}
	var err error
	if r.Host != "" {
		if r.hostRe, err = regexp.Compile(r.Host); err != nil {
			return fmt.Errorf("host regexp: %w", err)
		}
	}
	if r.URL != "" {
		if r.urlRe, err = regexp.Compile(r.URL); err != nil {
			return fmt.Errorf("url regexp: %w", err)
		}
	}
	if r.Action == ActBlock && r.Status == 0 {
		r.Status = http.StatusForbidden
	}
	return nil
}

// Matches reports whether the rule applies to req.
func (r *Rule) Matches(req *http.Request) bool {
	if r.Method != "" && !strings.EqualFold(r.Method, req.Method) {
		return false
	}
	if r.hostRe != nil && !r.hostRe.MatchString(req.URL.Hostname()) {
		return false
	}
	if r.urlRe != nil && !r.urlRe.MatchString(req.URL.String()) {
		return false
	}
	return true
}

// String renders the rule in the same syntax ParseRule accepts.
func (r *Rule) String() string {
	parts := []string{r.Action}
	add := func(k, v string) {
		if v != "" {
			parts = append(parts, k+"="+strconv.Quote(v))
		}
	}
	add("host", r.Host)
	add("url", r.URL)
	add("method", r.Method)
	add("name", r.Name)
	add("value", r.Value)
	if r.Status != 0 {
		parts = append(parts, "status="+strconv.Itoa(r.Status))
	}
	if r.LatencyMS != 0 {
		parts = append(parts, fmt.Sprintf("latency=%dms", r.LatencyMS))
	}
	if r.Kbps != 0 {
		parts = append(parts, "kbps="+strconv.Itoa(r.Kbps))
	}
	return fmt.Sprintf("#%d %s", r.ID, strings.Join(parts, " "))
}

// ParseRule parses `<action> key=value ...` (values may be double-quoted), e.g.
//
//	block host=^ads\.example\.com$ status=451
//	set-req-header url=/api/ name=X-Debug value="1"
//	throttle host=example\.com latency=400ms kbps=1600
func ParseRule(line string) (Rule, error) {
	toks, err := splitArgs(line)
	if err != nil {
		return Rule{}, err
	}
	if len(toks) == 0 {
		return Rule{}, fmt.Errorf("empty rule")
	}
	r := Rule{Action: toks[0]}
	for _, t := range toks[1:] {
		k, v, ok := strings.Cut(t, "=")
		if !ok {
			return Rule{}, fmt.Errorf("expected key=value, got %q", t)
		}
		switch k {
		case "host":
			r.Host = v
		case "url":
			r.URL = v
		case "method":
			r.Method = v
		case "name":
			r.Name = v
		case "value":
			r.Value = v
		case "status":
			if r.Status, err = strconv.Atoi(v); err != nil {
				return Rule{}, fmt.Errorf("status: %w", err)
			}
		case "latency":
			d, err := parseMillis(v)
			if err != nil {
				return Rule{}, fmt.Errorf("latency: %w", err)
			}
			r.LatencyMS = d
		case "kbps":
			if r.Kbps, err = strconv.Atoi(v); err != nil || r.Kbps < 0 {
				return Rule{}, fmt.Errorf("kbps: expected a positive integer, got %q", v)
			}
		default:
			return Rule{}, fmt.Errorf("unknown rule key %q", k)
		}
	}
	return r, r.compile()
}

// splitArgs splits on whitespace honouring double quotes and backslash escapes
// inside quotes.
func splitArgs(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inQ, have := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQ && c == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
		case c == '"':
			inQ, have = !inQ, true
		case !inQ && (c == ' ' || c == '\t'):
			if have {
				out = append(out, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteByte(c)
			have = true
		}
	}
	if inQ {
		return nil, fmt.Errorf("unterminated quote")
	}
	if have {
		out = append(out, cur.String())
	}
	return out, nil
}

// RuleSet is a concurrency-safe ordered list of rules.
type RuleSet struct {
	mu     sync.RWMutex
	rules  []*Rule
	nextID int
}

// Add compiles and appends a rule, returning its assigned ID.
func (s *RuleSet) Add(r Rule) (Rule, error) {
	if err := r.compile(); err != nil {
		return Rule{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	r.ID = s.nextID
	s.rules = append(s.rules, &r)
	return r, nil
}

// Delete removes a rule by ID.
func (s *RuleSet) Delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.rules {
		if r.ID == id {
			s.rules = append(s.rules[:i], s.rules[i+1:]...)
			return true
		}
	}
	return false
}

// List returns a snapshot of the rules.
func (s *RuleSet) List() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Rule, 0, len(s.rules))
	for _, r := range s.rules {
		out = append(out, *r)
	}
	return out
}

// Matching returns the rules that apply to req, in order.
func (s *RuleSet) Matching(req *http.Request) []*Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Rule
	for _, r := range s.rules {
		if r.Matches(req) {
			out = append(out, r)
		}
	}
	return out
}

// LoadFile appends rules from a JSON array file (see docs/ARCHITECTURE.md).
func (s *RuleSet) LoadFile(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var rules []Rule
	if err := json.Unmarshal(raw, &rules); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	for i, r := range rules {
		if _, err := s.Add(r); err != nil {
			return i, fmt.Errorf("rule %d: %w", i, err)
		}
	}
	return len(rules), nil
}
