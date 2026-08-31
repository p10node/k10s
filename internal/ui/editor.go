package ui

import (
	"fmt"
	"os/exec"
	"strings"
	"unicode"
)

// editorCommand turns $EDITOR into an executable and argument list, then
// appends the temporary YAML path. Quotes and escaped spaces make editor paths
// and flags such as `code --wait` work without involving a shell.
func editorCommand(value, path string) (*exec.Cmd, error) {
	if strings.TrimSpace(value) == "" {
		value = "vi"
	}
	args, err := splitCommandLine(value)
	if err != nil {
		return nil, fmt.Errorf("invalid $EDITOR: %w", err)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("invalid $EDITOR: command is empty")
	}
	return exec.Command(args[0], append(args[1:], path)...), nil
}

// splitCommandLine implements the small shell-like subset an editor setting
// needs: whitespace-separated arguments, single/double quotes and backslash
// escapes. It deliberately does not perform shell expansion or execute shell
// syntax.
func splitCommandLine(value string) ([]string, error) {
	var (
		args    []string
		current strings.Builder
		quote   rune
		started bool
	)
	runes := []rune(value)
	flush := func() {
		if started {
			args = append(args, current.String())
			current.Reset()
			started = false
		}
	}

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote == '\'' {
			if r == '\'' {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		if quote == '"' {
			switch r {
			case '"':
				quote = 0
			case '\\':
				if i+1 < len(runes) && (runes[i+1] == '"' || runes[i+1] == '\\') {
					i++
					current.WriteRune(runes[i])
				} else {
					current.WriteRune(r)
				}
			default:
				current.WriteRune(r)
			}
			continue
		}

		switch {
		case unicode.IsSpace(r):
			flush()
		case r == '\'' || r == '"':
			quote = r
			started = true
		case r == '\\':
			if i+1 >= len(runes) {
				return nil, fmt.Errorf("trailing escape")
			}
			next := runes[i+1]
			if unicode.IsSpace(next) || next == '\'' || next == '"' || next == '\\' {
				i++
				current.WriteRune(next)
			} else {
				current.WriteRune(r)
			}
			started = true
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote", quote)
	}
	flush()
	return args, nil
}
