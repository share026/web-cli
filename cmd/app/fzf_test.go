package main

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// TestSelectWithRealFzf pipes a candidate list into the system fzf in
// --filter mode (the same matcher the interactive UI uses).
func TestSelectWithRealFzf(t *testing.T) {
	if _, err := exec.LookPath("fzf"); err != nil {
		t.Skip("fzf not installed")
	}
	els := []Element{
		{Hint: 1, Tag: "a", Text: "Home", Href: "https://app.test/"},
		{Hint: 2, Tag: "button", Text: "Login"},
		{Hint: 3, Tag: "input", Type: "text", Text: "Search products"},
	}
	for query, want := range map[string]int{"login": 2, "search": 3, "home": 1, "[button]": 2} {
		got, err := selectWithFzf("fzf", els, query, "")
		if err != nil || got != want {
			t.Errorf("query %q: got %d, %v; want %d", query, got, err, want)
		}
	}
	if _, err := selectWithFzf("fzf", els, "zzzz-nothing", ""); err == nil {
		t.Error("expected no-match error")
	}
}

// TestInteractiveFzfOnTTY runs fzf in interactive mode. It needs a real
// terminal, so it only runs when WEBCLI_TTY_TEST=1 (scripts/test-tty.sh runs
// it under a pseudo-terminal and types a query + Enter).
func TestInteractiveFzfOnTTY(t *testing.T) {
	if os.Getenv("WEBCLI_TTY_TEST") != "1" {
		t.Skip("set WEBCLI_TTY_TEST=1 and run under a TTY (scripts/test-tty.sh)")
	}
	els := []Element{
		{Hint: 1, Tag: "a", Text: "Home"},
		{Hint: 2, Tag: "button", Text: "Login"},
		{Hint: 7, Tag: "button", Text: "Checkout"},
	}
	got, err := selectWithFzf("fzf", els, "", "tty test")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "INTERACTIVE_FZF_SELECTED=%d\n", got)
	if want := os.Getenv("WEBCLI_TTY_WANT"); want != "" && want != fmt.Sprint(got) {
		t.Fatalf("selected %d, want %s", got, want)
	}
}
