// Package text provides string and text utilities.
package text

import (
	"strings"
	"unicode"
)

// MultilineTrim trims whitespace from the beginning of the first line and the end of all lines.
func MultilineTrim(s string) string {
	lines := strings.Split(s, "\n")
	baseIndent := ""
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		content := strings.TrimLeft(line, " \t")
		baseIndent = line[:len(line)-len(content)]
		break
	}

	for i, line := range lines {
		line = strings.TrimPrefix(line, baseIndent)
		line = strings.TrimRightFunc(line, unicode.IsSpace)
		lines[i] = line
	}

	s = strings.Join(lines, "\n")
	s = strings.TrimSpace(s)

	return s
}
