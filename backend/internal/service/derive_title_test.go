package service

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDeriveTitle(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"empty", "", "New conversation"},
		{"whitespace only", "   \n  ", "New conversation"},
		{"short", "where is auth handled?", "where is auth handled?"},
		{"first line only", "line one\nline two", "line one"},
		{"trimmed", "  hello  ", "hello"},
		{
			"ascii truncated at 80",
			strings.Repeat("a", 100),
			strings.Repeat("a", 77) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveTitle(tt.content); got != tt.want {
				t.Errorf("deriveTitle(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}

// A long title made of multibyte runes must be truncated on a rune boundary,
// never mid-rune — the result must stay valid UTF-8.
func TestDeriveTitleMultibyteStaysValidUTF8(t *testing.T) {
	// Each "世" is 3 bytes; 100 of them is well over the 80-rune cap.
	title := deriveTitle(strings.Repeat("世", 100))

	if !utf8.ValidString(title) {
		t.Fatalf("deriveTitle produced invalid UTF-8: %q", title)
	}
	if !strings.HasSuffix(title, "...") {
		t.Fatalf("expected truncation ellipsis, got %q", title)
	}
	// 77 runes of content + the 3 ASCII dots.
	if got := utf8.RuneCountInString(title); got != 80 {
		t.Fatalf("expected 80 runes (77 + \"...\"), got %d in %q", got, title)
	}
}
