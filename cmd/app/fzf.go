package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Element is one hint target reported by content.js.
type Element struct {
	Hint int    `json:"hint"`
	Tag  string `json:"tag"`
	Type string `json:"type,omitempty"`
	Role string `json:"role,omitempty"`
	Text string `json:"text"`
	Href string `json:"href,omitempty"`
	Name string `json:"name,omitempty"`
	ID   string `json:"id,omitempty"`
	Rect struct {
		X, Y, W, H float64
	} `json:"rect"`
}

// ElementsPayload is the reply to a collect request.
type ElementsPayload struct {
	URL      string    `json:"url"`
	Title    string    `json:"title"`
	Elements []Element `json:"elements"`
}

func (e Element) kind() string {
	k := e.Tag
	if e.Type != "" {
		k += ":" + e.Type
	} else if e.Role != "" {
		k += "@" + e.Role
	}
	return k
}

// fzfLine renders "<hint>\t<kind>\t<text>\t<href>"; fzf shows fields 2.. and
// returns the full line so the hint can be parsed back.
func (e Element) fzfLine() string {
	clean := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	return fmt.Sprintf("%d\t[%s]\t%s\t%s", e.Hint, e.kind(), clean(e.Text), e.Href)
}

var errFzfCancelled = errors.New("selection cancelled")

// selectWithFzf pipes the element list into the system fzf binary.
//
// Interactive mode (query == ""): fzf reads the list from stdin and draws
// its UI on /dev/tty, so it must run while nothing else reads the terminal.
// Non-interactive mode (query != ""): `fzf --filter` ranks matches with
// the same algorithm and prints them; the best match is used. This is what
// automated tests and `hints <query>` use.
func selectWithFzf(fzfPath string, els []Element, query, header string) (int, error) {
	if len(els) == 0 {
		return 0, errors.New("no visible elements to select")
	}
	bin, err := exec.LookPath(fzfPath)
	if err != nil {
		return 0, fmt.Errorf("fzf not found (%v); install it: https://github.com/junegunn/fzf#installation", err)
	}
	var in bytes.Buffer
	for _, e := range els {
		in.WriteString(e.fzfLine())
		in.WriteByte('\n')
	}
	args := []string{"--delimiter=\t", "--with-nth=2..", "--no-multi"}
	if query != "" {
		args = append(args, "--filter="+query)
	} else {
		args = append(args, "--height=50%", "--reverse", "--prompt=hint> ", "--header="+header)
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdin = &in
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			switch ee.ExitCode() {
			case 1:
				return 0, fmt.Errorf("fzf: no element matches %q", query)
			case 130:
				return 0, errFzfCancelled
			}
		}
		return 0, fmt.Errorf("fzf: %w", err)
	}
	first, _, _ := strings.Cut(out.String(), "\n")
	hintField, _, _ := strings.Cut(first, "\t")
	hint, err := strconv.Atoi(strings.TrimSpace(hintField))
	if err != nil {
		return 0, fmt.Errorf("cannot parse fzf selection %q", first)
	}
	return hint, nil
}
