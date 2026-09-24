package main

import (
	"fmt"
	"os"
	"strings"
)

// token is one parsed argument: Value after quote removal and $VAR
// expansion, Raw exactly as written (recordings keep Raw so that secrets
// passed as $VAR are never written to disk).
type token struct {
	Value string
	Raw   string
}

// splitArgs splits a command line shell-style:
//
//	'single quotes'   literal
//	"double quotes"   \" \\ \$ escapes, $VAR / ${VAR} expanded
//	unquoted          $VAR / ${VAR} expanded, backslashes kept (regexps)
//
// An unset variable is an error rather than an empty string, so a missing
// password never types "" silently.
func splitArgs(line string, getenv func(string) (string, bool)) ([]token, error) {
	if getenv == nil {
		getenv = os.LookupEnv
	}
	var out []token
	i, n := 0, len(line)
	for {
		for i < n && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= n {
			return out, nil
		}
		start := i
		var b strings.Builder
		for i < n && line[i] != ' ' && line[i] != '\t' {
			switch c := line[i]; c {
			case '\'':
				j := strings.IndexByte(line[i+1:], '\'')
				if j < 0 {
					return nil, fmt.Errorf("unterminated ' quote")
				}
				b.WriteString(line[i+1 : i+1+j])
				i += j + 2
			case '"':
				i++
				closed := false
				for i < n {
					c := line[i]
					if c == '"' {
						closed = true
						i++
						break
					}
					if c == '\\' && i+1 < n && strings.IndexByte(`"\$`, line[i+1]) >= 0 {
						b.WriteByte(line[i+1])
						i += 2
						continue
					}
					if c == '$' {
						v, k, err := expandVar(line, i, getenv)
						if err != nil {
							return nil, err
						}
						if k > 0 {
							b.WriteString(v)
							i += k
							continue
						}
					}
					b.WriteByte(c)
					i++
				}
				if !closed {
					return nil, fmt.Errorf(`unterminated " quote`)
				}
			case '$':
				v, k, err := expandVar(line, i, getenv)
				if err != nil {
					return nil, err
				}
				if k == 0 {
					b.WriteByte(c)
					i++
				} else {
					b.WriteString(v)
					i += k
				}
			default:
				b.WriteByte(c)
				i++
			}
		}
		out = append(out, token{Value: b.String(), Raw: line[start:i]})
	}
}

// expandVar expands $NAME or ${NAME} at line[i] ('$'). It returns the value
// and the number of bytes consumed (0 when '$' does not start a variable).
func expandVar(line string, i int, getenv func(string) (string, bool)) (string, int, error) {
	isName := func(c byte, first bool) bool {
		return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (!first && c >= '0' && c <= '9')
	}
	j := i + 1
	var name string
	if j < len(line) && line[j] == '{' {
		k := strings.IndexByte(line[j:], '}')
		if k < 0 {
			return "", 0, fmt.Errorf("unterminated ${")
		}
		name = line[j+1 : j+k]
		j += k + 1
	} else {
		k := j
		for k < len(line) && isName(line[k], k == j) {
			k++
		}
		if k == j {
			return "", 0, nil
		}
		name = line[j:k]
		j = k
	}
	v, ok := getenv(name)
	if !ok {
		return "", 0, fmt.Errorf("environment variable %s is not set", name)
	}
	return v, j - i, nil
}

// quoteArg renders s so that splitArgs reads it back verbatim (used when
// writing recordings).
func quoteArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t'\"$\\") {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`)
	return `"` + r.Replace(s) + `"`
}

// restAfter returns the raw text after the first n whitespace-separated
// fields (for commands such as eval that take the rest of the line as is).
func restAfter(line string, n int) string {
	s := strings.TrimLeft(line, " \t")
	for k := 0; k < n; k++ {
		i := strings.IndexAny(s, " \t")
		if i < 0 {
			return ""
		}
		s = strings.TrimLeft(s[i:], " \t")
	}
	return s
}
