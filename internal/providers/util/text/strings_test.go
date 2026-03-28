package text

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMultilineTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard markdown indentation with trailing whitespace",
			input:    "    First line  \n    Second line\t\n      Nested third line \n    Fourth line",
			expected: "First line\nSecond line\n  Nested third line\nFourth line",
		},
		{
			name: "leading and trailing blank lines",
			input: `

  Description starts here
  And ends here
    
`,
			expected: "Description starts here\nAnd ends here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MultilineTrim(tt.input)
			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("MultilineTrim() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
