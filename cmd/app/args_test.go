package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	env := func(k string) (string, bool) {
		v, ok := map[string]string{"PW": "s3cr et", "USER": "alice"}[k]
		return v, ok
	}
	cases := []struct {
		in   string
		vals []string
		raws []string
	}{
		{`type Email alice@example.com`, []string{"type", "Email", "alice@example.com"}, nil},
		{`click "Sign in"`, []string{"click", "Sign in"}, []string{"click", `"Sign in"`}},
		{`type Password $PW`, []string{"type", "Password", "s3cr et"}, []string{"type", "Password", "$PW"}},
		{`type x "${USER}-\$HOME \"q\""`, []string{"type", "x", `alice-$HOME "q"`}, nil},
		{`type x 'lit $PW \n'`, []string{"type", "x", `lit $PW \n`}, nil},
		{`waitfor text="Welcome back"`, []string{"waitfor", "text=Welcome back"}, nil},
		{`click css=a\.b  $`, []string{"click", `css=a\.b`, "$"}, nil},
	}
	for _, c := range cases {
		toks, err := splitArgs(c.in, env)
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		var vals, raws []string
		for _, tk := range toks {
			vals, raws = append(vals, tk.Value), append(raws, tk.Raw)
		}
		if !reflect.DeepEqual(vals, c.vals) {
			t.Errorf("%s: values %q, want %q", c.in, vals, c.vals)
		}
		if c.raws != nil && !reflect.DeepEqual(raws, c.raws) {
			t.Errorf("%s: raws %q, want %q", c.in, raws, c.raws)
		}
	}
	if _, err := splitArgs(`type Password $NOPE`, env); err == nil || !strings.Contains(err.Error(), "NOPE is not set") {
		t.Fatalf("unset variable: %v", err)
	}
	if _, err := splitArgs(`click "open`, env); err == nil {
		t.Fatal("unterminated quote accepted")
	}
}

func TestQuoteArgRoundTrip(t *testing.T) {
	for _, s := range []string{"plain", "with space", `q"uote`, `back\slash`, "$HOME", "", "css=form > input:nth-of-type(2)", `input[name="a b"]`} {
		toks, err := splitArgs("x "+quoteArg(s), func(string) (string, bool) { return "", false })
		if err != nil || len(toks) != 2 || toks[1].Value != s {
			t.Fatalf("%q -> %q -> %+v (%v)", s, quoteArg(s), toks, err)
		}
	}
}

func TestRecordEventLines(t *testing.T) {
	e := RecordEvent{Op: "type", Selector: "#password", Tag: "input", Type: "password", Text: "Password", URL: "https://x/login", Secret: true}
	ls := e.lines(false)
	if ls[1] != "type css=#password $WEBCLI_PASSWORD" {
		t.Fatalf("secret: %q", ls)
	}
	e = RecordEvent{Op: "type", Selector: `input[name="q"]`, Tag: "input", Type: "text", Value: `a "b" $c`}
	toks, err := splitArgs(e.lines(false)[1], nil)
	if err != nil || toks[1].Value != `css=input[name="q"]` || toks[2].Value != `a "b" $c` {
		t.Fatalf("round trip: %q %+v %v", e.lines(false)[1], toks, err)
	}
	e = RecordEvent{Op: "press", Selector: "#q", Key: "Enter"}
	if got := e.lines(false)[1]; got != "press Enter css=#q" {
		t.Fatalf("press: %q", got)
	}
	e = RecordEvent{Op: "select", Selector: "#country", Value: "Japan"}
	if got := e.lines(false)[1]; got != "select css=#country Japan" {
		t.Fatalf("select: %q", got)
	}
}

func TestRestAfter(t *testing.T) {
	if got := restAfter(`eval  document.querySelectorAll("a b").length`, 1); got != `document.querySelectorAll("a b").length` {
		t.Fatalf("%q", got)
	}
}
