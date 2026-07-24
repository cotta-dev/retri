package logger

import "testing"

func TestStripAnsi(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no ansi",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "color code",
			input: "\x1b[31mred text\x1b[0m",
			want:  "red text",
		},
		{
			name:  "bold",
			input: "\x1b[1mbold\x1b[0m normal",
			want:  "bold normal",
		},
		{
			name:  "cursor movement",
			input: "\x1b[2Jhello",
			want:  "hello",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "multiple codes",
			input: "\x1b[32m\x1b[1mgreen bold\x1b[0m",
			want:  "green bold",
		},
		{
			name:  "mixed content",
			input: "before\x1b[31m red \x1b[0mafter",
			want:  "before red after",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripAnsi(tt.input)
			if got != tt.want {
				t.Errorf("StripAnsi(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
